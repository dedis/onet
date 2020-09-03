package onet

import (
	"sync"

	"github.com/google/uuid"
	"go.dedis.ch/onet/v3/log"
	"go.dedis.ch/onet/v3/network"
	"golang.org/x/xerrors"
)

// ProtocolID uniquely identifies a protocol
type ProtocolID uuid.UUID

// String returns canonical string representation of the ID
func (pid ProtocolID) String() string {
	return uuid.UUID(pid).String()
}

// Equal returns true if and only if pid2 equals this ProtocolID.
func (pid ProtocolID) Equal(pid2 ProtocolID) bool {
	return pid == pid2
}

// IsNil returns true iff the ProtocolID is Nil
func (pid ProtocolID) IsNil() bool {
	return pid.Equal(ProtocolID(uuid.Nil))
}

// NewProtocol is the function-signature needed to instantiate a new protocol
type NewProtocol func(*TreeNodeInstance) (ProtocolInstance, error)

// ProtocolInstance is the interface that instances have to use in order to be
// recognized as protocols
type ProtocolInstance interface {
	// Start is called when a leader has created its tree configuration and
	// wants to start a protocol, it calls host.StartProtocol(protocolID), that
	// in turn instantiates a new protocol (with a fresh token), and then calls
	// Start on it.
	Start() error
	// Dispatch is called at the beginning by onet for listening on the channels
	Dispatch() error

	// ProcessProtocolMsg is a method that is called each time a message
	// arrives for this protocolInstance. TreeNodeInstance implements that
	// method for you using channels or handlers.
	ProcessProtocolMsg(*ProtocolMsg)
	// The token representing this ProtocolInstance
	Token() *Token
	// Shutdown cleans up the resources used by this protocol instance
	Shutdown() error
}

var protocols = newProtocolStorage()

// protocolStorage holds all protocols either globally or per-Server.
type protocolStorage struct {
	// Lock used because of the 'serverStarted' flag: it can be changed from a
	// call to 'Server.Start' and is checked when calling
	// 'GlobalProtocolRegister'.
	sync.Mutex
	// Instantiators maps the name of the protocols to the `NewProtocol`-
	// methods.
	instantiators map[string]NewProtocol
	// Flag indicating if a server has already started; here to avoid calls
	// to 'GlobalProtocolRegister' when a server has already started.
	serverStarted bool
}

// newProtocolStorage returns an initialized ProtocolStorage-struct.
func newProtocolStorage() *protocolStorage {
	return &protocolStorage{
		instantiators: map[string]NewProtocol{},
	}
}

// ProtocolIDToName returns the name to the corresponding protocolID.
func (ps *protocolStorage) ProtocolIDToName(id ProtocolID) string {
	ps.Lock()
	defer ps.Unlock()
	for n := range ps.instantiators {
		if id.Equal(ProtocolNameToID(n)) {
			return n
		}
	}
	return ""
}

// ProtocolExists returns whether a certain protocol already has been
// registered.
func (ps *protocolStorage) ProtocolExists(protoID ProtocolID) bool {
	name := ps.ProtocolIDToName(protoID)
	ps.Lock()
	_, ok := ps.instantiators[name]
	ps.Unlock()
	return ok
}

// Register takes a name and a NewProtocol and stores it in the structure.
// If the protocol already exists, a warning is printed and the NewProtocol is
// *not* stored.
func (ps *protocolStorage) Register(name string, protocol NewProtocol) (ProtocolID, error) {
	ps.Lock()
	defer ps.Unlock()
	id := ProtocolNameToID(name)
	if _, exists := ps.instantiators[name]; exists {
		return ProtocolID(uuid.Nil),
			xerrors.Errorf("Protocol -%s- already exists - not overwriting", name)
	}
	ps.instantiators[name] = protocol
	log.Lvl4("Registered", name, "to", id)
	return id, nil
}

// ProtocolNameToID returns the ProtocolID corresponding to the given name.
func ProtocolNameToID(name string) ProtocolID {
	url := network.NamespaceURL + "protocolname/" + name
	return ProtocolID(uuid.NewMD5(uuid.NameSpaceURL, []byte(url)))
}

// GlobalProtocolRegister registers a protocol in the global namespace.
// This is used in protocols that register themselves in the `init`-method.
// All registered protocols will be copied to every instantiated Server. If a
// protocol is tied to a service, use `Server.ProtocolRegisterName`
func GlobalProtocolRegister(name string, protocol NewProtocol) (ProtocolID, error) {
	protocols.Lock()
	// Cannot defer the "Unlock" because "Register" is using the lock too.
	if protocols.serverStarted {
		protocols.Unlock()
		panic("Cannot call 'GlobalProtocolRegister' when a server has already started.")
	}
	protocols.Unlock()
	id, err := protocols.Register(name, protocol)
	if err != nil {
		return id, xerrors.Errorf("registering protocol: %v", err)
	}
	return id, nil
}

