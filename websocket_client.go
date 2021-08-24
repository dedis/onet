package onet

import (
	"crypto/tls"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.dedis.ch/onet/v3/log"
	"go.dedis.ch/onet/v3/network"
	"go.dedis.ch/protobuf"
	"golang.org/x/xerrors"
)

// Client is a struct used to communicate with a remote Service running on a
// onet.Server. Using Send it can connect to multiple remote Servers.
type Client struct {
	service         string
	connections     map[destination]*websocket.Conn
	connectionsLock map[destination]*sync.Mutex
	suite           network.Suite
	// if not nil, use TLS
	TLSClientConfig *tls.Config
	// whether to keep the connection
	keep bool
	rx   uint64
	tx   uint64
	// How long to wait for a reply
	ReadTimeout time.Duration
	// How long to wait to open a connection
	HandshakeTimeout time.Duration
	sync.Mutex
}

// NewClient returns a client using the service s. On the first Send, the
// connection will be started, until Close is called.
func NewClient(suite network.Suite, s string) *Client {
	return &Client{
		service:          s,
		connections:      make(map[destination]*websocket.Conn),
		connectionsLock:  make(map[destination]*sync.Mutex),
		suite:            suite,
		ReadTimeout:      time.Second * 60,
		HandshakeTimeout: time.Second * 5,
	}
}

// NewClientKeep returns a Client that doesn't close the connection between
// two messages if it's the same server.
func NewClientKeep(suite network.Suite, s string) *Client {
	cl := NewClient(suite, s)
	cl.keep = true
	return cl
}

// Suite returns the cryptographic suite in use on this connection.
func (c *Client) Suite() network.Suite {
	return c.suite
}

func (c *Client) closeSingleUseConn(dst *network.ServerIdentity, path string) {
	dest := destination{dst, path}
	if !c.keep {
		if err := c.closeConn(dest); err != nil {
			log.Errorf("error while closing the connection to %v : %+v\n",
				dest, err)
		}
	}
}

func (c *Client) newConnIfNotExist(dst *network.ServerIdentity, path string) (*websocket.Conn, *sync.Mutex, error) {
	var err error

	// c.Lock protects the connections and connectionsLock map
	// c.connectionsLock is held as long as the connection is in use - to avoid that two
	// processes send data over the same websocket concurrently.
	dest := destination{dst, path}
	c.Lock()
	connLock, exists := c.connectionsLock[dest]
	if !exists {
		c.connectionsLock[dest] = &sync.Mutex{}
		connLock = c.connectionsLock[dest]
	}
	c.Unlock()
	// if connLock.Lock is done while the c.Lock is still held, the next process trying to
	// use the same connection will deadlock, as it'll wait for connLock to be released,
	// while the other process will wait for c.Unlock to be released.
	connLock.Lock()
	c.Lock()
	conn, connected := c.connections[dest]
	c.Unlock()

	if !connected {
		d := &websocket.Dialer{}
		d.TLSClientConfig = c.TLSClientConfig

		var serverURL string
		var header http.Header

		// If the URL is in the dst, then use it.
		if dst.URL != "" {
			u, err := url.Parse(dst.URL)
			if err != nil {
				connLock.Unlock()
				return nil, nil, xerrors.Errorf("parsing url: %v", err)
			}
			if u.Scheme == "https" {
				u.Scheme = "wss"
			} else {
				u.Scheme = "ws"
			}
			if !strings.HasSuffix(u.Path, "/") {
				u.Path += "/"
			}
			u.Path += c.service + "/" + path
			serverURL = u.String()
			header = http.Header{"Origin": []string{dst.URL}}
		} else {
			// Open connection to service.
			hp, err := getWSHostPort(dst, false)
			if err != nil {
				connLock.Unlock()
				return nil, nil, xerrors.Errorf("parsing port: %v", err)
			}

			var wsProtocol string
			var protocol string

			// The old hacky way of deciding if this server has HTTPS or not:
			// the client somehow magically knows and tells onet by setting
			// c.TLSClientConfig to a non-nil value.
			if c.TLSClientConfig != nil {
				wsProtocol = "wss"
				protocol = "https"
			} else {
				wsProtocol = "ws"
				protocol = "http"
			}
			serverURL = fmt.Sprintf("%s://%s/%s/%s", wsProtocol, hp, c.service, path)
			header = http.Header{"Origin": []string{protocol + "://" + hp}}
		}

		// Re-try to connect in case the websocket is just about to start
		d.HandshakeTimeout = c.HandshakeTimeout
		for a := 0; a < network.MaxRetryConnect; a++ {
			conn, _, err = d.Dial(serverURL, header)
			if err == nil {
				break
			}
			time.Sleep(network.WaitRetry)
		}
		if err != nil {
			connLock.Unlock()
			return nil, nil, xerrors.Errorf("dial: %v", err)
		}
		c.Lock()
		c.connections[dest] = conn
		c.Unlock()
	}
	return conn, connLock, nil
}

