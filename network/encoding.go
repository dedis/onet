package network

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/big"
	"reflect"
	"sync"

	"github.com/dedis/crypto/abstract"
	"github.com/dedis/crypto/ed25519"
	"github.com/dedis/onet/log"
	"github.com/dedis/protobuf"
) /// Encoding part ///

// Suite used globally by this network library.
// For the moment, this will stay,as our focus is not on having the possibility
// to use any suite we want (the decoding stuff is much harder then, because we
// don't want to send the suite in the wire).
// It will surely change in futur releases so we can permit this behavior.
var Suite = ed25519.NewAES128SHA256Ed25519(false)

// Message is a type for any message that the user wants to send
type Message interface{}

// MessageID represents an unique identifier for each struct that must be
// marshalled/unmarshalled by the network library.
type MessageID uint32

const maxID = (1 << 32) - 1

// HashID is the hash function used to generate the MessageID out of a string
var HashID = sha256.New

// ErrorID is reserved by the network library. When you receive a message of
// ErrorID, it is generally because an error happened, then you can call
// Error() on it.
var ErrorID MessageID

// String returns the name of the structure if it is known, else it returns
// the hexadecimal value of the Id.
func (mId MessageID) String() string {
	if t := registry.msgType(mId); t != nil {
		return fmt.Sprintf("Message (%d) %s ", mId, t.String())
	}
	return fmt.Sprintf("Message %d", mId)
}

// Equal returns true if pId is equal to t
func (mId MessageID) Equal(t MessageID) bool {
	return mId == t
}

// NamespaceURL is the basic namespace used for uuid
// XXX should move that to external of the library as not every
// cothority/packages should be expected to use that.
const NamespaceURL = "https://dedis.epfl.ch/"

// NamespaceBodyType is the namespace used for PacketTypeID
const NamespaceBodyType = NamespaceURL + "/protocolType/"

var globalOrder = binary.BigEndian

// RegisterMessage registers any struct or ptr so it can be marshalled and
// unmarshalled by the network library. The ns parameters must uniquely identify
// this packet. It returns a MessageID generated from ns which is hashed with
// the HashID hash function and reduced to the MessageID's size.
func RegisterMessage(ns string, msg Message) MessageID {
	val := reflect.ValueOf(msg)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	t := val.Type()
	id := ID(ns)
	registry.put(id, t)
	return id
}

// MessageType returns a Message's MessageTypeID if registered or ErrorID if
// the message has not been registered with RegisterMessage().
func MessageType(msg Message) MessageID {
	msgType := reflect.TypeOf(msg)
	return registry.msgID(msgType)
}

// ID returns the MessageID identified by this name
func ID(name string) MessageID {
	h := HashID()
	h.Write([]byte(name))
	i := new(big.Int).SetBytes(h.Sum(nil))
	i.Mod(i, big.NewInt(maxID))
	id := i.Int64()
	if id > maxID {
		panic("something's wrong")
	}
	return MessageID(i.Int64())
}

// Marshal outputs the type and the byte representation of a structure.  It
// firsts marshals the type as a uuid,i.e.  a 16 byte length slice,then the
// struct encoded by protobuf.  That slice of bytes can be then decoded with
// Unmarshal.  msg must be a pointer to the message.
func Marshal(msg Message) ([]byte, error) {
	var msgID MessageID

	if msgID = MessageType(msg); msgID == ErrorID {
		return nil, fmt.Errorf("message %s not registered", reflect.TypeOf(msg))
	}
	b := new(bytes.Buffer)
	if err := binary.Write(b, globalOrder, msgID); err != nil {
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
// pointer.  The type must be registered to the network library in order to be
// decodable and the buffer must have been generated by Marshal otherwise it
// returns an error.
func Unmarshal(buf []byte) (MessageID, Message, error) {
	b := bytes.NewBuffer(buf)
	var tID MessageID
	if err := binary.Read(b, globalOrder, &tID); err != nil {
		return ErrorID, nil, err
	}
	typ := registry.msgType(tID)
	if typ == nil {
		return ErrorID, nil, fmt.Errorf("type %s not registered", tID.String())
	}
	ptrVal := reflect.New(typ)
	ptr := ptrVal.Interface()
	constructors := DefaultConstructors(Suite)
	if err := protobuf.DecodeWithConstructors(b.Bytes(), ptr, constructors); err != nil {
		return ErrorID, nil, err
	}
	return tID, ptrVal.Interface(), nil
}

// DumpTypes is used for debugging - it prints out all known types
func DumpTypes() {
	for t, m := range registry.types {
		log.Printf("ID %d => %s", t, m)
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

var registry = newTypeRegistry()

type typeRegistry struct {
	types map[MessageID]reflect.Type
	inv   map[reflect.Type]MessageID
	lock  sync.Mutex
}

func newTypeRegistry() *typeRegistry {
	return &typeRegistry{
		types: make(map[MessageID]reflect.Type),
		inv:   make(map[reflect.Type]MessageID),
		lock:  sync.Mutex{},
	}
}

// msgType returns the reflect.Type corresponding to the registered
// MessageTypeID. If the mid is not registered, it returns nil.
func (tr *typeRegistry) msgType(mid MessageID) reflect.Type {
	tr.lock.Lock()
	defer tr.lock.Unlock()
	return tr.types[mid]
}

func (tr *typeRegistry) msgID(t reflect.Type) MessageID {
	tr.lock.Lock()
	defer tr.lock.Unlock()
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return tr.inv[t]
}

// put stores the given type in the typeRegistry.
func (tr *typeRegistry) put(mid MessageID, typ reflect.Type) {
	tr.lock.Lock()
	defer tr.lock.Unlock()

	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	if t, ok := tr.types[mid]; ok && t != typ {
		panic(fmt.Sprintf("message id %d is already registered by %s", mid, t.String()))
	}
	tr.types[mid] = typ
	tr.inv[typ] = mid
}
