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

// Equal returns true when the two public keys are deeply equal.
func (pk *UnsecurePublicKey) Equal(other PublicKey) bool {
	otherPk, ok := other.(*UnsecurePublicKey)
	if !ok {
		return false
	}

	return bytes.Equal(pk.data, otherPk.data)
}

// Raw returns the cipher data of the public key.
func (pk *UnsecurePublicKey) Raw() *RawPublicKey {
	return &RawPublicKey{
		CipherData: &CipherData{Data: pk.data, CipherName: pk.Name()},
	}
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

// Raw returns the cipher data for the secret key.
func (sk *UnsecureSecretKey) Raw() *RawSecretKey {
	return &RawSecretKey{
		CipherData: &CipherData{Data: sk.data, CipherName: sk.Name()},
	}
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

// Raw returns the cipher data of a signature.
func (s *UnsecureSignature) Raw() *RawSignature {
	return &RawSignature{
		&CipherData{Data: s.data, CipherName: s.Name()},
	}
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
func (c *UnsecureCipherSuite) PublicKey(raw *RawPublicKey) (PublicKey, error) {
	if len(raw.Data) == 0 {
		return nil, xerrors.New("empty data")
	}

	return &UnsecurePublicKey{
		data: raw.Data,
	}, nil
}

// SecretKey generates an implementation of a secret key for the
// unsecure cipher suite.
func (c *UnsecureCipherSuite) SecretKey(raw *RawSecretKey) (SecretKey, error) {
	if len(raw.Data) == 0 {
		return nil, xerrors.New("empty data")
	}

	return &UnsecureSecretKey{
		data: raw.Data,
	}, nil
}

// Signature generates an implementation of a signature for the
// unsecure cipher suite.
func (c *UnsecureCipherSuite) Signature(raw *RawSignature) (Signature, error) {
	if len(raw.Data) == 0 {
		return nil, xerrors.New("empty data")
	}

	return newUnsecureSignature(raw.Data), nil
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
	signature, err := c.unpackSignature(sig)
	if err != nil {
		return xerrors.Errorf("unpacking signature: %v", err)
	}

	if !bytes.Equal(signature.data, msg) {
		return xerrors.New("mismatch data and msg")
	}

	return nil
}

func (c *UnsecureCipherSuite) unpackSignature(signature Signature) (*UnsecureSignature, error) {
	if data, ok := signature.(*RawSignature); ok {
		sig, err := c.Signature(data)
		if err != nil {
			return nil, err
		}
		return sig.(*UnsecureSignature), nil
	}

	if sig, ok := signature.(*UnsecureSignature); ok {
		return sig, nil
	}

	return nil, xerrors.New("incompatible signature type")
}
