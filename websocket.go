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
	graceful "gopkg.in/tylerb/graceful.v1"
)

// WebSocket handles incoming client-requests using the websocket
// protocol. When making a new WebSocket, it will listen one port above the
// ServerIdentity-port-#.
// The websocket protocol has been chosen as smallest common denominator
// for languages including JavaScript.
type WebSocket struct {
	services  map[string]Service
	server    *graceful.Server
	mux       *http.ServeMux
	startstop chan bool
	started   bool
	TLSConfig *tls.Config // can only be modified before Start is called
	sync.Mutex
}

// NewWebSocket opens a webservice-listener one port above the given
// ServerIdentity.
func NewWebSocket(si *network.ServerIdentity) *WebSocket {
	w := &WebSocket{
		services:  make(map[string]Service),
		startstop: make(chan bool),
	}
	webHost, err := getWSHostPort(si, true)
	log.ErrFatal(err)
	w.mux = http.NewServeMux()
	w.mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
		log.Lvl4("ok?", r.RemoteAddr)
		ok := []byte("ok\n")
		w.Write(ok)
	})

	// Add a catch-all handler (longest paths take precedence, so "/" takes
	// all non-registered paths) and correctly upgrade to a websocket and
	// throw an error.
	w.mux.HandleFunc("/", func(wr http.ResponseWriter, re *http.Request) {
		log.Error("request from ", re.RemoteAddr, "for invalid path ", re.URL.Path)

		u := websocket.Upgrader{
			EnableCompression: true,
			// As the website will not be served from ourselves, we
			// need to accept _all_ origins. Cross-site scripting is
			// required.
			CheckOrigin: func(*http.Request) bool {
				return true
			},
		}
		ws, err := u.Upgrade(wr, re, http.Header{})
		if err != nil {
			log.Error(err)
			return
		}

		ws.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(4001, "This service doesn't exist"),
			time.Now().Add(time.Millisecond*500))
		ws.Close()
	})
	w.server = &graceful.Server{
		Timeout: 100 * time.Millisecond,
		Server: &http.Server{
			Addr:    webHost,
			Handler: w.mux,
		},
		NoSignalHandling: true,
	}
	return w
}

// Listening returns true if the server has been started and is
// listening on the ports for incoming connections.
func (w *WebSocket) Listening() bool {
	w.Lock()
	defer w.Unlock()
	return w.started
}

// start listening on the port.
func (w *WebSocket) start() {
	w.Lock()
	w.started = true
	w.server.Server.TLSConfig = w.TLSConfig
	log.Lvl2("Starting to listen on", w.server.Server.Addr)
	started := make(chan bool)
	go func() {
		// Check if server is configured for TLS
		started <- true
		if w.server.Server.TLSConfig != nil && len(w.server.Server.TLSConfig.Certificates) >= 1 {
			w.server.ListenAndServeTLS("", "")
		} else {
			w.server.ListenAndServe()
		}
	}()
	<-started
	w.Unlock()
	w.startstop <- true
}

// registerService stores a service to the given path. All requests to that
// path and it's sub-endpoints will be forwarded to ProcessClientRequest.
func (w *WebSocket) registerService(service string, s Service) error {
	if service == "ok" {
		return errors.New("service name \"ok\" is not allowed")
	}

	w.services[service] = s
	h := &wsHandler{
		service:     s,
		serviceName: service,
	}
	w.mux.Handle(fmt.Sprintf("/%s/", service), h)
	return nil
}

// stop the websocket and free the port.
func (w *WebSocket) stop() {
	w.Lock()
	defer w.Unlock()
	if !w.started {
		return
	}
	log.Lvl3("Stopping", w.server.Server.Addr)
	w.server.Stop(100 * time.Millisecond)
	<-w.startstop
	w.started = false
}

// Pass the request to the websocket.
type wsHandler struct {
	serviceName string
	service     Service
}

