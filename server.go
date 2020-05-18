package onet

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/onet/v3/cfgpath"
	"go.dedis.ch/onet/v3/log"
	"go.dedis.ch/onet/v3/network"
	"golang.org/x/xerrors"
	"rsc.io/goversion/version"
)

// Server connects the Router, the Overlay, and the Services together. It sets
// up everything and returns once a working network has been set up.
type Server struct {
	// Our private-key
	private kyber.Scalar
	*network.Router
	// Overlay handles the mapping from tree and entityList to ServerIdentity.
	// It uses tokens to represent an unique ProtocolInstance in the system
	overlay *Overlay
	// lock associated to access trees
	treesLock            sync.Mutex
	serviceManager       *serviceManager
	statusReporterStruct *statusReporterStruct
	// protocols holds a map of all available protocols and how to create an
	// instance of it
	protocols *protocolStorage
	// webservice
	WebSocket *WebSocket
	// when this node has been started
	started time.Time
	// once everything's up and running
	closeitChannel chan bool
	IsStarted      bool

	suite network.Suite

	// set of valid peers, used to filter allowed in/out connections
	// (using an atomic.Value to prevent races)
	validPeers atomic.Value
}

func dbPathFromEnv() string {
	p := os.Getenv("CONODE_SERVICE_PATH")
	if p == "" {
		p = cfgpath.GetDataPath("conode")
	}
	return p
}

// NewServer returns a fresh Server tied to a given Router.
// If dbPath is "", the server will write its database to the default
// location. If dbPath is != "", it is considered a temp dir, and the
// DB is deleted on close.
func newServer(s network.Suite, dbPath string, r *network.Router, pkey kyber.Scalar) *Server {
	delDb := false
	if dbPath == "" {
		dbPath = dbPathFromEnv()
		log.ErrFatal(os.MkdirAll(dbPath, 0750))
	} else {
		delDb = true
	}

	c := &Server{
		private:              pkey,
		statusReporterStruct: newStatusReporterStruct(),
		Router:               r,
		protocols:            newProtocolStorage(),
		suite:                s,
		closeitChannel:       make(chan bool),
	}
	c.overlay = NewOverlay(c)
	c.WebSocket = NewWebSocket(r.ServerIdentity)
	c.serviceManager = newServiceManager(c, c.overlay, dbPath, delDb)
	c.statusReporterStruct.RegisterStatusReporter("Generic", c)
	return c
}

// NewServerTCP returns a new Server out of a private-key and its related
// public key within the ServerIdentity. The server will use a default
// TcpRouter as Router.
func NewServerTCP(e *network.ServerIdentity, suite network.Suite) *Server {
	return NewServerTCPWithListenAddr(e, suite, "")
}

// NewServerTCPWithListenAddr returns a new Server out of a private-key and
// its related public key within the ServerIdentity. The server will use a
// TcpRouter listening on the given address as Router.
func NewServerTCPWithListenAddr(e *network.ServerIdentity, suite network.Suite,
	listenAddr string) *Server {
	r, err := network.NewTCPRouterWithListenAddr(e, suite, listenAddr)
	log.ErrFatal(err)
	return newServer(suite, "", r, e.GetPrivate())
}

// Suite can (and should) be used to get the underlying Suite.
// Currently the suite is hardcoded into the network library.
// Don't use network.Suite but Host's Suite function instead if possible.
func (c *Server) Suite() network.Suite {
	return c.suite
}

var gover version.Version
var goverOnce sync.Once
var goverOk = false

// GetStatus is a function that returns the status report of the server.
func (c *Server) GetStatus() *Status {
	a := c.serviceManager.availableServices()
	sort.Strings(a)

	st := &Status{Field: map[string]string{
		"Available_Services": strings.Join(a, ","),
		"TX_bytes":           strconv.FormatUint(c.Router.Tx(), 10),
		"RX_bytes":           strconv.FormatUint(c.Router.Rx(), 10),
		"Uptime":             time.Now().Sub(c.started).String(),
		"System": fmt.Sprintf("%s/%s/%s", runtime.GOOS, runtime.GOARCH,
			runtime.Version()),
		"Host":        c.ServerIdentity.Address.Host(),
		"Port":        c.ServerIdentity.Address.Port(),
		"Description": c.ServerIdentity.Description,
		"ConnType":    string(c.ServerIdentity.Address.ConnType()),
		"GoRoutines":  fmt.Sprintf("%v", runtime.NumGoroutine()),
	}}

	goverOnce.Do(func() {
		v, err := version.ReadExe(os.Args[0])
		if err == nil {
			gover = v
			goverOk = true
		}
	})

	if goverOk {
		st.Field["GoRelease"] = gover.Release
		st.Field["GoModuleInfo"] = gover.ModuleInfo
	}

	return st
}

