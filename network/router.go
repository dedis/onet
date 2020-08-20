package network

import (
	"crypto/tls"
	"strings"
	"sync"

	"go.dedis.ch/onet/v3/log"
	"golang.org/x/xerrors"
)

// Router handles all networking operations such as:
//   * listening to incoming connections using a host.Listener method
//   * opening up new connections using host.Connect method
//   * dispatching incoming message using a Dispatcher
//   * dispatching outgoing message maintaining a translation
//   between ServerIdentity <-> address
//   * managing the re-connections of non-working Conn
// Most caller should use the creation function like NewTCPRouter(...),
// NewLocalRouter(...) then use the Host such as:
//
//   router.Start() // will listen for incoming Conn and block
//   router.Stop() // will stop the listening and the managing of all Conn
type Router struct {
	// id is our own ServerIdentity
	ServerIdentity *ServerIdentity
	// address is the real-actual address used by the listener.
	address Address
	// Dispatcher is used to dispatch incoming message to the right recipient
	Dispatcher
	// Host listens for new connections
	host Host
	// connections keeps track of all active connections. Because a connection
	// can be opened at the same time on both endpoints, there can be more
	// than one connection per ServerIdentityID.
	connections map[ServerIdentityID][]Conn
	sync.Mutex

	// boolean flag indicating that the router is already clos{ing,ed}.
	isClosed bool

	// wg waits for all handleConn routines to be done.
	wg sync.WaitGroup

	// Every handler in this list is called by this router when a network error occurs (Timeout, Connection
	// Closed, or EOF). Those handler should be added by using SetErrorHandler(). The 1st argument is the remote
	// server with whom the error happened
	connectionErrorHandlers []func(*ServerIdentity)

	// keep bandwidth of closed connections
	traffic    counterSafe
	msgTraffic counterSafe
	// If paused is not nil, then handleConn will stop processing. When unpaused
	// it will break the connection. This is for testing node failure cases.
	paused chan bool
	// This field should only be set during testing. It disables an important
	// log message meant to discourage TCP connections.
	UnauthOk bool
	// Quiets the startup of the server if set to true.
	Quiet bool

	// Set of valid peers, used to filter allowed in/out connections.
	// It is organized as a data structure allowing for subsets of peers to
	// evolve indipendently, each subset being identified by a PeerSetID.
	validPeers validPeers
}

// PeerSetID is the identifier for a subset of valid peers.
// This should typically be linked in a unique way to a service, e.g.
// hash(serviceID | skipChainID) for ByzCoin
type PeerSetID [32]byte

// NewPeerSetID creates a new PeerSetID from bytes
func NewPeerSetID(data []byte) PeerSetID {
	var p PeerSetID
	copy(p[:], data)

	return p
}

// peerSet is the type representing a subset of valid peers, implemented as a
// map of empty structs.
type peerSet map[ServerIdentityID]struct{}

// validPeers is the type representing all the valid peers.
type validPeers struct {
	peers map[PeerSetID]peerSet
	lock  sync.Mutex
}

// Sets the set of valid peers for a given identifier.
func (vp *validPeers) set(peerSetID PeerSetID, peers []*ServerIdentity) {
	newPeers := make(peerSet)

	for _, peer := range peers {
		newPeers[peer.ID] = struct{}{}
	}

	vp.lock.Lock()
	defer vp.lock.Unlock()

	if vp.peers == nil {
		vp.peers = make(map[PeerSetID]peerSet)
	}

	vp.peers[peerSetID] = newPeers
}

// Returns the set of valid peers for a given identifier.
func (vp *validPeers) get(peerSetID PeerSetID) []ServerIdentityID {
	vp.lock.Lock()
	defer vp.lock.Unlock()

	if vp.peers == nil {
		return nil
	}

	peerList := []ServerIdentityID{}

	for peer := range vp.peers[peerSetID] {
		peerList = append(peerList, peer)
	}

	return peerList
}

