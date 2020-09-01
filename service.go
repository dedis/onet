package onet

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"os"
	"path"
	"strconv"
	"sync"

	"github.com/google/uuid"
	"go.dedis.ch/kyber/v3/suites"
	"go.dedis.ch/kyber/v3/util/key"
	"go.dedis.ch/onet/v3/log"
	"go.dedis.ch/onet/v3/network"
	bbolt "go.etcd.io/bbolt"
	"golang.org/x/xerrors"
)

func init() {
	network.RegisterMessage(GenericConfig{})
}

// Service is a generic interface to define any type of services.
// A Service has multiple roles:
// * Processing websocket client requests with ProcessClientRequests
// * Handling onet information to ProtocolInstances created with
//  	NewProtocol
// * Handling any kind of messages between Services between different hosts with
//   	the Processor interface
type Service interface {
	// NewProtocol is called upon a ProtocolInstance's first message when Onet needs
	// to instantiate the protocol. A Service is expected to manually create
	// the ProtocolInstance it is using. If a Service returns (nil,nil), that
	// means this Service lets Onet handle the protocol instance.
	NewProtocol(*TreeNodeInstance, *GenericConfig) (ProtocolInstance, error)
	// ProcessClientRequest is called when a message from an external
	// client is received by the websocket for this service. The message is
	// forwarded to the corresponding handler keyed by the path. If the
	// handler is a normal one, i.e., a request-response handler, it
	// returns a message in the first return value and the second
	// (StreamingTunnel) will be set to nil. If the handler is a streaming
	// handler, the first return value is set to nil but the second
	// (StreamingTunnel) will exist. It should be used to stream messages
	// to the client. See the StreamingTunnel documentation on how it
	// should be used. The returned error will be formatted as a websocket
	// error code 4000, using the string form of the error as the message.
	ProcessClientRequest(req *http.Request, handler string, msg []byte) (reply []byte, tunnel *StreamingTunnel, err error)
	// Processor makes a Service being able to handle any kind of packets
	// directly from the network. It is used for inter service communications,
	// which are mostly single packets with no or little interactions needed. If
	// a complex logic is used for these messages, it's best to put that logic
	// into a ProtocolInstance that the Service will launch, since there's nicer
	// utilities for ProtocolInstance.
	network.Processor
}

// BidirectionalStreamer specifies the functions needed to handle a
// bi-directional streamer, where the client is able to use the same chanel in
// order to send multiple requests.
type BidirectionalStreamer interface {
	// ProcessClientStreamRequest is different from ProcessClientRequest in that
	// it takes a chanel of inputs and watches for additional inputs. Additional
	// inputs are then forwarded to the service.
	ProcessClientStreamRequest(req *http.Request, path string, clientInputs chan []byte) (chan []byte, error)
	// IsStreaming checks if the handler registered at the given path is a
	// streaming handler or not. It returns an error in the case the handler is
	// not found.
	IsStreaming(path string) (bool, error)
}

// NewServiceFunc is the type of a function that is used to instantiate a given Service
// A service is initialized with a Server (to send messages to someone).
type NewServiceFunc func(c *Context) (Service, error)

// ServiceID is a type to represent a uuid for a Service
type ServiceID uuid.UUID

// String returns the string representation of this ServiceID
func (s ServiceID) String() string {
	return uuid.UUID(s).String()
}

// Equal returns true if and only if s2 equals this ServiceID.
func (s ServiceID) Equal(s2 ServiceID) bool {
	return s == s2
}

// IsNil returns true iff the ServiceID is Nil
func (s ServiceID) IsNil() bool {
	return uuid.UUID(s) == uuid.Nil
}

// NilServiceID is the empty ServiceID
var NilServiceID = ServiceID(uuid.Nil)

// GenericConfig is a config that can hold any type of specific configs for
// protocols. It is passed down to the service NewProtocol function.
type GenericConfig struct {
	Data []byte
}

// A serviceFactory is used to register a NewServiceFunc
type serviceFactory struct {
	constructors []serviceEntry
	mutex        sync.RWMutex
}

// A serviceEntry holds all references to a service
type serviceEntry struct {
	constructor NewServiceFunc
	serviceID   ServiceID
	name        string
	suite       suites.Suite
}

// ServiceFactory is the global service factory to instantiate Services
var ServiceFactory = serviceFactory{
	constructors: []serviceEntry{},
}

