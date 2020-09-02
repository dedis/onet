package network

import (
	"bytes"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/suites"
	"go.dedis.ch/kyber/v3/util/encoding"
	"go.dedis.ch/kyber/v3/util/key"
	"go.dedis.ch/onet/v3/log"
	"go.dedis.ch/protobuf"
	"golang.org/x/xerrors"
)

// MaxRetryConnect defines how many times we should try to connect.
const MaxRetryConnect = 5

// MaxIdentityExchange is the timeout for an identityExchange.
const MaxIdentityExchange = 5 * time.Second

// WaitRetry is the timeout on connection-setups.
const WaitRetry = 20 * time.Millisecond

// ErrClosed is when a connection has been closed.
var ErrClosed = xerrors.New("Connection Closed")

// ErrEOF is when the connection sends an EOF signal (mostly because it has
// been shut down).
var ErrEOF = xerrors.New("EOF")

// ErrCanceled means something went wrong in the sending or receiving part.
var ErrCanceled = xerrors.New("Operation Canceled")

// ErrTimeout is raised if the timeout has been reached.
var ErrTimeout = xerrors.New("Timeout Error")

// ErrUnknown is an unknown error.
var ErrUnknown = xerrors.New("Unknown Error")

// Size is a type to reprensent the size that is sent before every packet to
// correctly decode it.
type Size uint32

// Envelope is a container for any Message received through the network that
// contains the Message itself as well as some metadata such as the type and the
// sender. This is created by the network stack upon reception and is never
// transmitted.
type Envelope struct {
	// The ServerIdentity of the remote peer we are talking to.
	// Basically, this means that when you open a new connection to someone, and
	// or listen to incoming connections, the network library will already
	// make some exchange between the two communicants so each knows the
	// ServerIdentity of the others.
	ServerIdentity *ServerIdentity
	// What kind of msg do we have
	MsgType MessageTypeID
	// A *pointer* to the underlying message
	Msg Message
	// The length of the message in bytes
	Size Size
	// which constructors are used
	Constructors protobuf.Constructors
}

// ServerIdentity is used to represent a Server in the whole internet.
// It's based on a public key, and there can be one or more addresses to contact it.
type ServerIdentity struct {
	// This is the public key of that ServerIdentity
	Public kyber.Point
	// This is the configuration for the services
	ServiceIdentities []ServiceIdentity
	// The ServerIdentityID corresponding to that public key
	// Deprecated: use GetID
	ID ServerIdentityID
	// The address where that Id might be found
	Address Address
	// Description of the server
	Description string
	// This is the private key, may be nil. It is not exported so that it will never
	// be marshalled.
	private kyber.Scalar
	// The URL where the WebSocket interface can be found. (If not set, then default is http, on port+1.)
	// optional
	URL string `protobuf:"opt"`
}

// ServerIdentityID uniquely identifies an ServerIdentity struct
type ServerIdentityID uuid.UUID

// ServiceIdentity contains the identity of a service which is its public and
// private keys
type ServiceIdentity struct {
	Name    string
	Suite   string
	Public  kyber.Point
	private kyber.Scalar
}

// GetPrivate returns the private key of the service identity if available
func (sid *ServiceIdentity) GetPrivate() kyber.Scalar {
	return sid.private
}

// NewServiceIdentity creates a new identity
func NewServiceIdentity(name string, suite suites.Suite, public kyber.Point, private kyber.Scalar) ServiceIdentity {
	return ServiceIdentity{
		Name:    name,
		Suite:   suite.String(),
		Public:  public,
		private: private,
	}
}

// NewServiceIdentityFromPair creates a new identity using the provided key pair
func NewServiceIdentityFromPair(name string, suite suites.Suite, kp *key.Pair) ServiceIdentity {
	return NewServiceIdentity(name, suite, kp.Public, kp.Private)
}

// ServiceIdentities provides definitions to sort the array by service name
type ServiceIdentities []ServiceIdentity

func (srvids ServiceIdentities) Len() int {
	return len(srvids)
}

func (srvids ServiceIdentities) Swap(i, j int) {
	srvids[i], srvids[j] = srvids[j], srvids[i]
}

func (srvids ServiceIdentities) Less(i, j int) bool {
	return strings.Compare(srvids[i].Name, srvids[j].Name) == -1
}

// String returns a canonical representation of the ServerIdentityID.
func (eId ServerIdentityID) String() string {
	return uuid.UUID(eId).String()
}

// Equal returns true if both ServerIdentityID are equal or false otherwise.
func (eId ServerIdentityID) Equal(other ServerIdentityID) bool {
	return eId == other
}

// IsNil returns true iff the ServerIdentityID is Nil
func (eId ServerIdentityID) IsNil() bool {
	return eId.Equal(ServerIdentityID(uuid.Nil))
}

func (si *ServerIdentity) String() string {
	return si.Address.String()
}

// ServerIdentityType can be used to recognise an ServerIdentity-message
var ServerIdentityType = RegisterMessage(ServerIdentity{})

