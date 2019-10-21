package onet

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"os"
	"path"
	"strconv"
	"sync"

	"go.dedis.ch/onet/v4/ciphersuite"
	"go.dedis.ch/onet/v4/log"
	"go.dedis.ch/onet/v4/network"
	bbolt "go.etcd.io/bbolt"
	"golang.org/x/xerrors"
	uuid "gopkg.in/satori/go.uuid.v1"
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

// NewServiceFunc is the type of a function that is used to instantiate a given Service
// A service is initialized with a Server (to send messages to someone).
type NewServiceFunc func(c *Context, suite ciphersuite.CipherSuite) (Service, error)

// ServiceID is a type to represent a uuid for a Service
type ServiceID uuid.UUID

// String returns the string representation of this ServiceID
func (s ServiceID) String() string {
	return uuid.UUID(s).String()
}

// Equal returns true if and only if s2 equals this ServiceID.
func (s ServiceID) Equal(s2 ServiceID) bool {
	return uuid.Equal(uuid.UUID(s), uuid.UUID(s2))
}

// IsNil returns true iff the ServiceID is Nil
func (s ServiceID) IsNil() bool {
	return s.Equal(ServiceID(uuid.Nil))
}

// NilServiceID is the empty ServiceID
var NilServiceID = ServiceID(uuid.Nil)

// GenericConfig is a config that can hold any type of specific configs for
// protocols. It is passed down to the service NewProtocol function.
type GenericConfig struct {
	Data []byte
}

// serviceManager is the place where all instantiated services are stored
// It gives access to: all the currently running services
type serviceManager struct {
	// the actual services
	services map[string]Service
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
	services := make(map[string]Service)
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

	log.Lvl3(srv.Address(), "instantiated all services")
	srv.statusReporterStruct.RegisterStatusReporter("Db", s)
	return s
}

func (s *serviceManager) register(suite ciphersuite.CipherSuite, name string, fn NewServiceFunc) error {
	ctx := newContext(name, s)
	srvc, err := fn(ctx, suite)
	if err != nil {
		return xerrors.Errorf("creating service: %v", err)
	}

	s.servicesMutex.Lock()
	s.services[name] = srvc
	s.servicesMutex.Unlock()
	s.server.WebSocket.registerService(name, srvc)

	return nil
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
	pub := s.server.ServerIdentity.PublicKey.String()
	return path.Join(s.dbPath, fmt.Sprintf("%x.db", pub))
}

func (s *serviceManager) dbFileName() string {
	pub := s.server.ServerIdentity.PublicKey
	h := sha256.New()
	pub.WriteTo(h)
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
	for name := range s.services {
		ret = append(ret, name)
	}
	return
}

// service returns the service implementation being registered to this name or
// nil if no service by this name is available.
func (s *serviceManager) service(name string) Service {
	s.servicesMutex.Lock()
	defer s.servicesMutex.Unlock()

	ser, ok := s.services[name]
	if !ok {
		log.Error("this service is not instantiated")
		return nil
	}
	return ser
}

func (s *serviceManager) serviceByID(id ServiceID) (Service, string) {
	s.servicesMutex.Lock()
	defer s.servicesMutex.Unlock()

	for name, service := range s.services {
		serviceID := ServiceID(uuid.NewV5(uuid.NamespaceURL, name))
		if serviceID.Equal(id) {
			return service, name
		}
	}

	return nil, ""
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
	si, _ := s.serviceByID(tni.Token().ServiceID)
	defaultHandle := func() (ProtocolInstance, error) { return s.server.protocolInstantiate(tni.Token().ProtoID, tni) }
	if si == nil {
		// let onet handle it
		return defaultHandle()
	}

	defer func() {
		if r := recover(); r != nil {
			pi = nil
			err = xerrors.Errorf("could not create new protocol: %v", r)
			return
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
