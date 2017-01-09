package network

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"reflect"
	"sync"

	"github.com/dedis/crypto/abstract"
	"github.com/dedis/crypto/ed25519"
	"github.com/dedis/onet/log"
	"github.com/dedis/protobuf"
	"github.com/satori/go.uuid"
)

/// Encoding part ///

// Suite used globally by this network library.
// For the moment, this will stay,as our focus is not on having the possibility
// to use any suite we want (the decoding stuff is much harder then, because we
// don't want to send the suite in the wire).
// It will surely change in futur releases so we can permit this behavior.
var Suite = ed25519.NewAES128SHA256Ed25519(false)

// Message is a type for any message that the user wants to send
type Message interface{}

// MessageTypeID is the ID used to uniquely identify different registered messages
type MessageTypeID uuid.UUID

// ErrorType is reserved by the network library. When you receive a message of
// ErrorType, it is generally because an error happened, then you can call
// Error() on it.
var ErrorType = MessageTypeID(uuid.Nil)

// String returns the name of the structure if it is known, else it returns
// the hexadecimal value of the Id.
func (mId MessageTypeID) String() string {
	t, ok := registry.get(mId)
	if ok {
		return fmt.Sprintf("PTID(%s:%x)", t.String(), uuid.UUID(mId).Bytes())
	}
	return uuid.UUID(mId).String()
}

// Equal returns true if pId is equal to t
func (mId MessageTypeID) Equal(t MessageTypeID) bool {
	return bytes.Compare(uuid.UUID(mId).Bytes(), uuid.UUID(t).Bytes()) == 0
}

// NamespaceURL is the basic namespace used for uuid
// XXX should move that to external of the library as not every
// cothority/packages should be expected to use that.
const NamespaceURL = "https://dedis.epfl.ch/"

// NamespaceBodyType is the namespace used for PacketTypeID
const NamespaceBodyType = NamespaceURL + "/protocolType/"

// RegisterMessage registers any struct or ptr and returns the
// corresponding MessageTypeID. Once a struct is registered, it can be sent and
// received by the network library.
func RegisterMessage(msg Message) MessageTypeID {
	msgType := computeMessageType(msg)
	val := reflect.ValueOf(msg)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	t := val.Type()
	registry.put(msgType, t)
	return msgType
}

func computeMessageType(msg Message) MessageTypeID {
	val := reflect.ValueOf(msg)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	url := NamespaceBodyType + val.Type().String()
	u := uuid.NewV5(uuid.NamespaceURL, url)
	return MessageTypeID(u)
}

// MessageType returns a Message's MessageTypeID if registered or ErrorType if
// the message has not been registered with RegisterMessage().
func MessageType(msg Message) MessageTypeID {
	msgType := computeMessageType(msg)
	_, ok := registry.get(msgType)
	if !ok {
		return ErrorType
	}
	return msgType
}

// RTypeToMessageTypeID converts a reflect.Type to a MessageTypeID
func RTypeToMessageTypeID(msg reflect.Type) MessageTypeID {
	url := NamespaceBodyType + msg.String()
	return MessageTypeID(uuid.NewV5(uuid.NamespaceURL, url))
}

// DumpTypes is used for debugging - it prints out all known types
func DumpTypes() {
	for t, m := range registry.types {
		log.Print("Type", t, "has message", m)
	}
}

// DefaultConstructors gives a default constructor for protobuf out of the global suite
func DefaultConstructors(suite abstract.Suite) protobuf.Constructors {
	constructors := make(protobuf.Constructors)
	var point abstract.Point
	var secret abstract.Scalar
	constructors[reflect.TypeOf(&point).Elem()] = func() interface{} { return suite.Point() }
	constructors[reflect.TypeOf(&secret).Elem()] = func() interface{} { return suite.Scalar() }
	return constructors
}

// Error returns the error that has been encountered during the unmarshaling of
// this message.
func (env *Envelope) Error() error {
	return env.err
}

// SetError is workaround so we can set the error after creation of the
// application message
func (env *Envelope) SetError(err error) {
	env.err = err
}

