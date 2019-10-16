// Package ciphersuite defines the cryptographic primitive that Onet needs. In order
// to register a cipher suite, it must comply to the interface.
//
// TODO: extend the doc
package ciphersuite

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
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

// WriteTo implements the io.WriteTo interface so that the cipher
// data can be written into any standard writer (e.g. hash).
func (d *CipherData) WriteTo(w io.Writer) (n int64, err error) {
	var size int
	size, err = w.Write([]byte(d.Name))
	n += int64(size)
	if err != nil {
		return
	}

	size, err = w.Write(d.Data)
	n += int64(size)
	return
}

// MarshalText implements the encoding interface TextMarshaler.
func (d *CipherData) MarshalText() ([]byte, error) {
	name := []byte(d.Name)
	size := make([]byte, 4)
	binary.LittleEndian.PutUint32(size, uint32(len(name)))

	data := append(append(size, name...), d.Data...)

	buf := make([]byte, hex.EncodedLen(len(data)))
	hex.Encode(buf, data)
	return buf, nil
}

// UnmarshalText implements the encoding interface TextUnmarshaler.
func (d *CipherData) UnmarshalText(text []byte) error {
	buf := make([]byte, hex.DecodedLen(len(text)))
	hex.Decode(buf, text)

	size := binary.LittleEndian.Uint32(buf[:4])
	d.Name = string(buf[4 : 4+size])
	d.Data = buf[4+size:]
	return nil
}

// Packable provides the primitives necessary to make network
// messages out of interfaces.
type Packable interface {
	Pack() *CipherData

	Unpack(data *CipherData) error
}

// PublicKey represents one of the two sides of an asymmetric key pair
// which can be safely shared publicly.
type PublicKey interface {
	Packable
	Nameable
	fmt.Stringer
}

// SecretKey represents one of the two sides of an asymmetric key pair
// which must remain private.
type SecretKey interface {
	Packable
	Nameable
	fmt.Stringer
}

// Signature represents a signature produced using a secret key and
// that can be verified with the associated public key.
type Signature interface {
	Packable
	Nameable
	fmt.Stringer
}

// CipherSuite provides the primitive needed to create and verify
// signatures using an asymmetric key pair.
type CipherSuite interface {
	Nameable

	PublicKey() PublicKey

	SecretKey() SecretKey

	Signature() Signature

	KeyPair() (PublicKey, SecretKey)

	Sign(sk SecretKey, msg []byte) (Signature, error)

	Verify(pk PublicKey, signature Signature, msg []byte) error
}