// Register takes a name and a function, then creates a ServiceID out of it and stores the
// mapping and the creation function.
//
// A suite can be provided to override the default one
func (s *serviceFactory) Register(name string, suite suites.Suite, fn NewServiceFunc) (ServiceID, error) {
	if !s.ServiceID(name).Equal(NilServiceID) {
		return NilServiceID, xerrors.Errorf("service %s already registered", name)
	}
	id := ServiceID(uuid.NewSHA1(uuid.NameSpaceURL, []byte(name)))
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.constructors = append(s.constructors, serviceEntry{
		constructor: fn,
		serviceID:   id,
		name:        name,
		suite:       suite,
	})
	return id, nil
}

// Unregister - mainly for tests
func (s *serviceFactory) Unregister(name string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	index := -1
	for i, c := range s.constructors {
		if c.name == name {
			index = i
			break
		}
	}
	if index < 0 {
		return xerrors.New("Didn't find service " + name)
	}
	s.constructors = append(s.constructors[:index], s.constructors[index+1:]...)
	return nil
}

// RegisterNewService is a wrapper around service factory to register
// a service with the default suite
func RegisterNewService(name string, fn NewServiceFunc) (ServiceID, error) {
	id, err := ServiceFactory.Register(name, nil, fn)
	if err != nil {
		return id, xerrors.Errorf("register service: %v", err)
	}
	return id, nil
}

// RegisterNewServiceWithSuite is wrapper around service factory to register
// a service with a given suite
func RegisterNewServiceWithSuite(name string, suite suites.Suite, fn NewServiceFunc) (ServiceID, error) {
	id, err := ServiceFactory.Register(name, suite, fn)
	if err != nil {
		return id, xerrors.Errorf("register service: %v", err)
	}
	return id, nil
}

// UnregisterService removes a service from the global pool.
func UnregisterService(name string) error {
	err := ServiceFactory.Unregister(name)
	if err != nil {
		return xerrors.Errorf("register service: %v", err)
	}
	return nil
}

// registeredServiceIDs returns all the services registered
func (s *serviceFactory) registeredServiceIDs() []ServiceID {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	var ids = make([]ServiceID, 0, len(s.constructors))
	for _, c := range s.constructors {
		ids = append(ids, c.serviceID)
	}
	return ids
}

// generateKeyPairs generates the key pairs for the services that
// have a suite registered with them. Other ones will use the default
// suite and the associated key pair.
func (s *serviceFactory) generateKeyPairs(si *network.ServerIdentity) {
	services := []network.ServiceIdentity{}
	for _, name := range ServiceFactory.RegisteredServiceNames() {
		suite := ServiceFactory.Suite(name)
		if suite != nil {
			pair := key.NewKeyPair(suite)
			sid := network.NewServiceIdentityFromPair(name, suite, pair)

			services = append(services, sid)
		}
	}
	si.ServiceIdentities = services
}

// RegisteredServiceNames returns all the names of the services registered
func (s *serviceFactory) RegisteredServiceNames() []string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	var names = make([]string, 0, len(s.constructors))
	for _, n := range s.constructors {
		names = append(names, n.name)
	}
	return names
}

// ServiceID returns the ServiceID out of the name of the service
func (s *serviceFactory) ServiceID(name string) ServiceID {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	for _, c := range s.constructors {
		if name == c.name {
			return c.serviceID
		}
	}
	return NilServiceID
}

// Suite returns the suite registered with the service or nil
func (s *serviceFactory) Suite(name string) suites.Suite {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	for _, c := range s.constructors {
		if name == c.name {
			return c.suite
		}
	}

	return nil
}

// SuiteByID returns the suite registered with the service or nil
// using the generated service ID
func (s *serviceFactory) SuiteByID(id ServiceID) suites.Suite {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	for _, c := range s.constructors {
		if id == c.serviceID {
			return c.suite
		}
	}

	return nil
}

// Name returns the Name out of the ID
func (s *serviceFactory) Name(id ServiceID) string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	for _, c := range s.constructors {
		if id.Equal(c.serviceID) {
			return c.name
		}
	}
	return ""
}

// start launches a new service
func (s *serviceFactory) start(name string, con *Context) (Service, error) {
	// Checks if we need a key pair and if it is available
	suite := s.Suite(name)
	if suite != nil && !con.ServerIdentity().HasServiceKeyPair(name) {
		return nil, xerrors.Errorf("Service `%s` requires a key pair. "+
			"Use the interactive setup to generate a new file that will include this service.", name)
	}

	s.mutex.RLock()
	defer s.mutex.RUnlock()
	for _, c := range s.constructors {
		if name == c.name {
			service, err := c.constructor(con)
			if err != nil {
				return nil, xerrors.Errorf("creating service: %v", err)
			}
			return service, nil
		}
	}
	return nil, xerrors.New("Didn't find service " + name)
}