// Wrapper-function so that http.Requests get 'upgraded' to websockets
// and handled correctly.
func (t wsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rx := 0
	tx := 0
	n := 0

	defer func() {
		log.Lvl2("ws close", r.RemoteAddr, "n", n, "rx", rx, "tx", tx)
	}()

	u := websocket.Upgrader{
		EnableCompression: true,
		// As the website will not be served from ourselves, we
		// need to accept _all_ origins. Cross-site scripting is
		// required.
		CheckOrigin: func(*http.Request) bool {
			return true
		},
	}
	ws, err := u.Upgrade(w, r, http.Header{})
	if err != nil {
		log.Error(err)
		return
	}
	defer ws.Close()

	// Loop for each message
outerReadLoop:
	for err == nil {
		mt, buf, rerr := ws.ReadMessage()
		if rerr != nil {
			err = rerr
			break
		}
		rx += len(buf)
		n++

		s := t.service
		var reply []byte
		var tun *StreamingTunnel
		path := strings.TrimPrefix(r.URL.Path, "/"+t.serviceName+"/")
		log.Lvlf2("ws request from %s: %s/%s", r.RemoteAddr, t.serviceName, path)
		reply, tun, err = s.ProcessClientRequest(r, path, buf)
		if err == nil {
			if tun == nil {
				tx += len(reply)
				if err = ws.SetWriteDeadline(time.Now().Add(5 * time.Minute)); err != nil {
					log.Error(err)
					break
				}
				if err = ws.WriteMessage(mt, reply); err != nil {
					log.Error(err)
					break
				}
			} else {
				closing := make(chan bool)
				go func() {
					for {
						// Listen for incoming messages to know if the client wants
						// to close the stream.
						_, _, err := ws.ReadMessage()
						if err != nil {
							close(closing)
							return
						}
					}
				}()

				for {
					select {
					case <-closing:
						close(tun.close)
						return
					case reply, ok := <-tun.out:
						if !ok {
							err = errors.New("service finished streaming")
							close(tun.close)
							break outerReadLoop
						}
						tx += len(reply)
						if err = ws.SetWriteDeadline(time.Now().Add(5 * time.Minute)); err != nil {
							log.Error(err)
							close(tun.close)
							break outerReadLoop
						}
						if err = ws.WriteMessage(mt, reply); err != nil {
							log.Error(err)
							close(tun.close)
							break outerReadLoop
						}
					}
				}
			}
		} else {
			log.Errorf("Got an error while executing %s/%s: %s", t.serviceName, path, err.Error())
		}
	}

	errMessage := "unexpected error: "
	if err != nil {
		errMessage += err.Error()
	}

	ws.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseProtocolError, errMessage),
		time.Now().Add(time.Millisecond*500))
	return
}

type destination struct {
	si   *network.ServerIdentity
	path string
}

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
	sync.Mutex
}

// NewClient returns a client using the service s. On the first Send, the
// connection will be started, until Close is called.
func NewClient(suite network.Suite, s string) *Client {
	return &Client{
		service:         s,
		connections:     make(map[destination]*websocket.Conn),
		connectionsLock: make(map[destination]*sync.Mutex),
		suite:           suite,
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
			log.Errorf("error while closing the connection to %v : %v\n", dest, err)
		}
	}
}