// Checks whether the given peer is valid (among all the subsets).
func (vp *validPeers) isValid(peer *ServerIdentity) bool {
	vp.lock.Lock()
	defer vp.lock.Unlock()

	// Until valid peers are initialized, all peers are valid
	if vp.peers == nil {
		return true
	}

	// Search whether the given peer is valid in any of the peer subsets
	for _, peers := range vp.peers {
		_, ok := peers[peer.ID]
		if ok {
			return true
		}
	}

	return false
}

// SetValidPeers sets the set of valid peers for a given PeerSetID
func (r *Router) SetValidPeers(peerSetID PeerSetID, peers []*ServerIdentity) {
	r.validPeers.set(peerSetID, peers)
}

// GetValidPeers returns the set of valid peers for a given PeerSetID
// The return value is `nil` in case the set of valid peers has not yet been
// initialized, meaning that all peers are valid.
func (r *Router) GetValidPeers(peerSetID PeerSetID) []ServerIdentityID {
	return r.validPeers.get(peerSetID)
}

// isPeerValid checks whether the provided ServerIdentity is among the valid
// peers for this router.
func (r *Router) isPeerValid(peer *ServerIdentity) bool {
	return r.validPeers.isValid(peer)
}

// NewRouter returns a new Router attached to a ServerIdentity and the host we want to
// use.
func NewRouter(own *ServerIdentity, h Host) *Router {
	r := &Router{
		ServerIdentity:          own,
		connections:             make(map[ServerIdentityID][]Conn),
		host:                    h,
		Dispatcher:              NewBlockingDispatcher(),
		connectionErrorHandlers: make([]func(*ServerIdentity), 0),
	}
	r.address = h.Address()
	return r
}

// Pause casues the router to stop after reading the next incoming message. It
// sleeps until it is woken up by Unpause. For testing use only.
func (r *Router) Pause() {
	r.Lock()
	if r.paused == nil {
		r.paused = make(chan bool)
	}
	r.Unlock()
}

// Unpause reverses a previous call to Pause. All paused connections are closed
// and the Router is again ready to process messages normally. For testing use only.
func (r *Router) Unpause() {
	r.Lock()
	if r.paused != nil {
		close(r.paused)
		r.paused = nil
	}
	r.Unlock()
}

// Start the listening routine of the underlying Host. This is a
// blocking call until r.Stop() is called.
func (r *Router) Start() {
	if !r.Quiet {
		log.Lvlf3("New router with address %s and public key %s", r.address, r.ServerIdentity.Public)
	}

	// Any incoming connection waits for the remote server identity
	// and will create a new handling routine.
	err := r.host.Listen(func(c Conn) {
		dst, err := r.receiveServerIdentity(c)
		if err != nil {
			if !strings.Contains(err.Error(), "EOF") {
				// Avoid printing error message if it's just a stray connection.
				log.Errorf("receiving server identity from %#v failed: %+v",
					c.Remote().NetworkAddress(), err)
			}
			if err := c.Close(); err != nil {
				log.Error("Couldn't close secure connection:",
					err)
			}
			return
		}

		// Reject incoming connections from invalid peers
		if !r.isPeerValid(dst) {
			log.Errorf("rejecting incoming connection from %v: invalid peer %v",
				c.Remote(), dst.ID)
			if err := c.Close(); err != nil {
				log.Warnf("closing connection: %v", err)
			}
			return
		}

		if err := r.registerConnection(dst, c); err != nil {
			log.Lvl3(r.address, "does not accept incoming connection from", c.Remote(), "because it's closed")
			return
		}
		// start handleConn in a go routine that waits for incoming messages and
		// dispatches them.
		if err := r.launchHandleRoutine(dst, c); err != nil {
			log.Lvl3(r.address, "does not accept incoming connection from", c.Remote(), "because it's closed")
			return
		}
	})
	if err != nil {
		log.Error("Error listening:", err)
	}
}