// Send will marshal the message into a ClientRequest message and send it. It has a
// very simple parallel sending mechanism included: if the send goes to a new or an
// idle connection, the message is sent right away. If the current connection is busy,
// it waits for it to be free.
func (c *Client) Send(dst *network.ServerIdentity, path string, buf []byte) ([]byte, error) {
	conn, connLock, err := c.newConnIfNotExist(dst, path)
	if err != nil {
		return nil, xerrors.Errorf("new connection: %v", err)
	}
	defer connLock.Unlock()

	var rcv []byte
	defer func() {
		c.Lock()
		c.closeSingleUseConn(dst, path)
		c.rx += uint64(len(rcv))
		c.tx += uint64(len(buf))
		c.Unlock()
	}()

	log.Lvlf4("Sending %x to %s/%s", buf, c.service, path)
	if err := conn.WriteMessage(websocket.BinaryMessage, buf); err != nil {
		return nil, xerrors.Errorf("connection write: %v", err)
	}

	if err := conn.SetReadDeadline(time.Now().Add(c.ReadTimeout)); err != nil {
		return nil, xerrors.Errorf("read deadline: %v", err)
	}
	_, rcv, err = conn.ReadMessage()
	if err != nil {
		return nil, xerrors.Errorf("connection read: %v", err)
	}
	return rcv, nil
}

// SendProtobuf wraps protobuf.(En|De)code over the Client.Send-function. It
// takes the destination, a pointer to a msg-structure that will be
// protobuf-encoded and sent over the websocket. If ret is non-nil, it
// has to be a pointer to the struct that is sent back to the
// client. If there is no error, the ret-structure is filled with the
// data from the service.
func (c *Client) SendProtobuf(dst *network.ServerIdentity, msg interface{}, ret interface{}) error {
	buf, err := protobuf.Encode(msg)
	if err != nil {
		return xerrors.Errorf("encoding: %v", err)
	}
	path := strings.Split(reflect.TypeOf(msg).String(), ".")[1]
	reply, err := c.Send(dst, path, buf)
	if err != nil {
		return xerrors.Errorf("sending: %v", err)
	}
	if ret != nil {
		err := protobuf.DecodeWithConstructors(reply, ret, network.DefaultConstructors(c.suite))
		if err != nil {
			return xerrors.Errorf("decoding: %v", err)
		}
	}
	return nil
}

// ParallelOptions defines how SendProtobufParallel behaves. Each field has a default
// value that will be used if 'nil' is passed to SendProtobufParallel. For integers,
// the default will also be used if the integer = 0.
type ParallelOptions struct {
	// Parallel indicates how many requests are sent in parallel.
	//   Default: half of all nodes in the roster
	Parallel int
	// AskNodes indicates how many requests are sent in total.
	//   Default: all nodes in the roster, except if StartNodes is set > 0
	AskNodes int
	// StartNode indicates where to start in the roster. If StartNode is > 0 and < len(roster),
	// but AskNodes is 0, then AskNodes will be set to len(Roster)-StartNode.
	//   Default: 0
	StartNode int
	// QuitError - if true, the first error received will be returned.
	//   Default: false
	QuitError bool
	// IgnoreNodes is a set of nodes that will not be contacted. They are counted towards
	// AskNodes and StartNode, but not contacted.
	//   Default: false
	IgnoreNodes []*network.ServerIdentity
	// DontShuffle - if true, the nodes will be contacted in the same order as given in the Roster.
	// StartNode will be applied before shuffling.
	//   Default: false
	DontShuffle bool
}