// InformServerStarted allows to set the 'serverStarted' flag to true.
func InformServerStarted() {
	protocols.Lock()
	defer protocols.Unlock()
	protocols.serverStarted = true
}

// InformAllServersStopped allows to set the 'serverStarted' flag to false.
func InformAllServersStopped() {
	protocols.Lock()
	defer protocols.Unlock()
	protocols.serverStarted = false
}

// MessageProxy is an interface that allows one protocol to completely define its
// wire protocol format while still using the Overlay.
// Cothority sends different messages dynamically as slices of bytes, whereas
// Google proposes to use union-types:
// https://developers.google.com/protocol-buffers/docs/techniques#union
// This is a wrapper to enable union-types while still keeping compatibility with
// the dynamic cothority-messages. Implementations must provide methods to
// pass from the 'union-types' to 'cothority-dynamic-messages' with the Wrap
// and Unwrap method.
// A default one is provided with defaultMessageProxy so the regular wire-format
// protocol can still be used.
type MessageProxy interface {
	// Wrap takes a message and the overlay information and returns the message
	// that has to be sent directly to the network alongside with any error that
	// happened.
	// If msg is nil, it is only an internal message of the Overlay.
	Wrap(msg interface{}, info *OverlayMsg) (interface{}, error)
	// Unwrap takes the message coming from the network and returns the
	// inner message that is going to be dispatched to the ProtocolInstance, the
	// OverlayMessage needed by the Overlay to function correctly and then any
	// error that might have occurred.
	Unwrap(msg interface{}) (interface{}, *OverlayMsg, error)
	// PacketType returns the packet type ID that this Protocol expects from the
	// network. This is needed in order for the Overlay to receive those
	// messages and dispatch them to the correct MessageProxy.
	PacketType() network.MessageTypeID
	// Name returns the name associated with this MessageProxy. When creating a
	// protocol, if one use a name used by a MessageProxy, this MessageProxy will be
	// used to Wrap and Unwrap messages.
	Name() string
}

// NewMessageProxy is a function typedef to instantiate a new MessageProxy.
type NewMessageProxy func() MessageProxy

type messageProxyFactoryStruct struct {
	factories []NewMessageProxy
}

// RegisterMessageProxy stores the message proxy creation function
func (mpfs *messageProxyFactoryStruct) RegisterMessageProxy(n NewMessageProxy) {
	mpfs.factories = append(mpfs.factories, n)
}

var messageProxyFactory = messageProxyFactoryStruct{}

// RegisterMessageProxy saves a new NewMessageProxy under its name.
// When a Server is instantiated, all MessageProxys will be generated and stored
// for this Server.
func RegisterMessageProxy(n NewMessageProxy) {
	messageProxyFactory.RegisterMessageProxy(n)
}

// messageProxyStore contains all created MessageProxys. It contains the default
// MessageProxy used by the Overlay for backwards-compatibility.
type messageProxyStore struct {
	sync.Mutex
	protos    []MessageProxy
	defaultIO MessageProxy
}

// RegisterMessageProxy saves directly the given MessageProxy. It's useful if
// one wants different message proxy per server/overlay.
func (p *messageProxyStore) RegisterMessageProxy(mp MessageProxy) {
	if p.getByName(mp.Name()) == p.defaultIO {
		return
	}
	p.Lock()
	defer p.Unlock()
	p.protos = append(p.protos, mp)
}

func (p *messageProxyStore) getByName(name string) MessageProxy {
	p.Lock()
	defer p.Unlock()
	for _, pio := range p.protos {
		if pio.Name() == name {
			return pio
		}
	}
	return p.defaultIO
}

func (p *messageProxyStore) getByPacketType(mid network.MessageTypeID) MessageProxy {
	p.Lock()
	defer p.Unlock()
	for _, pio := range p.protos {
		if pio.PacketType().Equal(mid) {
			return pio
		}
	}
	return p.defaultIO
}

func newMessageProxyStore(s network.Suite, disp network.Dispatcher, proc network.Processor) *messageProxyStore {
	pstore := &messageProxyStore{
		// also add the default one
		defaultIO: &defaultProtoIO{s},
	}
	for name, newIO := range messageProxyFactory.factories {
		io := newIO()
		pstore.protos = append(pstore.protos, io)
		disp.RegisterProcessor(proc, io.PacketType())
		log.Lvl3("Instantiating MessageProxy", name, "at position", len(pstore.protos))
	}
	return pstore
}