// Stop the listening routine, and stop any routine of handling
// connections. Calling r.Start(), then r.Stop() then r.Start() again leads to
// an undefined behaviour. Callers should most of the time re-create a fresh
// Router.
func (r *Router) Stop() error {
	var err error
	err = r.host.Stop()
	r.Unpause()
	r.Lock()
	// set the isClosed to true
	r.isClosed = true

	// then close all connections
	for _, arr := range r.connections {
		// take all connections to close
		for _, c := range arr {
			if err := c.Close(); err != nil {
				log.Lvl5(err)
			}
		}
	}
	// wait for all handleConn to finish
	r.Unlock()
	r.wg.Wait()

	if err != nil {
		return xerrors.Errorf("stopping: %v", err)
	}
	return nil
}

// Send sends to an ServerIdentity without wrapping the msg into a
// ProtocolMsg. It can take more than one message at once to be sure that all
// the messages are sent through the same connection and thus are correctly
// ordered.
func (r *Router) Send(e *ServerIdentity, msgs ...Message) (uint64, error) {
	if !r.isPeerValid(e) {
		return 0, xerrors.Errorf("%v rejecting send to %v: invalid peer",
			r.ServerIdentity.ID, e.ID)
	}

	for _, msg := range msgs {
		if msg == nil {
			return 0, xerrors.New("cannot send nil-packets")
		}
	}
	if len(msgs) == 0 {
		return 0, xerrors.New("need to send at least one message")
	}

	// Update the message counter with the new message about to be sent.
	r.msgTraffic.updateTx(1)

	// If sending to ourself, directly dispatch it
	if e.GetID().Equal(r.ServerIdentity.GetID()) {
		var sent uint64
		for _, msg := range msgs {
			log.Lvlf4("Sending to ourself (%s) msg: %+v", e, msg)
			packet := &Envelope{
				ServerIdentity: e,
				MsgType:        MessageType(msg),
				Msg:            msg,
			}
			if err := r.Dispatch(packet); err != nil {
				return 0, xerrors.Errorf("Error dispatching: %s", err)
			}
			// Marshal the message to get its length
			b, err := Marshal(msg)
			if err != nil {
				return 0, xerrors.Errorf("marshaling: %v", err)
			}
			log.Lvl5("Message sent")
			sent += uint64(len(b))
		}
		return sent, nil
	}

	var totSentLen uint64
	c := r.connection(e.GetID())
	if c == nil {
		var sentLen uint64
		var err error
		c, sentLen, err = r.connect(e)
		totSentLen += sentLen
		if err != nil {
			return totSentLen, xerrors.Errorf("connecting: %v", err)
		}
	}

	for _, msg := range msgs {
		log.Lvlf4("%s sends a msg to %s", r.address, e)
		sentLen, err := c.Send(msg)
		totSentLen += sentLen
		if err != nil {
			log.Lvl2(r.address, "Couldn't send to", e, ":", err, "trying again")
			c, sentLen, err := r.connect(e)
			totSentLen += sentLen
			if err != nil {
				return totSentLen, xerrors.Errorf("connecting: %v", err)
			}
			sentLen, err = c.Send(msg)
			totSentLen += sentLen
			if err != nil {
				return totSentLen, xerrors.Errorf("connecting: %v", err)
			}
		}
	}
	log.Lvl5("Message sent")
	return totSentLen, nil
}

// connect starts a new connection and launches the listener for incoming
// messages.
func (r *Router) connect(si *ServerIdentity) (Conn, uint64, error) {
	log.Lvl3(r.address, "Connecting to", si.Address)
	c, err := r.host.Connect(si)
	if err != nil {
		log.Lvl3("Could not connect to", si.Address, err)
		return nil, 0, xerrors.Errorf("connecting: %v", err)
	}
	log.Lvl3(r.address, "Connected to", si.Address)
	var sentLen uint64
	if sentLen, err = c.Send(r.ServerIdentity); err != nil {
		return nil, sentLen, xerrors.Errorf("sending: %v", err)
	}

	if err = r.registerConnection(si, c); err != nil {
		return nil, sentLen, xerrors.Errorf("register connection: %v", err)
	}

	if err = r.launchHandleRoutine(si, c); err != nil {
		return nil, sentLen, xerrors.Errorf("handling routine: %v", err)
	}
	return c, sentLen, nil

}