// GetList returns how many requests to start in parallel and a channel of nodes to be used.
// If po == nil, it uses default values.
func (po *ParallelOptions) GetList(nodes []*network.ServerIdentity) (parallel int, nodesChan chan *network.ServerIdentity) {
	// Default values
	parallel = (len(nodes) + 1) / 2
	askNodes := len(nodes)
	startNode := 0
	var ignoreNodes []*network.ServerIdentity
	var perm []int
	if po != nil {
		if po.Parallel > 0 && po.Parallel < parallel {
			parallel = po.Parallel
		}
		if po.StartNode > 0 && po.StartNode < len(nodes) {
			startNode = po.StartNode
			askNodes -= startNode
		}
		if po.AskNodes > 0 && po.AskNodes < len(nodes) {
			askNodes = po.AskNodes
		}
		if askNodes < parallel {
			parallel = askNodes
		}
		if po.DontShuffle {
			for i := range nodes {
				perm = append(perm, i)
			}
		}
		ignoreNodes = po.IgnoreNodes
	}
	if len(perm) == 0 {
		perm = rand.Perm(len(nodes))
	}

	nodesChan = make(chan *network.ServerIdentity, askNodes)
	for i := range nodes {
		addNode := true
		node := nodes[(startNode+perm[i])%len(nodes)]
		for _, ignore := range ignoreNodes {
			if node.Equal(ignore) {
				addNode = false
				break
			}
		}
		if addNode {
			nodesChan <- node
		}
		if len(nodesChan) == askNodes {
			break
		}
	}
	return parallel, nodesChan
}

// Quit return false if po == nil, or the value in po.QuitError.
func (po *ParallelOptions) Quit() bool {
	if po == nil {
		return false
	}
	return po.QuitError
}

// Decoder is a function that takes the data and the interface to fill in
// as input and decodes the message.
type Decoder func(data []byte, ret interface{}) error

// SendProtobufParallelWithDecoder sends the msg to a set of nodes in parallel and returns the first successful
// answer. If all nodes return an error, only the first error is returned.
// The behaviour of this method can be changed using the ParallelOptions argument. It is kept
// as a structure for future enhancements. If opt is nil, then standard values will be taken.
func (c *Client) SendProtobufParallelWithDecoder(nodes []*network.ServerIdentity, msg interface{}, ret interface{},
	opt *ParallelOptions, decoder Decoder) (*network.ServerIdentity, error) {
	buf, err := protobuf.Encode(msg)
	if err != nil {
		return nil, xerrors.Errorf("decoding: %v", err)
	}
	path := strings.Split(reflect.TypeOf(msg).String(), ".")[1]

	parallel, nodesChan := opt.GetList(nodes)
	nodesNbr := len(nodesChan)
	errChan := make(chan error, nodesNbr)
	decodedChan := make(chan *network.ServerIdentity, 1)
	var decoding sync.Mutex
	done := make(chan bool)

	contactNode := func() bool {
		select {
		case <-done:
			return false
		default:
			select {
			case node := <-nodesChan:
				log.Lvlf3("Asking %T from: %v - %v", msg, node.Address, node.URL)
				reply, err := c.Send(node, path, buf)
				if err != nil {
					log.Lvl2("Error while sending to node:", node, err)
					errChan <- err
				} else {
					log.Lvl3("Done asking node", node, len(reply))
					decoding.Lock()
					select {
					case <-done:
					default:
						if ret != nil {
							err := decoder(reply, ret)
							if err != nil {
								errChan <- err
								break
							}
						}
						decodedChan <- node
						close(done)
					}
					decoding.Unlock()
				}
			default:
				return false
			}
		}
		return true
	}

	// Producer that puts messages in errChan and replyChan
	for g := 0; g < parallel; g++ {
		go func() {
			for {
				if !contactNode() {
					return
				}
			}
		}()
	}

	var errs []error
	for len(errs) < nodesNbr {
		select {
		case node := <-decodedChan:
			return node, nil
		case err := <-errChan:
			if opt.Quit() {
				close(done)
				return nil, err
			}
			errs = append(errs, xerrors.Errorf("sending: %v", err))
		}
	}

	return nil, errs[0]
}

