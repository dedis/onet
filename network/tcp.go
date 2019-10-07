package network

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"go.dedis.ch/onet/v3/log"
	"golang.org/x/xerrors"
)

// a connection will return an io.EOF after networkTimeout if nothing has been
// received. sends and connects will timeout using this timeout as well.
var timeout = 1 * time.Minute

// dialTimeout is the timeout for connecting to an end point.
var dialTimeout = 1 * time.Minute

// Global lock for 'timeout' (because also used in 'tcp_test.go')
// Using a 'RWMutex' to be as efficient as possible, because it will be used
// quite a lot in 'Receive()'.
var timeoutLock = sync.RWMutex{}

// MaxPacketSize limits the amount of memory that is allocated before a packet
// is checked and thrown away if it's not legit. If you need more than 10MB
// packets, increase this value.
var MaxPacketSize = Size(10 * 1024 * 1024)

// NewTCPAddress returns a new Address that has type PlainTCP with the given
// address addr.
func NewTCPAddress(addr string) Address {
	return NewAddress(PlainTCP, addr)
}

// NewTCPRouter returns a new Router using TCPHost as the underlying Host.
func NewTCPRouter(sid *ServerIdentity, suite Suite) (*Router, error) {
	return NewTCPRouterWithListenAddr(sid, suite, "")
}

// NewTCPRouterWithListenAddr returns a new Router using TCPHost with the
// given listen address as the underlying Host.
func NewTCPRouterWithListenAddr(sid *ServerIdentity, suite Suite,
	listenAddr string) (*Router, error) {
	h, err := NewTCPHostWithListenAddr(sid, suite, listenAddr)
	if err != nil {
		return nil, err
	}
	r := NewRouter(sid, h)
	return r, nil
}

// SetTCPDialTimeout sets the dialing timeout for the TCP connection. The
// default is one minute. This function is not thread-safe.
func SetTCPDialTimeout(dur time.Duration) {
	dialTimeout = dur
}

// TCPConn implements the Conn interface using plain, unencrypted TCP.
type TCPConn struct {
	// The connection used
	conn net.Conn

	// the suite used to unmarshal messages
	suite Suite

	// closed indicator
	closed    bool
	closedMut sync.Mutex
	// So we only handle one receiving packet at a time
	receiveMutex sync.Mutex
	// So we only handle one sending packet at a time
	sendMutex sync.Mutex

	counterSafe

	// a hook to let us test dead servers
	receiveRawTest func() ([]byte, error)
}

// NewTCPConn will open a TCPConn to the given address.
// In case of an error it returns a nil TCPConn and the error.
func NewTCPConn(addr Address, suite Suite) (conn *TCPConn, err error) {
	netAddr := addr.NetworkAddress()
	for i := 1; i <= MaxRetryConnect; i++ {
		var c net.Conn
		c, err = net.DialTimeout("tcp", netAddr, dialTimeout)
		if err == nil {
			conn = &TCPConn{
				conn:  c,
				suite: suite,
			}
			return
		}
		if i < MaxRetryConnect {
			time.Sleep(WaitRetry)
		}
	}
	if err == nil {
		err = ErrTimeout
	}
	return
}

// Receive get the bytes from the connection then decodes the buffer.
// It returns the Envelope containing the message,
// or EmptyEnvelope and an error if something wrong happened.
func (c *TCPConn) Receive() (env *Envelope, e error) {
	buff, err := c.receiveRaw()
	if err != nil {
		return nil, err
	}

	id, body, err := Unmarshal(buff, c.suite)
	return &Envelope{
		MsgType: id,
		Msg:     body,
		Size:    Size(len(buff)),
	}, err
}

func (c *TCPConn) receiveRaw() ([]byte, error) {
	if c.receiveRawTest != nil {
		return c.receiveRawTest()
	}
	return c.receiveRawProd()
}