// serviceManager is the place where all instantiated services are stored
// It gives access to: all the currently running services
type serviceManager struct {
	// the actual services
	services map[ServiceID]Service
	// making sure we're not racing for services
	servicesMutex sync.Mutex
	// the onet host
	server *Server
	// a bbolt database for all services
	db     *bbolt.DB
	dbPath string
	// should the db be deleted on close?
	delDb bool
	// the dispatcher can take registration of Processors
	network.Dispatcher
}

// newServiceManager will create a serviceStore out of all the registered Service
func newServiceManager(srv *Server, o *Overlay, dbPath string, delDb bool) *serviceManager {
	services := make(map[ServiceID]Service)
	s := &serviceManager{
		services:   services,
		server:     srv,
		dbPath:     dbPath,
		delDb:      delDb,
		Dispatcher: network.NewRoutineDispatcher(),
	}

	s.updateDbFileName()

	db, err := openDb(s.dbFileName())
	if err != nil {
		log.Panic("Failed to create new database: " + err.Error())
	}
	s.db = db

	for name, inst := range protocols.instantiators {
		log.Lvl4("Registering global protocol", name)
		srv.ProtocolRegister(name, inst)
	}

	ids := ServiceFactory.registeredServiceIDs()
	for _, id := range ids {
		name := ServiceFactory.Name(id)
		log.Lvl3("Starting service", name)

		cont := newContext(srv, o, id, s)

		srvc, err := ServiceFactory.start(name, cont)
		if err != nil {
			log.Fatalf("Trying to instantiate service %v: %+v", name, err)
		}
		log.Lvl3("Started Service", name)
		s.servicesMutex.Lock()
		services[id] = srvc
		s.servicesMutex.Unlock()
		srv.WebSocket.registerService(name, srvc)
	}
	log.Lvl3(srv.Address(), "instantiated all services")
	srv.statusReporterStruct.RegisterStatusReporter("Db", s)
	return s
}

// openDb opens a database at `path`. It creates the database if it does not exist.
// The caller must ensure that all parent directories exist.
func openDb(path string) (*bbolt.DB, error) {
	db, err := bbolt.Open(path, 0600, nil)
	if err != nil {
		return nil, xerrors.Errorf("opening db: %v", err)
	}
	return db, nil
}

func (s *serviceManager) dbFileNameOld() string {
	pub, _ := s.server.ServerIdentity.Public.MarshalBinary()
	return path.Join(s.dbPath, fmt.Sprintf("%x.db", pub))
}

func (s *serviceManager) dbFileName() string {
	pub, _ := s.server.ServerIdentity.Public.MarshalBinary()
	h := sha256.New()
	h.Write(pub)
	return path.Join(s.dbPath, fmt.Sprintf("%x.db", h.Sum(nil)))
}

// updateDbFileName checks if the old database file name exists, if it does, it
// will rename it to the new file name.
func (s *serviceManager) updateDbFileName() {
	if _, err := os.Stat(s.dbFileNameOld()); err == nil {
		// we assume the new name does not exist
		log.Lvl2("Renaming database from", s.dbFileNameOld(), "to", s.dbFileName())
		if err := os.Rename(s.dbFileNameOld(), s.dbFileName()); err != nil {
			log.Error(err)
		}
	}
}

// Process implements the Processor interface: service manager will relay
// messages to the right Service.
func (s *serviceManager) Process(env *network.Envelope) {
	// will launch a go routine for that message
	s.Dispatch(env)
}

// closeDatabase closes the database.
// It also removes the database file if the path is not default (i.e. testing config)
func (s *serviceManager) closeDatabase() error {
	if s.db != nil {
		err := s.db.Close()
		if err != nil {
			log.Error("Close database failed with: " + err.Error())
		}
	}

	if s.delDb {
		err := os.Remove(s.dbFileName())
		if err != nil {
			return xerrors.Errorf("removing file: %v", err)
		}
	}
	return nil
}

