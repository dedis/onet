package onet

import (
	"bytes"
	"encoding/binary"
	"sync"

	"go.dedis.ch/onet/v3/log"
	"go.dedis.ch/onet/v3/network"
	bbolt "go.etcd.io/bbolt"
	"golang.org/x/xerrors"
)

// Context represents the methods that are available to a service.
type Context struct {
	overlay           *Overlay
	server            *Server
	serviceID         ServiceID
	manager           *serviceManager
	bucketName        []byte
	bucketVersionName []byte
}

// defaultContext is the implementation of the Context interface. It is
// instantiated for each Service.
func newContext(c *Server, o *Overlay, servID ServiceID, manager *serviceManager) *Context {
	ctx := &Context{
		overlay:           o,
		server:            c,
		serviceID:         servID,
		manager:           manager,
		bucketName:        []byte(ServiceFactory.Name(servID)),
		bucketVersionName: []byte(ServiceFactory.Name(servID) + "version"),
	}
	err := manager.db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(ctx.bucketName)
		if err != nil {
			return xerrors.Errorf("creating bucket: %+v", err)
		}
		_, err = tx.CreateBucketIfNotExists(ctx.bucketVersionName)
		if err != nil {
			return xerrors.Errorf("creating bucket: %+v", err)
		}
		return nil
	})
	if err != nil {
		log.Panic("Failed to create bucket: " + err.Error())
	}
	return ctx
}

// NewTreeNodeInstance creates a TreeNodeInstance that is bound to a
// service instead of the Overlay.
func (c *Context) NewTreeNodeInstance(t *Tree, tn *TreeNode, protoName string) *TreeNodeInstance {
	io := c.overlay.protoIO.getByName(protoName)
	return c.overlay.NewTreeNodeInstanceFromService(t, tn, ProtocolNameToID(protoName), c.serviceID, io)
}

// SendRaw sends a message to the ServerIdentity.
func (c *Context) SendRaw(si *network.ServerIdentity, msg interface{}) error {
	_, err := c.server.Send(si, msg)
	if err != nil {
		xerrors.Errorf("sending message: %+v", err)
	}
	return nil
}

// ServerIdentity returns this server's identity.
func (c *Context) ServerIdentity() *network.ServerIdentity {
	return c.server.ServerIdentity
}

// Suite returns the suite for the context's associated server.
func (c *Context) Suite() network.Suite {
	return c.server.Suite()
}

// ServiceID returns the service-id.
func (c *Context) ServiceID() ServiceID {
	return c.serviceID
}

// CreateProtocol returns a ProtocolInstance bound to the service.
func (c *Context) CreateProtocol(name string, t *Tree) (ProtocolInstance, error) {
	pi, err := c.overlay.CreateProtocol(name, t, c.serviceID)
	if err != nil {
		return nil, xerrors.Errorf("creating protocol: %+v", err)
	}

	return pi, nil
}

// ProtocolRegister signs up a new protocol to this Server. Contrary go
// GlobalProtocolRegister, the protocol registered here is tied to that server.
// This is useful for simulations where more than one Server exists in the
// global namespace.
// It returns the ID of the protocol.
func (c *Context) ProtocolRegister(name string, protocol NewProtocol) (ProtocolID, error) {
	id, err := c.server.ProtocolRegister(name, protocol)
	if err != nil {
		return id, xerrors.Errorf("protocol registration: %+v", err)
	}
	return id, nil
}

// RegisterProtocolInstance registers a new instance of a protocol using overlay.
func (c *Context) RegisterProtocolInstance(pi ProtocolInstance) error {
	err := c.overlay.RegisterProtocolInstance(pi)
	if err != nil {
		return xerrors.Errorf("protocol instance regisration: %+v", err)
	}
	return nil
}

// ReportStatus returns all status of the services.
func (c *Context) ReportStatus() map[string]*Status {
	return c.server.statusReporterStruct.ReportStatus()
}

// RegisterStatusReporter registers a new StatusReporter.
func (c *Context) RegisterStatusReporter(name string, s StatusReporter) {
	c.server.statusReporterStruct.RegisterStatusReporter(name, s)
}

// RegisterProcessor overrides the RegisterProcessor methods of the Dispatcher.
// It delegates the dispatching to the serviceManager.
func (c *Context) RegisterProcessor(p network.Processor, msgType network.MessageTypeID) {
	c.manager.registerProcessor(p, msgType)
}

// RegisterProcessorFunc takes a message-type and a function that will be called
// if this message-type is received.
func (c *Context) RegisterProcessorFunc(msgType network.MessageTypeID, fn func(*network.Envelope) error) {
	c.manager.registerProcessorFunc(msgType, fn)
}

// RegisterMessageProxy registers a message proxy only for this server /
// overlay
func (c *Context) RegisterMessageProxy(m MessageProxy) {
	c.overlay.RegisterMessageProxy(m)
}

