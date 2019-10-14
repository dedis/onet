// Package ciphersuite defines the cryptographic primitive that Onet needs. In order
// to register a cipher suite, it must comply to the interface.
//
// TODO: extend the doc
package ciphersuite

import (
	"encoding/hex"
	"reflect"
)

// Name is the type that can differentiate multiple ciphers.
type Name = string

// Nameable binds a structure to a cipher.
type Nameable interface {
	Name() Name
}

// CipherData is a self-contained message type that can be used
// over the network.
type CipherData struct {
	Data []byte
	Name Name
}

func (d *CipherData) String() string {
	return hex.EncodeToString(d.Data)
}

// Equal verifies if both self and other are deeply equal.
func (d *CipherData) Equal(other *CipherData) bool {
	return reflect.DeepEqual(d, other)
}

// Packable provides the primitives necessary to make network
// messages out of interfaces.
type Packable interface {
	Pack() (*CipherData, error)

	Unpack(data *CipherData) error
}

// PublicKey represents one of the two sides of an asymmetric key pair
// which can be safely shared publicly.
type PublicKey interface {
	Packable
	Nameable
}

// SecretKey represents one of the two sides of an asymmetric key pair
// which must remain private.
type SecretKey interface {
	Packable
	Nameable
}

// Signature represents a signature produced using a secret key and
// that can be verified with the associated public key.
type Signature interface {
	Packable
	Nameable
}

// CipherSuite provides the primitive needed to create and verify
// signatures using an asymmetric key pair.
type CipherSuite interface {
	Nameable

	PublicKey() PublicKey

	SecretKey() SecretKey

	Signature() Signature

	KeyPair() (PublicKey, SecretKey, error)

	Sign(sk SecretKey, msg []byte) (Signature, error)

	Verify(pk PublicKey, signature Signature, msg []byte) error
}