// GetStatus is a function that returns the status report of the server.
func (s *serviceManager) GetStatus() *Status {
	if s.db == nil {
		return &Status{Field: map[string]string{"Open": "false"}}
	}
	st := s.db.Stats()
	return &Status{Field: map[string]string{
		"Open":             "true",
		"FreePageN":        strconv.Itoa(st.FreePageN),
		"PendingPageN":     strconv.Itoa(st.PendingPageN),
		"FreeAlloc":        strconv.Itoa(st.FreeAlloc),
		"FreelistInuse":    strconv.Itoa(st.FreelistInuse),
		"TxN":              strconv.Itoa(st.TxN),
		"OpenTxN":          strconv.Itoa(st.OpenTxN),
		"Tx.PageCount":     strconv.Itoa(st.TxStats.PageCount),
		"Tx.PageAlloc":     strconv.Itoa(st.TxStats.PageAlloc),
		"Tx.CursorCount":   strconv.Itoa(st.TxStats.CursorCount),
		"Tx.NodeCount":     strconv.Itoa(st.TxStats.NodeCount),
		"Tx.NodeDeref":     strconv.Itoa(st.TxStats.NodeDeref),
		"Tx.Rebalance":     strconv.Itoa(st.TxStats.Rebalance),
		"Tx.RebalanceTime": st.TxStats.RebalanceTime.String(),
		"Tx.Split":         strconv.Itoa(st.TxStats.Split),
		"Tx.Spill":         strconv.Itoa(st.TxStats.Spill),
		"Tx.SpillTime":     st.TxStats.SpillTime.String(),
		"Tx.Write":         strconv.Itoa(st.TxStats.Write),
		"Tx.WriteTime":     st.TxStats.WriteTime.String(),
	}}
}

// registerProcessor the processor to the service manager and tells the host to dispatch
// this message to the service manager. The service manager will then dispatch
// the message in a go routine. XXX This is needed because we need to have
// messages for service dispatched in asynchronously regarding the protocols.
// This behavior with go routine is fine for the moment but for better
// performance / memory / resilience, it may be changed to a real queuing
// system later.
func (s *serviceManager) registerProcessor(p network.Processor, msgType network.MessageTypeID) {
	// delegate message to host so the host will pass the message to ourself
	s.server.RegisterProcessor(s, msgType)
	// handle the message ourselves (will be launched in a go routine)
	s.Dispatcher.RegisterProcessor(p, msgType)
}

func (s *serviceManager) registerProcessorFunc(msgType network.MessageTypeID, fn func(*network.Envelope) error) {
	// delegate message to host so the host will pass the message to ourself
	s.server.RegisterProcessor(s, msgType)
	// handle the message ourselves (will be launched in a go routine)
	s.Dispatcher.RegisterProcessorFunc(msgType, fn)

}

// availableServices returns a list of all services available to the serviceManager.
// If no services are instantiated, it returns an empty list.
func (s *serviceManager) availableServices() (ret []string) {
	s.servicesMutex.Lock()
	defer s.servicesMutex.Unlock()
	for id := range s.services {
		ret = append(ret, ServiceFactory.Name(id))
	}
	return
}

// service returns the service implementation being registered to this name or
// nil if no service by this name is available.
func (s *serviceManager) service(name string) Service {
	id := ServiceFactory.ServiceID(name)
	if id.Equal(NilServiceID) {
		return nil
	}
	s.servicesMutex.Lock()
	defer s.servicesMutex.Unlock()
	ser, ok := s.services[id]
	if !ok {
		log.Error("this service is not instantiated")
		return nil
	}
	return ser
}

func (s *serviceManager) serviceByID(id ServiceID) (Service, bool) {
	var serv Service
	var ok bool
	s.servicesMutex.Lock()
	defer s.servicesMutex.Unlock()
	if serv, ok = s.services[id]; !ok {
		return nil, false
	}
	return serv, true
}

// newProtocol contains the logic of how and where a ProtocolInstance is
// created. If the token's ServiceID is nil, then onet handles the creation of
// the PI. If the corresponding service returns (nil,nil), then onet handles
// the creation of the PI. Otherwise the service is responsible for setting up
// the PI.
func (s *serviceManager) newProtocol(tni *TreeNodeInstance, config *GenericConfig) (pi ProtocolInstance, err error) {
	if s.server.Closed() {
		err = xerrors.New("will not pass protocol once the server is closed")
		return
	}
	si, ok := s.serviceByID(tni.Token().ServiceID)
	defaultHandle := func() (ProtocolInstance, error) { return s.server.protocolInstantiate(tni.Token().ProtoID, tni) }
	if !ok {
		// let onet handle it
		return defaultHandle()
	}

	defer func() {
		if r := recover(); r != nil {
			pi = nil
			err = xerrors.Errorf("could not create new protocol: %v at %s",
				r, log.Stack())
		}
	}()

	pi, err = si.NewProtocol(tni, config)
	if pi == nil && err == nil {
		return defaultHandle()
	}
	if err != nil {
		err = xerrors.Errorf("creating protocol: %v", err)
	}
	return
}
