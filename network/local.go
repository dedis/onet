package network

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// NewLocalRouter returns a fresh router which uses only local queues. It uses
// the default local manager.
// If you need multiple independent local-queues, use NewLocalRouterWithManager.
// In case of an error it is returned together with a nil-Router.
func NewLocalRouter(sid *ServerIdentity, s Suite) (*Router, error) {
	return NewLocalRouterWithManager(defaultLocalManager, sid, s)
}

// NewLocalRouterWithManager is the same as NewLocalRouter but takes a specific
// LocalManager. This is useful to run parallel different local overlays.
// In case of an error it is returned together with a nil-Router.
func NewLocalRouterWithManager(lm *LocalManager, sid *ServerIdentity, s Suite) (*Router, error) {
	h, err := NewLocalHostWithManager(lm, sid.Address, s)
	if err != nil {
		return nil, err
	}
	r := NewRouter(sid, h)
	r.UnauthOk = true
	return r, nil
}

// LocalManager keeps a reference to all opened local connections.
// It also keeps track of who is "listening", so it's possible to mimic
// Conn & Listener.
type LocalManager struct {
	// conns maps a remote endpoint to the remote connection.
	conns map[endpoint]*LocalConn
	sync.Mutex
	// The listening-functions used when a new connection-request arrives.
	listening map[Address]func(Conn)

	// connection-counter for giving unique IDs to each connection.
	counter uint64
	// close on this channel indicates that connection retries should stop
	stopping chan bool
	stopped  bool
	// a waitgroup to check that all serving goroutines are done
	wg sync.WaitGroup
}

// NewLocalManager returns a fresh new manager that can be used by LocalConn,
// LocalListener & LocalHost.
func NewLocalManager() *LocalManager {
	return &LocalManager{
		conns:     make(map[endpoint]*LocalConn),
		listening: make(map[Address]func(Conn)),
		stopping:  make(chan bool),
	}
}

// defaultLocalManager can be used if you need only one LocalManager.
var defaultLocalManager = NewLocalManager()

// endpoint represents one endpoint of a connection.
type endpoint struct {
	// addr is the Address of this endpoint.
	addr Address
	// uid is a unique identifier of the remote endpoint
	// it's unique  for each direction:
	// 127.0.0.1:2000 -> 127.0.0.1:7869 => 14
	// 127.0.0.1:7869 <- 127.0.0.1:2000 => 15
	uid uint64
}

// LocalReset resets the map of connections + listeners for the defaultLocalManager.
func LocalReset() {
	defaultLocalManager = NewLocalManager()

}

// isListening returns true if the remote address is listening for connections.
func (lm *LocalManager) isListening(remote Address) bool {
	lm.Lock()
	defer lm.Unlock()
	_, ok := lm.listening[remote]
	return ok
}

// setListening marks the address as being able to accept incoming connections.
// For each incoming connection, fn will be called in a go routine.
func (lm *LocalManager) setListening(addr Address, fn func(Conn)) {
	lm.Lock()
	defer lm.Unlock()
	lm.listening[addr] = fn
}

// unsetListening marks the address as *not* being able to accept incoming
// connections.
func (lm *LocalManager) unsetListening(addr Address) {
	lm.Lock()
	defer lm.Unlock()
	delete(lm.listening, addr)
}

// connect checks if the remote address is listening. Then it creates
// the two connections, and launches the listening function in a go routine.
// It returns the outgoing connection, or nil followed by an error, if any.
func (lm *LocalManager) connect(local, remote Address, s Suite) (*LocalConn, error) {
	lm.Lock()
	defer lm.Unlock()
	if lm.stopped {
		return nil, errors.New("system is stopped")
	}

	fn, ok := lm.listening[remote]
	if !ok {
		return nil, fmt.Errorf("%s can't connect to %s: it's not listening", local, remote)
	}

	outEndpoint := endpoint{local, lm.counter}
	lm.counter++

	incEndpoint := endpoint{remote, lm.counter}
	lm.counter++

	outgoing := newLocalConn(lm, outEndpoint, incEndpoint, s)
	incoming := newLocalConn(lm, incEndpoint, outEndpoint, s)

	// map the endpoint to the connection
	lm.conns[outEndpoint] = outgoing
	lm.conns[incEndpoint] = incoming

	go fn(incoming)
	return outgoing, nil
}

// send gets the connection denoted by this endpoint and calls queueMsg
// with the packet as argument to it.
// It returns ErrClosed if it does not find the connection.
func (lm *LocalManager) send(e endpoint, msg []byte) error {
	lm.Lock()
	defer lm.Unlock()
	q, ok := lm.conns[e]
	if !ok {
		return ErrClosed
	}

	q.incomingQueue <- msg
	return nil
}