// SendProtobufParallel sends the msg to a set of nodes in parallel and returns the first successful
// answer. If all nodes return an error, only the first error is returned.
// The behaviour of this method can be changed using the ParallelOptions argument. It is kept
// as a structure for future enhancements. If opt is nil, then standard values will be taken.
func (c *Client) SendProtobufParallel(nodes []*network.ServerIdentity, msg interface{}, ret interface{},
	opt *ParallelOptions) (*network.ServerIdentity, error) {
	si, err := c.SendProtobufParallelWithDecoder(nodes, msg, ret, opt, protobuf.Decode)
	if err != nil {
		return nil, xerrors.Errorf("sending: %v", err)
	}
	return si, nil
}

// StreamingConn allows clients to read from it without sending additional
// requests.
type StreamingConn struct {
	conn  *websocket.Conn
	suite network.Suite
}

// StreamingReadOpts contains options for the ReadMessageWithOpts. It allows us
// to add new options in the future without making breaking changes.
type StreamingReadOpts struct {
	Deadline time.Time
}

// ReadMessage read more data from the connection, it will block if there are
// no messages.
func (c *StreamingConn) ReadMessage(ret interface{}) error {
	opts := StreamingReadOpts{
		Deadline: time.Now().Add(5 * time.Minute),
	}

	return c.readMsg(ret, opts)
}

// ReadMessageWithOpts does the same as ReadMessage and allows to pass options.
func (c *StreamingConn) ReadMessageWithOpts(ret interface{}, opts StreamingReadOpts) error {
	return c.readMsg(ret, opts)
}

func (c *StreamingConn) readMsg(ret interface{}, opts StreamingReadOpts) error {
	if err := c.conn.SetReadDeadline(opts.Deadline); err != nil {
		return xerrors.Errorf("read deadline: %v", err)
	}
	// No need to add bytes to counter here because this function is only
	// called by the client.
	_, buf, err := c.conn.ReadMessage()
	if err != nil {
		return xerrors.Errorf("connection read: %w", err)
	}
	err = protobuf.DecodeWithConstructors(buf, ret, network.DefaultConstructors(c.suite))
	if err != nil {
		return xerrors.Errorf("decoding: %v", err)
	}
	return nil
}

// Ping sends a ping message. Data can be nil.
func (c *StreamingConn) Ping(data []byte, deadline time.Time) error {
	return c.conn.WriteControl(websocket.PingMessage, data, deadline)
}

// Stream will send a request to start streaming, it returns a connection where
// the client can continue to read values from it.
func (c *Client) Stream(dst *network.ServerIdentity, msg interface{}) (StreamingConn, error) {
	buf, err := protobuf.Encode(msg)
	if err != nil {
		return StreamingConn{}, err
	}
	path := strings.Split(reflect.TypeOf(msg).String(), ".")[1]

	conn, connLock, err := c.newConnIfNotExist(dst, path)
	if err != nil {
		return StreamingConn{}, err
	}
	defer connLock.Unlock()
	err = conn.WriteMessage(websocket.BinaryMessage, buf)
	if err != nil {
		return StreamingConn{}, err
	}
	c.Lock()
	c.tx += uint64(len(buf))
	c.Unlock()
	return StreamingConn{conn, c.Suite()}, nil
}

