// Package ciphersuite defines the interfaces that Onet needs to setup
// a secure channel between the conodes. It is built around a cipher suite
// interface that provides the cryptographic primitives.
//
// The package also provides a mock cipher suite that can be used for the
// tests but that is *not* secure per se.
//
// As a server could use multiple cipher suites, the package implements a
// cipher registry that takes an implementation of a cipher suite and
// registered using the name of the suite.
//
// Public keys and signatures may need to be transmitted over the network and
// interfaces cannot be used as is. That is why every the different elements
// can be packed as CipherData. The registry provides functions to unpack
// them as the structure is self-contained.
package ciphersuite

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"

	"golang.org/x/xerrors"
)

var sizeLength = 32 / 8

// Name is the type that can differentiate multiple ciphers.
type Name = string

// Nameable binds a structure to a cipher.
type Nameable interface {
	Name() Name
}

// CipherData is a self-contained message type that can be used
// over the network in the contrary of the interfaces.
type CipherData struct {
	Data       []byte
	CipherName Name
}

// Name returns the name of the cipher suite compatible with the data
// contained in the raw structure.
func (d *CipherData) Name() Name {
	return d.CipherName
}

func (d *CipherData) String() string {
	buf := append([]byte(d.Name()), d.Data...)
	return hex.EncodeToString(buf)
}

// Equal verifies if both self and other are deeply equal.
func (d *CipherData) Equal(other *CipherData) bool {
	return d.Name() == other.Name() && bytes.Equal(d.Data, other.Data)
}

// Clone returns a clone of the cipher data.
func (d *CipherData) Clone() *CipherData {
	data := make([]byte, len(d.Data))
	copy(data, d.Data)
	return &CipherData{
		CipherName: d.Name(),
		Data:       data,
	}
}

// WriteTo implements the io.WriteTo interface so that the cipher
// data can be written into any standard writer (e.g. hash).
func (d *CipherData) WriteTo(w io.Writer) (n int64, err error) {
	var size int
	size, err = w.Write([]byte(d.Name()))
	n += int64(size)
	if err != nil {
		return
	}

	size, err = w.Write(d.Data)
	n += int64(size)
	return
}

// MarshalText implements the encoding interface TextMarshaler so that
// it can be serialized in format such as TOML.
func (d *CipherData) MarshalText() ([]byte, error) {
	name := []byte(d.Name())
	size := make([]byte, sizeLength)
	binary.LittleEndian.PutUint32(size, uint32(len(name)))

	// Buffer starts with the size of the cipher suite name, then the name
	// and finally the data.
	data := append(append(size, name...), d.Data...)

	buf := make([]byte, hex.EncodedLen(len(data)))
	hex.Encode(buf, data)
	return buf, nil
}

// UnmarshalText implements the encoding interface TextUnmarshaler so that
// format such as TOML can deserialize the data.
func (d *CipherData) UnmarshalText(text []byte) error {
	buf := make([]byte, hex.DecodedLen(len(text)))
	_, err := hex.Decode(buf, text)
	if err != nil {
		return xerrors.Errorf("decoding hex: %v", err)
	}

	if len(buf) < sizeLength {
		return xerrors.Errorf("data is too small")
	}

	size := int(binary.LittleEndian.Uint32(buf[:sizeLength]))
	if len(buf) < sizeLength+size {
		return xerrors.Errorf("data is too small")
	}

	d.CipherName = string(buf[sizeLength : sizeLength+size])
	d.Data = buf[sizeLength+size:]
	return nil
}

// RawPublicKey is a raw data structure of a public key implementation.
type RawPublicKey struct {
	*CipherData
}

// NewRawPublicKey returns an instance of a public key.
func NewRawPublicKey(name Name, data []byte) *RawPublicKey {
	return &RawPublicKey{
		CipherData: &CipherData{
			Data:       data,
			CipherName: name,
		},
	}
}

// Raw returns the raw data of a public key. It is implemented to allow
// a raw public key to be compatible with the interface.
func (raw *RawPublicKey) Raw() *RawPublicKey {
	return raw
}