// Close closes the overlay and the Router
func (c *Server) Close() error {
	c.Lock()
	if c.IsStarted {
		c.closeitChannel <- true
		c.IsStarted = false
	}
	c.Unlock()

	err := c.Router.Stop()
	if err != nil {
		err = xerrors.Errorf("stopping: %v", err)
		log.Error("While stopping router:", err)
	}
	c.WebSocket.stop()
	c.overlay.Close()
	err = c.serviceManager.closeDatabase()
	if err != nil {
		err = xerrors.Errorf("closing db: %v", err)
		log.Lvl3("Error closing database: " + err.Error())
	}
	log.Lvl3("Host Close", c.ServerIdentity.Address, "listening?", c.Router.Listening())
	return err
}

// Address returns the address used by the Router.
func (c *Server) Address() network.Address {
	return c.ServerIdentity.Address
}

// Service returns the service with the given name.
func (c *Server) Service(name string) Service {
	return c.serviceManager.service(name)
}

// GetService is kept for backward-compatibility.
func (c *Server) GetService(name string) Service {
	log.Warn("This method is deprecated - use `Server.Service` instead")
	return c.Service(name)
}

// ProtocolRegister will sign up a new protocol to this Server.
// It returns the ID of the protocol.
func (c *Server) ProtocolRegister(name string, protocol NewProtocol) (ProtocolID, error) {
	id, err := c.protocols.Register(name, protocol)
	if err != nil {
		return id, xerrors.Errorf("registering protocol: %v", err)
	}
	return id, nil
}

// protocolInstantiate instantiate a protocol from its ID
func (c *Server) protocolInstantiate(protoID ProtocolID, tni *TreeNodeInstance) (ProtocolInstance, error) {
	fn, ok := c.protocols.instantiators[c.protocols.ProtocolIDToName(protoID)]
	if !ok {
		return nil, xerrors.New("No protocol constructor with this ID")
	}
	pi, err := fn(tni)
	if err != nil {
		return nil, xerrors.Errorf("creating protocol: %v", err)
	}
	return pi, nil
}

// Start makes the router and the WebSocket listen on their respective
// ports. It returns once all servers are started.
func (c *Server) Start() {
	InformServerStarted()
	c.started = time.Now()
	if !c.Quiet {
		log.Lvlf1("Starting server at %s on address %s",
			c.started.Format("2006-01-02 15:04:05"),
			c.ServerIdentity.Address)
	}
	go c.Router.Start(c.isPeerValid)
	go c.WebSocket.start()
	for !c.Router.Listening() || !c.WebSocket.Listening() {
		time.Sleep(50 * time.Millisecond)
	}
	c.Lock()
	c.IsStarted = true
	c.Unlock()
	// Wait for closing of the channel
	<-c.closeitChannel
}

// StartInBackground starts the services and returns once everything
// is up and running.
func (c *Server) StartInBackground() {
	go c.Start()
	c.WaitStartup()
}

// WaitStartup can be called to ensure that the server is up and
// running. It will loop and wait 50 milliseconds between each
// test.
func (c *Server) WaitStartup() {
	for {
		c.Lock()
		s := c.IsStarted
		c.Unlock()
		if s {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// For all services that have `TestClose` defined, call it to make
// sure they are able to clean up. This should only be used for tests!
func (c *Server) callTestClose() {
	wg := sync.WaitGroup{}
	c.serviceManager.servicesMutex.Lock()
	for _, serv := range c.serviceManager.services {
		wg.Add(1)
		go func(s Service) {
			defer wg.Done()
			c, ok := s.(TestClose)
			if ok {
				c.TestClose()
			}
		}(serv)
	}
	c.serviceManager.servicesMutex.Unlock()
	wg.Wait()
}

// SetValidPeers sets the set of peers with which this server can communicate.
func (c *Server) SetValidPeers(peers []*network.ServerIdentity) {
	newPeers := make(map[network.ServerIdentityID]struct{})

	for _, peer := range peers {
		newPeers[peer.ID] = struct{}{}
	}

	c.validPeers.Store(newPeers)

	// FIXME: lower log priority -- used for debug for now
	log.LLvlf2("[%v @ %v] valid peers are now: %v)",
		c.ServerIdentity.ID, c.Address(), c.validPeers.Load())
}

// isPeerValid checks whether the given peer is valid for communication.
func (c *Server) isPeerValid(peer network.ServerIdentityID) bool {
	validPeersValue := c.validPeers.Load()

	// FIXME: lower log priority -- used for debug for now
	log.LLvlf2("[%v @ %v] check for %v (valid: %v)",
		c.ServerIdentity.ID, c.Address(), peer, validPeersValue)

	// Until the set of valid peers is initialized, all peers are valid
	if validPeersValue == nil {
		return true
	}

	validPeers := validPeersValue.(map[network.ServerIdentityID]struct{})
	_, ok := validPeers[peer]

	return ok
}

// Send sends a list of messages to the given ServerIdentity.
// Overrides Router.Send()
func (c *Server) Send(e *network.ServerIdentity,
	msgs ...network.Message) (uint64, error) {
	// FIXME: lower log priority -- used for debug for now
	log.LLvlf2("[%v @ %v] sending to %v",
		c.ServerIdentity.ID, c.Address(), e.ID)

	if !c.isPeerValid(e.ID) {
		return 0, xerrors.Errorf("%v rejecting send to %v: invalid peer",
			c.ServerIdentity.ID, e.ID)
	}

	return c.Router.Send(e, msgs...)
}