// close gets the connection denoted by this endpoint and closes it if
// it is present.
func (lm *LocalManager) close(conn *LocalConn) error {
	lm.Lock()
	defer lm.Unlock()
	_, ok := lm.conns[conn.local]
	if !ok {
		// connection already closed
		return ErrClosed
	}
	// delete this conn
	delete(lm.conns, conn.local)
	conn.closeChannels()
	// and delete the remote one + close it
	remote, ok := lm.conns[conn.remote]
	if !ok {
		return nil
	}
	delete(lm.conns, conn.remote)
	remote.closeChannels()
	return nil
}

// len returns how many local connections are open.
func (lm *LocalManager) count() int {
	lm.Lock()
	defer lm.Unlock()
	return len(lm.conns)
}

// Stop tells any connections that are sleeping on retry
// to stop sleeping and return an error.
func (lm *LocalManager) Stop() {
	lm.Lock()
	lm.stopped = true
	close(lm.stopping)
	lm.Unlock()
	for _, v := range lm.conns {
		lm.close(v)
	}

	lm.wg.Wait()
}

// LocalConn is a connection that sends and receives messages to other
// connections locally.
type LocalConn struct {
	local  endpoint
	remote endpoint

	// the channel where incoming messages are dispatched
	incomingQueue chan []byte
	// the channel where messages stored can be retrieved with Receive()
	outgoingQueue chan []byte
	// the channel used to communicate the stopping of the operations
	closeCh chan bool
	// the confirmation channel for the go routine
	closeConfirm chan bool

	// counter to keep track of how many bytes read / written this connection
	// has seen.
	counterSafe
	// the localManager responsible for that connection.
	manager *LocalManager

	// the suite used to unmarshal
	suite Suite
}

// newLocalConn initializes the fields of a LocalConn but doesn't
// connect. It should not be used from the outside, most user want
// to use NewLocalConn.
func newLocalConn(lm *LocalManager, local, remote endpoint, s Suite) *LocalConn {
	lc := &LocalConn{
		remote:        remote,
		local:         local,
		manager:       lm,
		incomingQueue: make(chan []byte, LocalMaxBuffer),
		outgoingQueue: make(chan []byte, LocalMaxBuffer),
		closeCh:       make(chan bool),
		closeConfirm:  make(chan bool),
		suite:         s,
	}
	lm.wg.Add(1)
	go lc.start(&lm.wg)
	return lc
}

// NewLocalConnWithManager is similar to NewLocalConn but takes a specific
// LocalManager.
func NewLocalConnWithManager(lm *LocalManager, local, remote Address, s Suite) (*LocalConn, error) {
	for i := 0; i < MaxRetryConnect; i++ {
		c, err := lm.connect(local, remote, s)
		if err == nil {
			return c, nil
		} else if i == MaxRetryConnect-1 {
			return nil, err
		}
		time.Sleep(WaitRetry)
	}
	return nil, errors.New("Could not connect")
}

func (lc *LocalConn) start(wg *sync.WaitGroup) {
	for {
		select {
		case buff := <-lc.incomingQueue:
			lc.outgoingQueue <- buff
		case <-lc.closeCh:
			// to signal that the conn is closed
			close(lc.outgoingQueue)
			close(lc.incomingQueue)
			lc.closeConfirm <- true
			wg.Done()
			return
		}
	}
}

// Send takes a context (that is not used in any way) and a message that
// will be sent to the remote endpoint.
// If there is an error in the connection, it will be returned.
func (lc *LocalConn) Send(msg Message, opt ...Option) (uint64, error) {
	buff, err := Marshal(msg)
	if err != nil {
		return 0, err
	}
	sentLen := uint64(len(buff))
	lc.updateTx(sentLen)
	return sentLen, lc.manager.send(lc.remote, buff)
}

// Receive takes a context (that is not used) and waits for a packet to
// be ready. It returns the received packet.
// In case of an error the packet is nil and the error is returned.
func (lc *LocalConn) Receive() (*Envelope, error) {
	buff, opened := <-lc.outgoingQueue
	if !opened {
		return nil, ErrClosed
	}
	lc.updateRx(uint64(len(buff)))

	id, body, err := Unmarshal(buff, lc.suite)
	return &Envelope{
		MsgType: id,
		Msg:     body,
	}, err
}

// Local returns the local address.
func (lc *LocalConn) Local() Address {
	return lc.local.addr
}

// Remote returns the remote address.
func (lc *LocalConn) Remote() Address {
	return lc.remote.addr
}

// Close shuts down the connection on the local and the remote
// side.
// If the connection is not open, it returns an error.
func (lc *LocalConn) Close() error {
	select {
	case _, o := <-lc.closeCh:
		if !o {
			return ErrClosed
		}
	default:
	}
	return lc.manager.close(lc)
}

func (lc *LocalConn) closeChannels() {
	close(lc.closeCh)
	<-lc.closeConfirm
	close(lc.closeConfirm)
}

// Type implements the Conn interface
func (lc *LocalConn) Type() ConnType {
	return Local
}