// Equal returns true when the two data structure contains the same public
// key.
func (raw *RawPublicKey) Equal(other PublicKey) bool {
	data := other.Raw()
	return data.CipherData.Equal(raw.CipherData)
}

// Clone returns a clone of the raw public key.
func (raw *RawPublicKey) Clone() *RawPublicKey {
	return &RawPublicKey{CipherData: raw.CipherData.Clone()}
}

// UnmarshalText converts the raw public key back from a text marshaling.
func (raw *RawPublicKey) UnmarshalText(text []byte) error {
	raw.CipherData = &CipherData{}
	return raw.CipherData.UnmarshalText(text)
}

// RawSecretKey is a raw data structure of a secret key implementation.
type RawSecretKey struct {
	*CipherData
}

// NewRawSecretKey returns an instance of a raw secret key.
func NewRawSecretKey(name Name, data []byte) *RawSecretKey {
	return &RawSecretKey{
		CipherData: &CipherData{
			CipherName: name,
			Data:       data,
		},
	}
}

// Raw returns the raw data of a secret key. It is implemented to allow
// a raw secret key to be compatible with the interface.
func (raw *RawSecretKey) Raw() *RawSecretKey {
	return raw
}

// Clone makes a clone of the secret key.
func (raw *RawSecretKey) Clone() *RawSecretKey {
	return &RawSecretKey{CipherData: raw.CipherData.Clone()}
}

// UnmarshalText converts the raw secret key back from a text marshaling.
func (raw *RawSecretKey) UnmarshalText(text []byte) error {
	raw.CipherData = &CipherData{}
	return raw.CipherData.UnmarshalText(text)
}

// RawSignature is a raw data structure of a signature implementation.
type RawSignature struct {
	*CipherData
}

// NewRawSignature returns an instance of a raw signature.
func NewRawSignature(name Name, data []byte) *RawSignature {
	return &RawSignature{
		CipherData: &CipherData{
			CipherName: name,
			Data:       data,
		},
	}
}

// Raw returns the raw data of a signature. It is implemented to allow
// a raw signature to be compatible with the interface.
func (raw *RawSignature) Raw() *RawSignature {
	return raw
}

// Clone returns a clone of a raw signature.
func (raw *RawSignature) Clone() *RawSignature {
	return &RawSignature{CipherData: raw.CipherData.Clone()}
}

// UnmarshalText converts the raw signature back from a text marshaling.
func (raw *RawSignature) UnmarshalText(text []byte) error {
	raw.CipherData = &CipherData{}
	return raw.CipherData.UnmarshalText(text)
}

// PublicKey represents one of the two sides of an asymmetric key pair
// which can be safely shared publicly.
type PublicKey interface {
	Nameable

	fmt.Stringer

	Raw() *RawPublicKey

	Equal(other PublicKey) bool
}

// SecretKey represents one of the two sides of an asymmetric key pair
// which must remain private.
type SecretKey interface {
	Nameable

	fmt.Stringer

	Raw() *RawSecretKey
}

// Signature represents a signature produced using a secret key and
// that can be verified with the associated public key.
type Signature interface {
	Nameable

	fmt.Stringer

	Raw() *RawSignature
}

// CipherSuite provides the primitive needed to create and verify
// signatures using an asymmetric key pair.
type CipherSuite interface {
	Nameable

	// PublicKey must return an implementation of a public key.
	PublicKey(raw *RawPublicKey) (PublicKey, error)

	// SecretKey must return an implementation of a secret key.
	SecretKey(raw *RawSecretKey) (SecretKey, error)

	// Signature must return an implementation of a signature.
	Signature(raw *RawSignature) (Signature, error)

	// KeyPair must return a random secret key and its associated public key.
	KeyPair() (PublicKey, SecretKey)

	// Sign must produce a signature that can be validated by the
	// associated public key of the secret key.
	Sign(sk SecretKey, msg []byte) (Signature, error)

	// Verify must return nil when the signature is valid for the
	// message and the public key. Otherwise it should return the
	// reason of the invalidity.
	Verify(pk PublicKey, signature Signature, msg []byte) error
}
