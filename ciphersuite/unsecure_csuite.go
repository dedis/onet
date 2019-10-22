package ciphersuite

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"

	"golang.org/x/xerrors"
)

const nonceLength = 16

// UnsecureCipherSuiteName is the reference name for the cipher suite
// intended to be used for testing.
var UnsecureCipherSuiteName = "CIPHER_SUITE_UNSECURE"

// UnsecurePublicKey is the public key implementation for the insecure
// cipher.
type UnsecurePublicKey struct {
	data []byte
}

func newUnsecurePublicKey() *UnsecurePublicKey {
	data := make([]byte, nonceLength)
	rand.Read(data)
	return &UnsecurePublicKey{data}
}

// Name returns the name of the unsecure cipher suite.
func (pk *UnsecurePublicKey) Name() Name {
	return UnsecureCipherSuiteName
}

// Pack returns the cipher data of the public key.
func (pk *UnsecurePublicKey) Pack() *CipherData {
	return &CipherData{Data: pk.data, Name: pk.Name()}
}

// Unpack tries to convert the cipher data back to a public key.
func (pk *UnsecurePublicKey) Unpack(p *CipherData) error {
	if len(p.Data) == 0 {
		return xerrors.New("empty data")
	}

	pk.data = p.Data
	return nil
}

func (pk *UnsecurePublicKey) String() string {
	return hex.EncodeToString(pk.data)
}

// UnsecureSecretKey is the secret key implementation of the unsecure
// cipher.
type UnsecureSecretKey struct {
	data []byte
}

func newUnsecureSecretKey() *UnsecureSecretKey {
	data := make([]byte, nonceLength)
	rand.Read(data)
	return &UnsecureSecretKey{data}
}

// Name returns the name of the unsecure cipher suite.
func (sk *UnsecureSecretKey) Name() Name {
	return UnsecureCipherSuiteName
}

// Pack returns the cipher data for the secret key.
func (sk *UnsecureSecretKey) Pack() *CipherData {
	return &CipherData{Data: sk.data, Name: sk.Name()}
}

// Unpack tries to convert the cipher data back to a secret key.
func (sk *UnsecureSecretKey) Unpack(p *CipherData) error {
	if len(p.Data) == 0 {
		return xerrors.New("empty data")
	}

	sk.data = p.Data
	return nil
}

func (sk *UnsecureSecretKey) String() string {
	return hex.EncodeToString(sk.data)
}

// UnsecureSignature is the signature implementation of the unsecure
// cipher.
type UnsecureSignature struct {
	data []byte
}

func newUnsecureSignature(msg []byte) *UnsecureSignature {
	return &UnsecureSignature{data: msg}
}

// Name returns the name of the unsecure cipher suite.
func (s *UnsecureSignature) Name() Name {
	return UnsecureCipherSuiteName
}

// Pack returns the cipher data of a signature.
func (s *UnsecureSignature) Pack() *CipherData {
	return &CipherData{Data: s.data, Name: s.Name()}
}

// Unpack tries to convert a cipher data back to a signature.
func (s *UnsecureSignature) Unpack(p *CipherData) error {
	if len(p.Data) == 0 {
		return xerrors.New("empty data")
	}

	s.data = p.Data
	return nil
}

func (s *UnsecureSignature) String() string {
	return hex.EncodeToString(s.data)
}

// UnsecureCipherSuite provides a cipher suite that can be used for testing
// but it *cannot* be assume to be safe.
type UnsecureCipherSuite struct{}

// Name returns the unsecure cipher suite name.
func (c *UnsecureCipherSuite) Name() Name {
	return UnsecureCipherSuiteName
}

// PublicKey generates an implementation of a public key for the
// unsecure cipher suite.
func (c *UnsecureCipherSuite) PublicKey() PublicKey {
	return &UnsecurePublicKey{}
}

// SecretKey generates an implementation of a secret key for the
// unsecure cipher suite.
func (c *UnsecureCipherSuite) SecretKey() SecretKey {
	return &UnsecureSecretKey{}
}

// Signature generates an implementation of a signature for the
// unsecure cipher suite.
func (c *UnsecureCipherSuite) Signature() Signature {
	return newUnsecureSignature([]byte{})
}

// KeyPair generates a valid pair of public and secret key for the
// unsecure cipher suite.
func (c *UnsecureCipherSuite) KeyPair() (PublicKey, SecretKey) {
	return newUnsecurePublicKey(), newUnsecureSecretKey()
}

// Sign takes a secret key and a message and returns the signature of
// the message that can be verified by the associated public key.
func (c *UnsecureCipherSuite) Sign(sk SecretKey, msg []byte) (Signature, error) {
	if len(msg) == 0 {
		return nil, xerrors.New("empty message")
	}

	return newUnsecureSignature(msg), nil
}

// Verify takes a public key, a signature and a message and returns nil
// if the signature matches the message. It returns the reasonas an error
// otherwise.
func (c *UnsecureCipherSuite) Verify(pk PublicKey, sig Signature, msg []byte) error {
	usig, ok := sig.(*UnsecureSignature)
	if !ok {
		return xerrors.New("wrong type of signature")
	}

	if !bytes.Equal(usig.data, msg) {
		return xerrors.New("mismatch data and msg")
	}

	return nil
}