func (c *Client) newConnIfNotExist(dst *network.ServerIdentity, path string) (*websocket.Conn, error) {
	var err error

	dest := destination{dst, path}
	c.Lock()
	conn, ok := c.connections[dest]
	c.Unlock()

	if !ok {
		d := &websocket.Dialer{}
		d.TLSClientConfig = c.TLSClientConfig

		var serverURL string
		var header http.Header

		// If the URL is in the dst, then use it.
		if dst.URL != "" {
			u, err := url.Parse(dst.URL)
			if err != nil {
				return nil, err
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
				return nil, err
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
		for a := 0; a < network.MaxRetryConnect; a++ {
			conn, _, err = d.Dial(serverURL, header)
			if err == nil {
				break
			}
			time.Sleep(network.WaitRetry)
		}
		if err != nil {
			return nil, err
		}
		c.Lock()
		c.connections[dest] = conn
		c.Unlock()
	}
	return conn, nil
}

// Send will marshal the message into a ClientRequest message and send it. It has a
// very simple parallel sending mechanism included: if the send goes to a new or an
// idle connection, the message is sent right away. If the current connection is busy,
// it waits for it to be free.
func (c *Client) Send(dst *network.ServerIdentity, path string, buf []byte) ([]byte, error) {
	c.Lock()
	dest := destination{dst, path}
	if c.connectionsLock[dest] == nil {
		c.connectionsLock[dest] = &sync.Mutex{}
	}
	connLock := c.connectionsLock[dest]
	c.Unlock()
	connLock.Lock()
	defer connLock.Unlock()
	conn, err := c.newConnIfNotExist(dst, path)
	if err != nil {
		return nil, err
	}

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
		return nil, err
	}

	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Minute)); err != nil {
		return nil, err
	}
	_, rcv, err = conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	log.Lvlf4("Received %x", rcv)
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
		return err
	}
	path := strings.Split(reflect.TypeOf(msg).String(), ".")[1]
	reply, err := c.Send(dst, path, buf)
	if err != nil {
		return err
	}
	if ret != nil {
		return protobuf.DecodeWithConstructors(reply, ret,
			network.DefaultConstructors(c.suite))
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

// SendProtobufParallel sends the msg to a set of nodes in parallel and returns the first successful
// answer. If all nodes return an error, only the first error is returned.
// The behaviour of this method can be changed using the ParallelOptions argument. It is kept
// as a structure for future enhancements. If opt is nil, then standard values will be taken.
func (c *Client) SendProtobufParallel(nodes []*network.ServerIdentity, msg interface{}, ret interface{},
	opt *ParallelOptions) (*network.ServerIdentity, error) {
	buf, err := protobuf.Encode(msg)
	if err != nil {
		return nil, err
	}
	path := strings.Split(reflect.TypeOf(msg).String(), ".")[1]

	parallel, nodesChan := opt.GetList(nodes)
	errChan := make(chan error, parallel)
	replyChan := make(chan []byte, parallel)
	siChan := make(chan *network.ServerIdentity, parallel)
	finish := &sync.Once{}
	for g := 0; g < parallel; g++ {
		go func() {
			for {
				var node *network.ServerIdentity
				select {
				case node = <-nodesChan:
				default:
				}
				if node == nil {
					return
				}
				log.Lvlf2("Asking %T from: %v - %v", msg, node.Address, node.URL)
				reply, err := c.Send(node, path, buf)
				if err != nil {
					log.Lvl2("Error while sending to node:", node, err)
					errChan <- err
				} else {
					finish.Do(func() { close(nodesChan) })
					siChan <- node
					replyChan <- reply
				}
			}
		}()
	}
	var errors []error
	for len(errors) < parallel {
		select {
		case reply := <-replyChan:
			if ret != nil {
				return <-siChan, protobuf.DecodeWithConstructors(reply, ret,
					network.DefaultConstructors(c.suite))
			}
			return <-siChan, nil
		case err := <-errChan:
			if opt.Quit() {
				return nil, err
			}
			errors = append(errors, err)
		}
	}
	return nil, errors[0]
}

// StreamingConn allows clients to read from it without sending additional
// requests.
type StreamingConn struct {
	conn  *websocket.Conn
	suite network.Suite
}

// ReadMessage read more data from the connection, it will block if there are
// no messages.
func (c *StreamingConn) ReadMessage(ret interface{}) error {
	if err := c.conn.SetReadDeadline(time.Now().Add(5 * time.Minute)); err != nil {
		return err
	}
	// No need to add bytes to counter here because this function is only
	// called by the client.
	_, buf, err := c.conn.ReadMessage()
	if err != nil {
		return err
	}
	return protobuf.DecodeWithConstructors(buf, ret,
		network.DefaultConstructors(c.suite))
}

// Stream will send a request to start streaming, it returns a connection where
// the client can continue to read values from it.
func (c *Client) Stream(dst *network.ServerIdentity, msg interface{}) (StreamingConn, error) {
	buf, err := protobuf.Encode(msg)
	if err != nil {
		return StreamingConn{}, err
	}
	path := strings.Split(reflect.TypeOf(msg).String(), ".")[1]

	conn, err := c.newConnIfNotExist(dst, path)
	c.Lock()
	defer c.Unlock()
	if err != nil {
		return StreamingConn{}, err
	}
	err = conn.WriteMessage(websocket.BinaryMessage, buf)
	if err != nil {
		return StreamingConn{}, err
	}
	c.tx += uint64(len(buf))
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
		err = errors.New(strings.Join(errstrs, "\n"))
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
		if err := c.closeConn(dest); err != nil {
			errstrs = append(errstrs, err.Error())
		}
	}
	var err error
	if len(errstrs) > 0 {
		err = errors.New(strings.Join(errstrs, "\n"))
	}
	return err
}

// closeConn sends a close-command to the connection.
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

// getWSHostPort returns the host:port+1 of the serverIdentity. If
// global is true, the address is set to the unspecified 0.0.0.0-address.
func getWSHostPort(si *network.ServerIdentity, global bool) (string, error) {
	p, err := strconv.Atoi(si.Address.Port())
	if err != nil {
		return "", err
	}
	host := si.Address.Host()
	if global {
		host = "0.0.0.0"
	}
	return net.JoinHostPort(host, strconv.Itoa(p+1)), nil
}