// receiveRawProd reads the size of the message, then the
// whole message. It returns the raw message as slice of bytes.
// If there is no message available, it blocks until one becomes
// available.
// In case of an error it returns a nil slice and the error.
func (c *TCPConn) receiveRawProd() ([]byte, error) {
	c.receiveMutex.Lock()
	defer c.receiveMutex.Unlock()
	timeoutLock.RLock()
	c.conn.SetReadDeadline(time.Now().Add(timeout))
	timeoutLock.RUnlock()
	// First read the size
	var total Size
	if err := binary.Read(c.conn, globalOrder, &total); err != nil {
		return nil, handleError(err)
	}
	if total > MaxPacketSize {
		return nil, fmt.Errorf("%v sends too big packet: %v>%v",
			c.conn.RemoteAddr().String(), total, MaxPacketSize)
	}

	b := make([]byte, total)
	var read Size
	var buffer bytes.Buffer
	for read < total {
		// Read the size of the next packet.
		timeoutLock.RLock()
		c.conn.SetReadDeadline(time.Now().Add(timeout))
		timeoutLock.RUnlock()
		n, err := c.conn.Read(b)
		// Quit if there is an error.
		if err != nil {
			c.updateRx(4 + uint64(read))
			return nil, handleError(err)
		}
		// Append the read bytes into the buffer.
		if _, err := buffer.Write(b[:n]); err != nil {
			log.Error("Couldn't write to buffer:", err)
		}
		read += Size(n)
		b = b[n:]
	}

	// register how many bytes we read. (4 is for the frame size
	// that we read up above).
	c.updateRx(4 + uint64(read))
	return buffer.Bytes(), nil
}

// Send converts the NetworkMessage into an ApplicationMessage
// and sends it using send().
// It returns the number of bytes sent and an error if anything was wrong.
func (c *TCPConn) Send(msg Message) (uint64, error) {
	c.sendMutex.Lock()
	defer c.sendMutex.Unlock()

	b, err := Marshal(msg)
	if err != nil {
		return 0, fmt.Errorf("Error marshaling  message: %s", err.Error())
	}
	return c.sendRaw(b)
}

// sendRaw writes the number of bytes of the message to the network then the
// whole message b in slices of size maxChunkSize.
// In case of an error it aborts.
func (c *TCPConn) sendRaw(b []byte) (uint64, error) {
	timeoutLock.RLock()
	c.conn.SetWriteDeadline(time.Now().Add(timeout))
	timeoutLock.RUnlock()

	// First write the size
	packetSize := Size(len(b))
	if err := binary.Write(c.conn, globalOrder, packetSize); err != nil {
		return 0, err
	}
	// Then send everything through the connection
	// Send chunk by chunk
	log.Lvl5("Sending from", c.conn.LocalAddr(), "to", c.conn.RemoteAddr())
	var sent Size
	for sent < packetSize {
		n, err := c.conn.Write(b[sent:])
		if err != nil {
			sentLen := 4 + uint64(sent)
			c.updateTx(sentLen)
			return sentLen, handleError(err)
		}
		sent += Size(n)
	}
	// update stats on the connection. Plus 4 for the uint32 for the frame size.
	sentLen := 4 + uint64(sent)
	c.updateTx(sentLen)
	return sentLen, nil
}

// Remote returns the name of the peer at the end point of
// the connection.
func (c *TCPConn) Remote() Address {
	return Address(c.conn.RemoteAddr().String())
}

// Local returns the local address and port.
func (c *TCPConn) Local() Address {
	return NewTCPAddress(c.conn.LocalAddr().String())
}

// Type returns PlainTCP.
func (c *TCPConn) Type() ConnType {
	return PlainTCP
}

// Close the connection.
// Returns error if it couldn't close the connection.
func (c *TCPConn) Close() error {
	c.closedMut.Lock()
	defer c.closedMut.Unlock()
	if c.closed == true {
		return ErrClosed
	}
	err := c.conn.Close()
	c.closed = true
	if err != nil {
		handleError(err)
	}
	return nil
}

