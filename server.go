package onet

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.dedis.ch/onet/v3/cfgpath"
	"go.dedis.ch/onet/v3/ciphersuite"
	"go.dedis.ch/onet/v3/log"
	"go.dedis.ch/onet/v3/network"
	"golang.org/x/xerrors"
	"rsc.io/goversion/version"
)

// Server connects the Router, the Overlay, and the Services together. It sets
// up everything and returns once a working network has been set up.
type Server struct {
	*network.Router

	// Our private-key
	secretKey ciphersuite.SecretKey
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
func newServer(cr *ciphersuite.Registry, dbPath string, r *network.Router, pkey *ciphersuite.RawSecretKey) *Server {
	delDb := false
	if dbPath == "" {
		dbPath = dbPathFromEnv()
		log.ErrFatal(os.MkdirAll(dbPath, 0750))
	} else {
		delDb = true
	}

	sk, err := cr.UnpackSecretKey(pkey)
	if err != nil {
		panic(err)
	}

	srv := &Server{
		secretKey:            sk,
		statusReporterStruct: newStatusReporterStruct(),
		Router:               r,
		protocols:            newProtocolStorage(),
		closeitChannel:       make(chan bool),
	}
	srv.overlay = NewOverlay(srv, cr)
	srv.WebSocket = NewWebSocket(r.ServerIdentity)
	srv.serviceManager = newServiceManager(srv, srv.overlay, dbPath, delDb)
	srv.statusReporterStruct.RegisterStatusReporter("Generic", srv)
	return srv
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
	go c.Router.Start()
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