// connQueue manages the message queue of a LocalConn.
// Messages are pushed and retrieved in a FIFO-queue.
// All operations are thread-safe.
// The messages are marshalled and stored in the queue as a slice of bytes.
type connQueue struct {
	wg sync.WaitGroup
}

// LocalMaxBuffer is the number of packets that can be sent simultaneously to the
// same address.
const LocalMaxBuffer = 200

// LocalListener implements Listener and uses LocalConn to communicate. It
// behaves as much as possible as a real golang net.Listener but using LocalConn
// as the underlying communication layer.
type LocalListener struct {
	// addr is the addr we're listening to.
	addr Address
	// whether the listening started or not.
	listening bool
	// suite given to connections
	suite Suite

	sync.Mutex

	// quit is used to stop the listening routine.
	quit chan bool

	// the LocalManager used.
	manager *LocalManager
}

// NewLocalListener returns a fresh LocalListener using the defaultLocalManager.
// In case of an error the LocalListener is nil and the error is returned.
func NewLocalListener(addr Address, s Suite) (*LocalListener, error) {
	return NewLocalListenerWithManager(defaultLocalManager, addr, s)
}

// NewLocalListenerWithManager returns a new LocalListener using the
// given LocalManager.
// In case of an error, the LocalListener is nil and the error is returned.
// An error occurs in case the address is invalid or the manager is already
// listening on that address.
func NewLocalListenerWithManager(lm *LocalManager, addr Address, s Suite) (*LocalListener, error) {
	l := &LocalListener{
		quit:    make(chan bool),
		manager: lm,
		suite:   s,
	}
	if addr.ConnType() != Local {
		return nil, errors.New("Wrong address type for local listener")
	}
	if l.manager.isListening(addr) {
		return nil, fmt.Errorf("%s is already listening: can't listen again", addr)
	}
	l.addr = addr
	return l, nil
}

// Listen calls fn every time a connection-request is received. This call blocks until Stop() is
// called on the listener.
// It returns an error if the LocalListener is already listening.
func (ll *LocalListener) Listen(fn func(Conn)) error {
	ll.Lock()
	if ll.listening {
		ll.Unlock()
		return fmt.Errorf("Already listening on %s", ll.addr)
	}
	ll.quit = make(chan bool)
	ll.manager.setListening(ll.addr, fn)
	ll.listening = true
	ll.Unlock()

	<-ll.quit
	return nil
}

// Stop shuts down listening.
// It always returns nil whether ll is listening or not.
func (ll *LocalListener) Stop() error {
	ll.Lock()
	defer ll.Unlock()
	if !ll.listening {
		return nil
	}

	ll.manager.unsetListening(ll.addr)
	close(ll.quit)
	ll.listening = false
	return nil
}

// Address returns the address used to listen.
func (ll *LocalListener) Address() Address {
	ll.Lock()
	defer ll.Unlock()
	return ll.addr
}

// Listening returns true if this Listener is listening for incoming connections.
func (ll *LocalListener) Listening() bool {
	ll.Lock()
	defer ll.Unlock()
	return ll.listening
}

// LocalHost implements the Host interface. It uses LocalConn and LocalListener as
// the underlying means of communication.
type LocalHost struct {
	addr  Address
	suite Suite
	*LocalListener
	lm *LocalManager
}

// NewLocalHost returns a new Host using Local communication. It listens
// on the given addr.
// If an error happened during setup, it returns a nil LocalHost and the error.
func NewLocalHost(addr Address, s Suite) (*LocalHost, error) {
	return NewLocalHostWithManager(defaultLocalManager, addr, s)
}

// NewLocalHostWithManager is similar to NewLocalHost but takes a
// LocalManager used for communication.
// If an error happened during setup, it returns a nil LocalHost and the error.
func NewLocalHostWithManager(lm *LocalManager, addr Address, s Suite) (*LocalHost, error) {
	lh := &LocalHost{
		addr:  addr,
		lm:    lm,
		suite: s,
	}
	var err error
	lh.LocalListener, err = NewLocalListenerWithManager(lm, addr, s)
	return lh, err

}

// Connect sets up a connection to addr. It retries up to
// MaxRetryConnect while waiting between each try.
// In case of an error, it will return a nil Conn.
func (lh *LocalHost) Connect(si *ServerIdentity, opts ...Option) (Conn, error) {
	if si.Address.ConnType() != Local {
		return nil, errors.New("Can't connect to non-Local address")
	}
	var finalErr error
	for i := 0; i < MaxRetryConnect; i++ {
		c, err := NewLocalConnWithManager(lh.lm, lh.addr, si.Address, lh.suite)
		if err == nil {
			return c, nil
		}
		finalErr = err
		select {
		case <-time.After(WaitRetry):
			// sleep done, go try again
		case <-lh.lm.stopping:
			// stop sleeping and return immediately
			return nil, finalErr
		}
	}
	return nil, finalErr

}

// NewLocalAddress returns an Address of type Local with the given raw addr.
func NewLocalAddress(addr string) Address {
	return NewAddress(Local, addr)
}