// SendToAll sends a message to all ServerIdentities of the Roster and returns
// all errors encountered concatenated together as a string.
func (c *Client) SendToAll(dst *Roster, path string, buf []byte) ([][]byte, error) {
	msgs := make([][]byte, len(dst.List))
	var errstrs []string
	for i, e := range dst.List {
		var err error
		msgs[i], err = c.Send(e, path, buf)
		if err != nil {
			errstrs = append(errstrs, fmt.Sprint(e.String(), err.Error()))
		}
	}
	var err error
	if len(errstrs) > 0 {
		err = xerrors.New(strings.Join(errstrs, "\n"))
	}
	return msgs, err
}

// Close sends a close-command to all open connections and returns nil if no
// errors occurred or all errors encountered concatenated together as a string.
func (c *Client) Close() error {
	c.Lock()
	defer c.Unlock()
	var errstrs []string
	for dest := range c.connections {
		connLock := c.connectionsLock[dest]
		c.Unlock()
		connLock.Lock()
		c.Lock()
		if err := c.closeConn(dest); err != nil {
			errstrs = append(errstrs, err.Error())
		}
		connLock.Unlock()
	}
	var err error
	if len(errstrs) > 0 {
		err = xerrors.New(strings.Join(errstrs, "\n"))
	}
	return err
}

// closeConn sends a close-command to the connection. Correct locking must be done
// befor calling this method.
func (c *Client) closeConn(dst destination) error {
	conn, ok := c.connections[dst]
	if ok {
		delete(c.connections, dst)
		err := conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "client closed"))
		if err != nil {
			log.Error("Error while sending closing type:", err)
		}
		return conn.Close()
	}
	return nil
}

// Tx returns the number of bytes transmitted by this Client. It implements
// the monitor.CounterIOMeasure interface.
func (c *Client) Tx() uint64 {
	c.Lock()
	defer c.Unlock()
	return c.tx
}

// Rx returns the number of bytes read by this Client. It implements
// the monitor.CounterIOMeasure interface.
func (c *Client) Rx() uint64 {
	c.Lock()
	defer c.Unlock()
	return c.rx
}

// schemeToPort returns the port corresponding to the given scheme, much like netdb.
func schemeToPort(name string) (uint16, error) {
	switch name {
	case "http":
		return 80, nil
	case "https":
		return 443, nil
	default:
		return 0, fmt.Errorf("no such scheme: %v", name)
	}
}

// getWSHostPort returns the hostname:port to bind to with WebSocket.
// If global is true, the hostname is set to the unspecified 0.0.0.0-address.
// If si.URL is "", the url uses the hostname and port+1 of si.Address.
func getWSHostPort(si *network.ServerIdentity, global bool) (string, error) {
	const portBitSize = 16
	const portNumericBase = 10

	var hostname string
	var port uint16

	if si.URL != "" {
		url, err := url.Parse(si.URL)
		if err != nil {
			return "", fmt.Errorf("unable to parse URL: %v", err)
		}
		if !url.IsAbs() {
			return "", errors.New("URL is not absolute")
		}

		protocolPort, err := schemeToPort(url.Scheme)
		if err != nil {
			return "", fmt.Errorf("unable to translate URL' scheme to port: %v", err)
		}

		portStr := url.Port()
		if portStr == "" {
			port = protocolPort
		} else {
			portRaw, err := strconv.ParseUint(portStr, portNumericBase, portBitSize)
			if err != nil {
				return "", fmt.Errorf("URL doesn't contain a valid port: %v", err)
			}
			port = uint16(portRaw)
		}
		hostname = url.Hostname()
	} else {
		portRaw, err := strconv.ParseUint(si.Address.Port(), portNumericBase, portBitSize)
		if err != nil {
			return "", fmt.Errorf("unable to parse port of Address as int: %v", err)
		}
		port = uint16(portRaw + 1)
		hostname = si.Address.Host()
	}

	if global {
		hostname = "0.0.0.0"
	}

	portFormatted := strconv.FormatUint(uint64(port), 10)
	return net.JoinHostPort(hostname, portFormatted), nil
}