// handleError translates the network-layer error to a set of errors
// used in our packages.
func handleError(err error) error {
	if strings.Contains(err.Error(), "use of closed") || strings.Contains(err.Error(), "broken pipe") {
		return ErrClosed
	} else if strings.Contains(err.Error(), "canceled") {
		return ErrCanceled
	} else if err == io.EOF || strings.Contains(err.Error(), "EOF") {
		return ErrEOF
	}

	netErr, ok := err.(net.Error)
	if !ok {
		return ErrUnknown
	}
	if netErr.Timeout() {
		return ErrTimeout
	}

	log.Errorf("Unknown error caught: %s", err.Error())
	return ErrUnknown
}

// TCPListener implements the Host-interface using Tcp as a communication
// channel.
type TCPListener struct {
	// the underlying golang/net listener.
	listener net.Listener
	// the close channel used to indicate to the listener we want to quit.
	quit chan bool
	// quitListener is a channel to indicate to the closing function that the
	// listener has actually really quit.
	quitListener  chan bool
	listeningLock sync.Mutex
	listening     bool

	// closed tells the listen routine to return immediately if a
	// Stop() has been called.
	closed bool

	// actual listening addr which might differ from initial address in
	// case of ":0"-address.
	addr net.Addr

	// Is this a TCP or a TLS listener?
	conntype ConnType

	// suite that is given to each incoming connection
	suite Suite
}

// NewTCPListener returns a TCPListener. This function binds globally using
// the port of 'addr'.
// It returns the listener and an error if one occurred during
// the binding.
// A subsequent call to Address() gives the actual listening
// address which is different if you gave it a ":0"-address.
func NewTCPListener(addr Address, s Suite) (*TCPListener, error) {
	return NewTCPListenerWithListenAddr(addr, s, "")
}

// NewTCPListenerWithListenAddr returns a TCPListener. This function binds to the
// given 'listenAddr'. If it is empty, the function binds globally using
// the port of 'addr'.
// It returns the listener and an error if one occurred during
// the binding.
// A subsequent call to Address() gives the actual listening
// address which is different if you gave it a ":0"-address.
func NewTCPListenerWithListenAddr(addr Address,
	s Suite, listenAddr string) (*TCPListener, error) {
	if addr.ConnType() != PlainTCP && addr.ConnType() != TLS {
		return nil, xerrors.New("TCPListener can only listen on TCP and TLS addresses")
	}
	t := &TCPListener{
		conntype:     addr.ConnType(),
		quit:         make(chan bool),
		quitListener: make(chan bool),
		suite:        s,
	}
	listenOn, err := getListenAddress(addr, listenAddr)
	if err != nil {
		return nil, err
	}
	for i := 0; i < MaxRetryConnect; i++ {
		ln, err := net.Listen("tcp", listenOn)
		if err == nil {
			t.listener = ln
			break
		} else if i == MaxRetryConnect-1 {
			return nil, xerrors.New("Error opening listener: " + err.Error())
		}
		time.Sleep(WaitRetry)
	}
	t.addr = t.listener.Addr()
	return t, nil
}

// Listen starts to listen for incoming connections and calls fn for every
// connection-request it receives.
// If the connection is closed, an error will be returned.
func (t *TCPListener) Listen(fn func(Conn)) error {
	receiver := func(tc Conn) {
		go fn(tc)
	}
	return t.listen(receiver)
}

// listen is the private function that takes a function that takes a TCPConn.
// That way we can control what to do of the TCPConn before returning it to the
// function given by the user. fn is called in the same routine.
func (t *TCPListener) listen(fn func(Conn)) error {
	t.listeningLock.Lock()
	if t.closed == true {
		t.listeningLock.Unlock()
		return nil
	}
	t.listening = true
	t.listeningLock.Unlock()
	for {
		conn, err := t.listener.Accept()
		if err != nil {
			select {
			case <-t.quit:
				t.quitListener <- true
				return nil
			default:
			}
			continue
		}
		c := TCPConn{
			conn:  conn,
			suite: t.suite,
		}
		fn(&c)
	}
}