func (r *Router) removeConnection(si *ServerIdentity, c Conn) {
	r.Lock()
	defer r.Unlock()

	var toDelete = -1
	arr := r.connections[si.GetID()]
	for i, cc := range arr {
		if c == cc {
			toDelete = i
		}
	}

	if toDelete == -1 {
		log.Error("Remove a connection which is not registered !?")
		return
	}

	arr[toDelete] = arr[len(arr)-1]
	arr[len(arr)-1] = nil
	r.connections[si.GetID()] = arr[:len(arr)-1]
}

// triggerConnectionErrorHandlers trigger all registered connectionsErrorHandlers
func (r *Router) triggerConnectionErrorHandlers(remote *ServerIdentity) {
	for _, v := range r.connectionErrorHandlers {
		v(remote)
	}
}

// handleConn waits for incoming messages and calls the dispatcher for
// each new message. It only quits if the connection is closed or another
// unrecoverable error in the connection appears.
func (r *Router) handleConn(remote *ServerIdentity, c Conn) {
	defer func() {
		// Clean up the connection by making sure it's closed.
		if err := c.Close(); err != nil {
			log.Lvl5(r.address, "having error closing conn to", remote.Address, ":", err)
		}
		rx, tx := c.Rx(), c.Tx()
		r.traffic.updateRx(rx)
		r.traffic.updateTx(tx)
		r.wg.Done()
		r.removeConnection(remote, c)
		log.Lvl4("onet close", c.Remote(), "rx", rx, "tx", tx)
	}()
	address := c.Remote()
	log.Lvl3(r.address, "Handling new connection from", remote.Address)
	for {
		packet, err := c.Receive()

		// Be careful not to hold r's mutex while
		// pausing, or else Unpause would deadlock.
		r.Lock()
		paused := r.paused
		r.Unlock()
		if paused != nil {
			<-paused
			r.Lock()
			r.paused = nil
			r.Unlock()
			return
		}

		if r.Closed() {
			return
		}

		if err != nil {
			if xerrors.Is(err, ErrTimeout) {
				log.Lvlf5("%s drops %s connection: timeout", r.ServerIdentity.Address, remote.Address)
				r.triggerConnectionErrorHandlers(remote)
				return
			}

			if xerrors.Is(err, ErrClosed) || xerrors.Is(err, ErrEOF) {
				// Connection got closed.
				log.Lvlf5("%s drops %s connection: closed", r.ServerIdentity.Address, remote.Address)
				r.triggerConnectionErrorHandlers(remote)
				return
			}
			if xerrors.Is(err, ErrUnknown) {
				// The error might not be recoverable so the connection is dropped
				log.Lvlf5("%v drops %v connection: unknown", r.ServerIdentity, remote)
				r.triggerConnectionErrorHandlers(remote)
				return
			}
			// Temporary error, continue.
			log.Lvl3(r.ServerIdentity, "Error with connection", address, "=>", err)
			continue
		}

		packet.ServerIdentity = remote

		// Update the message counter with the new message about to be processed.
		r.msgTraffic.updateRx(1)

		if err := r.Dispatch(packet); err != nil {
			log.Lvl3("Error dispatching:", err)
		}

	}
}

// connection returns the first connection associated with this ServerIdentity.
// If no connection is found, it returns nil.
func (r *Router) connection(sid ServerIdentityID) Conn {
	r.Lock()
	defer r.Unlock()
	arr := r.connections[sid]
	if len(arr) == 0 {
		return nil
	}
	return arr[0]
}

// registerConnection registers a ServerIdentity for a new connection, mapped with the
// real physical address of the connection and the connection itself.
// It uses the networkLock mutex.
func (r *Router) registerConnection(remote *ServerIdentity, c Conn) error {
	log.Lvl4(r.address, "Registers", remote.Address)
	r.Lock()
	defer r.Unlock()
	if r.isClosed {
		return xerrors.Errorf("closing: %w", ErrClosed)
	}
	_, okc := r.connections[remote.GetID()]
	if okc {
		log.Lvl5("Connection already registered. " +
			"Appending new connection to same identity.")
	}
	r.connections[remote.GetID()] = append(r.connections[remote.GetID()], c)
	return nil
}