type typeRegistry struct {
	types map[MessageTypeID]reflect.Type
	lock  sync.Mutex
}

func newTypeRegistry() *typeRegistry {
	return &typeRegistry{
		types: make(map[MessageTypeID]reflect.Type),
		lock:  sync.Mutex{},
	}
}

// get returns the reflect.Type corresponding to the registered PacketTypeID
// an a boolean indicating if the type is actually registered or not.
func (tr *typeRegistry) get(mid MessageTypeID) (reflect.Type, bool) {
	tr.lock.Lock()
	defer tr.lock.Unlock()
	t, ok := tr.types[mid]
	return t, ok
}

// put stores the given type in the typeRegistry.
func (tr *typeRegistry) put(mid MessageTypeID, typ reflect.Type) {
	tr.lock.Lock()
	defer tr.lock.Unlock()
	tr.types[mid] = typ
}

var registry = newTypeRegistry()

var globalOrder = binary.BigEndian

// Marshal marshals a struct with its respective type into a
// slice of bytes. That slice of bytes can be then decoded with
// Unmarshal. msg must be a pointer to the message.
func Marshal(msg Message) ([]byte, error) {
	var msgType MessageTypeID
	if msgType = MessageType(msg); msgType == ErrorType {
		return nil, fmt.Errorf("type of message %s not registered to the network library", reflect.TypeOf(msg))
	}
	b := new(bytes.Buffer)
	if err := binary.Write(b, globalOrder, msgType); err != nil {
		return nil, err
	}
	var buf []byte
	var err error
	if buf, err = protobuf.Encode(msg); err != nil {
		log.Errorf("Error for protobuf encoding: %s %+v", err, msg)
		if log.DebugVisible() > 0 {
			log.Error(log.Stack())
		}
		return nil, err
	}
	_, err = b.Write(buf)
	return b.Bytes(), err
}

// Unmarshal returns the type and the message out of a buffer. One can cast the
// resulting Message to a *pointer* of the underlying type,i.e. it returns a
// pointer.
// The type must be registered to the network library in order to be decodable
// and the buffer must have been generated by Marshal otherwise it returns an
// error.
func Unmarshal(buf []byte) (MessageTypeID, Message, error) {
	b := bytes.NewBuffer(buf)
	var tID MessageTypeID
	if err := binary.Read(b, globalOrder, &tID); err != nil {
		return ErrorType, nil, err
	}
	typ, ok := registry.get(tID)
	if !ok {
		return ErrorType, nil, fmt.Errorf("type %s not registered", tID.String())
	}
	ptrVal := reflect.New(typ)
	ptr := ptrVal.Interface()
	constructors := DefaultConstructors(Suite)
	if err := protobuf.DecodeWithConstructors(b.Bytes(), ptr, constructors); err != nil {
		return ErrorType, nil, err
	}
	return tID, ptrVal.Interface(), nil
}

// MarshalBinary returns the bytes representation of the envelope's message and
// message type.
func (env *Envelope) MarshalBinary() ([]byte, error) {
	return Marshal(env.Msg)
}

// UnmarshalBinary will decode the incoming bytes
// It uses protobuf for decoding (using the constructors in the Packet).
func (env *Envelope) UnmarshalBinary(buf []byte) error {
	t, msg, err := Unmarshal(buf)
	if err != nil {
		return err
	}
	env.MsgType = t
	env.Msg = msg
	return nil
}

// newEnvelope takes a Body and then constructs a
// Message from it. Error if the type is unknown
func newEnvelope(msg Message) (*Envelope, error) {
	val := reflect.ValueOf(msg)
	if val.Kind() != reflect.Ptr {
		return nil, fmt.Errorf("Expected a pointer to the message")
	}
	ty := MessageType(msg)
	if ty == ErrorType {
		return nil, fmt.Errorf("Packet to send is not known. Please register packet: %s",
			reflect.TypeOf(msg).String())
	}
	return &Envelope{
		MsgType: ty,
		Msg:     msg}, nil
}