// ServerIdentityToml is the struct that can be marshalled into a toml file
type ServerIdentityToml struct {
	Public  string
	Address Address
}

// NewServerIdentity creates a new ServerIdentity based on a public key and with a slice
// of IP-addresses where to find that entity. The Id is based on a
// version5-UUID which can include a URL that is based on it's public key.
func NewServerIdentity(public kyber.Point, address Address) *ServerIdentity {
	si := &ServerIdentity{
		Public:  public,
		Address: address,
	}

	// compat for deprecated si.ID
	si.ID = si.GetID()
	return si
}

// GetID returns the ServerIdentityID corresponding to that public key
func (si ServerIdentity) GetID() ServerIdentityID {
	if si.Public == nil {
		return ServerIdentityID(uuid.Nil)
	}

	url := NamespaceURL + "id/" + si.Public.String()
	return ServerIdentityID(uuid.NewSHA1(uuid.NameSpaceURL, []byte(url)))
}

// Equal tests on same public key
func (si *ServerIdentity) Equal(e2 *ServerIdentity) bool {
	if si == nil || e2 == nil || si.Public == nil || e2.Public == nil {
		return false
	}
	return si.Public.Equal(e2.Public)
}

// SetPrivate sets a private key associated with this ServerIdentity.
// It will not be marshalled or output as Toml.
//
// Before calling NewTCPRouter for a TLS server, you must set the private
// key with SetPrivate.
func (si *ServerIdentity) SetPrivate(p kyber.Scalar) {
	si.private = p
}

// GetPrivate returns the private key set with SetPrivate.
func (si *ServerIdentity) GetPrivate() kyber.Scalar {
	return si.private
}

// ServicePublic returns the public key of the service or the default
// one if the service has not been registered with a suite
func (si *ServerIdentity) ServicePublic(name string) kyber.Point {
	for _, srvid := range si.ServiceIdentities {
		if srvid.Name == name {
			return srvid.Public
		}
	}

	return si.Public
}

// ServicePrivate returns the private key of the service or the default
// one if the service has not been registered with a suite
func (si *ServerIdentity) ServicePrivate(name string) kyber.Scalar {
	for _, srvid := range si.ServiceIdentities {
		if srvid.Name == name {
			return srvid.private
		}
	}

	return si.private
}

// HasServiceKeyPair returns true if the public and private keys are
// generated for the given service. The default key pair is ignored.
func (si *ServerIdentity) HasServiceKeyPair(name string) bool {
	for _, srvid := range si.ServiceIdentities {
		if srvid.Name == name && srvid.Public != nil && srvid.private != nil {
			return true
		}
	}

	return false
}

// HasServicePublic returns true if the public key is
// generated for the given service. The default public key is ignored.
func (si *ServerIdentity) HasServicePublic(name string) bool {
	for _, srvid := range si.ServiceIdentities {
		if srvid.Name == name && srvid.Public != nil {
			return true
		}
	}
	return false
}

// Toml converts an ServerIdentity to a Toml-structure
func (si *ServerIdentity) Toml(suite Suite) *ServerIdentityToml {
	var buf bytes.Buffer
	if err := encoding.WriteHexPoint(suite, &buf, si.Public); err != nil {
		log.Error("Error while writing public key:", err)
	}
	return &ServerIdentityToml{
		Address: si.Address,
		Public:  buf.String(),
	}
}

// ServerIdentity converts an ServerIdentityToml structure back to an ServerIdentity
func (si *ServerIdentityToml) ServerIdentity(suite Suite) *ServerIdentity {
	pub, err := encoding.ReadHexPoint(suite, strings.NewReader(si.Public))
	if err != nil {
		log.Error("Error while reading public key:", err)
	}
	return &ServerIdentity{
		Public:  pub,
		Address: si.Address,
	}
}

// GlobalBind returns the global-binding address. Given any IP:PORT combination,
// it will return ":PORT".
func GlobalBind(address string) (string, error) {
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", xerrors.Errorf("invalid address: %v", err)
	}
	return ":" + port, nil
}

// counterSafe is a struct that enables to update two counters Rx & Tx
// atomically that can be have increasing values.
// It's main use is for Conn to update how many bytes they've
// written / read. This struct implements the monitor.CounterIO interface.
type counterSafe struct {
	tx uint64
	rx uint64
	sync.Mutex
}

// Rx returns the rx counter
func (c *counterSafe) Rx() (out uint64) {
	c.Lock()
	out = c.rx
	c.Unlock()
	return
}

// Tx returns the tx counter
func (c *counterSafe) Tx() (out uint64) {
	c.Lock()
	out = c.tx
	c.Unlock()
	return
}

// updateRx adds delta to the rx counter
func (c *counterSafe) updateRx(delta uint64) {
	c.Lock()
	c.rx += delta
	c.Unlock()
}

// updateTx adds delta to the tx counter
func (c *counterSafe) updateTx(delta uint64) {
	c.Lock()
	c.tx += delta
	c.Unlock()
}