func (r *Router) launchHandleRoutine(dst *ServerIdentity, c Conn) error {
	r.Lock()
	defer r.Unlock()
	if r.isClosed {
		return xerrors.Errorf("closing: %w", ErrClosed)
	}
	r.wg.Add(1)
	go r.handleConn(dst, c)
	return nil
}

// Closed returns true if the router is closed (or is closing). For a router
// to be closed means that a call to Stop() must have been made.
func (r *Router) Closed() bool {
	r.Lock()
	defer r.Unlock()
	return r.isClosed
}

// Tx implements monitor/CounterIO
// It returns the Tx for all connections managed by this router
func (r *Router) Tx() uint64 {
	r.Lock()
	defer r.Unlock()
	var tx uint64
	for _, arr := range r.connections {
		for _, c := range arr {
			tx += c.Tx()
		}
	}
	tx += r.traffic.Tx()
	return tx
}

// Rx implements monitor/CounterIO
// It returns the Rx for all connections managed by this router
func (r *Router) Rx() uint64 {
	r.Lock()
	defer r.Unlock()
	var rx uint64
	for _, arr := range r.connections {
		for _, c := range arr {
			rx += c.Rx()
		}
	}
	rx += r.traffic.Rx()
	return rx
}

// MsgTx implements monitor/CounterIO.
// It returns the number of messages transmitted by the interface.
func (r *Router) MsgTx() uint64 {
	return r.msgTraffic.Tx()
}

// MsgRx implements monitor/CounterIO.
// It returns the number of messages received by the interface.
func (r *Router) MsgRx() uint64 {
	return r.msgTraffic.Rx()
}

// Listening returns true if this router is started.
func (r *Router) Listening() bool {
	return r.host.Listening()
}

// receiveServerIdentity takes a fresh new conn issued by the listener and
// wait for the server identities of the remote party. It returns
// the ServerIdentity of the remote party and register the connection.
func (r *Router) receiveServerIdentity(c Conn) (*ServerIdentity, error) {
	// Receive the other ServerIdentity
	nm, err := c.Receive()
	if err != nil {
		return nil, xerrors.Errorf(
			"Error while receiving ServerIdentity during negotiation: %s", err)
	}
	// Check if it is correct
	if nm.MsgType != ServerIdentityType {
		return nil, xerrors.Errorf("Received wrong type during negotiation %s", nm.MsgType.String())
	}

	// Set the ServerIdentity for this connection
	dst := nm.Msg.(*ServerIdentity)

	// See if we have a cryptographically proven pubkey for this peer. If so,
	// check it against dst.Public.
	if tcpConn, ok := c.(*TCPConn); ok {
		if tlsConn, ok := tcpConn.conn.(*tls.Conn); ok {
			cs := tlsConn.ConnectionState()
			if len(cs.PeerCertificates) == 0 {
				return nil, xerrors.New("TLS connection with no peer certs?")
			}
			pub, err := pubFromCN(tcpConn.suite, cs.PeerCertificates[0].Subject.CommonName)
			if err != nil {
				return nil, xerrors.Errorf("decoding key: %v", err)
			}

			if !pub.Equal(dst.Public) {
				return nil, xerrors.New("mismatch between certificate CommonName and ServerIdentity.Public")
			}
			log.Lvl4(r.address, "Public key from CommonName and ServerIdentity match:", pub)
		} else {
			// We get here for TCPConn && !tls.Conn. Make them wish they were using TLS...
			if !r.UnauthOk {
				log.Warn("Public key", dst.Public, "from ServerIdentity not authenticated.")
			}
		}
	}
	log.Lvlf3("%s: Identity received si=%v from %s", r.address, dst.Public, dst.Address)
	return dst, nil
}

// AddErrorHandler adds a network error handler function for this router. The functions will be called
// on network error (e.g. Timeout, Connection Closed, or EOF) with the identity of the faulty
// remote host as 1st parameter.
func (r *Router) AddErrorHandler(errorHandler func(*ServerIdentity)) {
	r.connectionErrorHandlers = append(r.connectionErrorHandlers, errorHandler)
}