// Service returns the corresponding service.
func (c *Context) Service(name string) Service {
	return c.manager.service(name)
}

// String returns the host it's running on.
func (c *Context) String() string {
	return c.server.ServerIdentity.String()
}

var testContextData = struct {
	service map[string][]byte
	sync.Mutex
}{service: make(map[string][]byte, 0)}

// The ContextDB interface allows for easy testing in the services.
type ContextDB interface {
	Load(key []byte) (interface{}, error)
	LoadRaw(key []byte) ([]byte, error)
	LoadVersion() (int, error)
	SaveVersion(version int) error
}

// Save takes a key and an interface. The interface will be network.Marshal'ed
// and saved in the database under the bucket named after the service name.
//
// The data will be stored in a different bucket for every service.
func (c *Context) Save(key []byte, data interface{}) error {
	buf, err := network.Marshal(data)
	if err != nil {
		return xerrors.Errorf("marshaling: %+v", err)
	}
	err = c.manager.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(c.bucketName)
		return b.Put(key, buf)
	})
	if err != nil {
		return xerrors.Errorf("tx error: %+v", err)
	}
	return nil
}

// Load takes a key and returns the network.Unmarshaled data.
// Returns a nil value if the key does not exist.
func (c *Context) Load(key []byte) (interface{}, error) {
	var buf []byte
	err := c.manager.db.View(func(tx *bbolt.Tx) error {
		v := tx.Bucket(c.bucketName).Get(key)
		if v == nil {
			return nil
		}

		buf = make([]byte, len(v))
		copy(buf, v)
		return nil
	})
	if err != nil {
		return nil, xerrors.Errorf("tx error: %+v", err)
	}

	if buf == nil {
		return nil, nil
	}

	_, ret, err := network.Unmarshal(buf, c.server.suite)
	if err != nil {
		return nil, xerrors.Errorf("unmarshaling: %+v")
	}

	return ret, nil
}

// LoadRaw takes a key and returns the raw, unmarshalled data.
// Returns a nil value if the key does not exist.
func (c *Context) LoadRaw(key []byte) ([]byte, error) {
	var buf []byte
	err := c.manager.db.View(func(tx *bbolt.Tx) error {
		v := tx.Bucket(c.bucketName).Get(key)
		if v == nil {
			return nil
		}

		buf = make([]byte, len(v))
		copy(buf, v)
		return nil
	})
	if err != nil {
		return nil, xerrors.Errorf("tx error: %+v", err)
	}
	return buf, nil
}

var dbVersion = []byte("dbVersion")

// LoadVersion returns the version of the database, or 0 if
// no version has been found.
func (c *Context) LoadVersion() (int, error) {
	var buf []byte
	err := c.manager.db.View(func(tx *bbolt.Tx) error {
		v := tx.Bucket(c.bucketVersionName).Get(dbVersion)
		if v == nil {
			return nil
		}

		buf = make([]byte, len(v))
		copy(buf, v)
		return nil
	})

	if err != nil {
		return -1, xerrors.Errorf("tx error: %+v", err)
	}

	if len(buf) == 0 {
		return 0, nil
	}
	var version int32
	err = binary.Read(bytes.NewReader(buf), binary.LittleEndian, &version)
	if err != nil {
		return -1, xerrors.Errorf("bytes to int: %+v", err)
	}
	return int(version), nil
}

// SaveVersion stores the given version as the current database version.
func (c *Context) SaveVersion(version int) error {
	buf := bytes.NewBuffer(nil)
	err := binary.Write(buf, binary.LittleEndian, int32(version))
	if err != nil {
		return xerrors.Errorf("int to bytes: %+v", err)
	}
	err = c.manager.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(c.bucketVersionName)
		return b.Put(dbVersion, buf.Bytes())
	})
	if err != nil {
		return xerrors.Errorf("tx error: %+v")
	}
	return nil
}

// GetAdditionalBucket makes sure that a bucket with the given name
// exists, by eventually creating it, and returns the created bucket name,
// which is the servicename + "_" + the given name.
//
// This function should only be used if the Load and Save functions are not sufficient.
// Additionally, the user should not create buckets directly on the DB but always
// call this function to create new buckets to avoid bucket name conflicts.
func (c *Context) GetAdditionalBucket(name []byte) (*bbolt.DB, []byte) {
	// make a copy to insure c.bucketName is not written
	bucketName := make([]byte, len(c.bucketName))
	copy(bucketName, c.bucketName)

	fullName := append(append(bucketName, byte('_')), name...)
	err := c.manager.db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(fullName)
		if err != nil {
			return xerrors.Errorf("create bucket: %+v", err)
		}
		return nil
	})
	if err != nil {
		panic(xerrors.Errorf("tx error: %+v", err))
	}
	return c.manager.db, fullName
}