// Stop the listener. It waits till all connections are closed
// and returned from.
// If there is no listener it will return an error.
func (t *TCPListener) Stop() error {
	// lets see if we launched a listening routing
	t.listeningLock.Lock()
	defer t.listeningLock.Unlock()

	close(t.quit)

	if t.listener != nil {
		if err := t.listener.Close(); err != nil {
			if handleError(err) != ErrClosed {
				return err
			}
		}
	}
	var stop bool
	if t.listening {
		for !stop {
			select {
			case <-t.quitListener:
				stop = true
			case <-time.After(time.Millisecond * 50):
				continue
			}
		}
	}

	t.quit = make(chan bool)
	t.listening = false
	t.closed = true
	return nil
}

// Address returns the listening address.
func (t *TCPListener) Address() Address {
	t.listeningLock.Lock()
	defer t.listeningLock.Unlock()
	return NewAddress(t.conntype, t.addr.String())
}

// Listening returns whether it's already listening.
func (t *TCPListener) Listening() bool {
	t.listeningLock.Lock()
	defer t.listeningLock.Unlock()
	return t.listening
}

// getListenAddress returns the address the listener should listen
// on given the server's address (addr) and the address it was told to listen
// on (listenAddr), which could be empty.
// Rules:
// 1. If there is no listenAddr, bind globally with addr.
// 2. If there is only an IP in listenAddr, take the port from addr.
// 3. If there is an IP:Port in listenAddr, take only listenAddr.
// Otherwise return an error.
func getListenAddress(addr Address, listenAddr string) (string, error) {
	// If no `listenAddr`, bind globally.
	if listenAddr == "" {
		return GlobalBind(addr.NetworkAddress())
	}
	_, port, err := net.SplitHostPort(addr.NetworkAddress())
	if err != nil {
		return "", err
	}

	// If 'listenAddr' only contains the host, combine it with the port
	// of 'addr'.
	splitted := strings.Split(listenAddr, ":")
	if len(splitted) == 1 && port != "" {
		return splitted[0] + ":" + port, nil
	}

	// If host and port in `listenAddr`, choose this one.
	hostListen, portListen, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return "", err
	}
	if hostListen != "" && portListen != "" {
		return listenAddr, nil
	}

	return "", fmt.Errorf("Invalid combination of 'addr' (%s) and 'listenAddr' (%s)", addr.NetworkAddress(), listenAddr)
}

// TCPHost implements the Host interface using TCP connections.
type TCPHost struct {
	suite Suite
	sid   *ServerIdentity
	*TCPListener
}

// NewTCPHost returns a new Host using TCP connection based type.
func NewTCPHost(sid *ServerIdentity, s Suite) (*TCPHost, error) {
	return NewTCPHostWithListenAddr(sid, s, "")
}

// NewTCPHostWithListenAddr returns a new Host using TCP connection based type
// listening on the given address.
func NewTCPHostWithListenAddr(sid *ServerIdentity, s Suite,
	listenAddr string) (*TCPHost, error) {
	h := &TCPHost{
		suite: s,
		sid:   sid,
	}
	var err error
	if sid.Address.ConnType() == TLS {
		h.TCPListener, err = NewTLSListenerWithListenAddr(sid, s, listenAddr)
	} else {
		h.TCPListener, err = NewTCPListenerWithListenAddr(sid.Address, s, listenAddr)
	}
	return h, err
}

// Connect can only connect to PlainTCP connections.
// It will return an error if it is not a PlainTCP-connection-type.
func (t *TCPHost) Connect(si *ServerIdentity) (Conn, error) {
	switch si.Address.ConnType() {
	case PlainTCP:
		c, err := NewTCPConn(si.Address, t.suite)
		return c, err
	case TLS:
		return NewTLSConn(t.sid, si, t.suite)
	case InvalidConnType:
		return nil, xerrors.New("This address is not correctly formatted: " + si.Address.String())
	}
	return nil, fmt.Errorf("TCPHost %s can't handle this type of connection: %s", si.Address, si.Address.ConnType())
}
