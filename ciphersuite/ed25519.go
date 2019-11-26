package ciphersuite

import (
	"io"

	"golang.org/x/crypto/ed25519"
	"golang.org/x/xerrors"
)

const errNotEd25519CipherSuite = "invalid cipher suite"
const errInvalidBufferSize = "invalid buffer size"

// Ed25519CipherSuiteName is the name of the cipher suite that is using Ed25519 as
// the signature algorithm.
const Ed25519CipherSuiteName = "ED25519_CIPHER_SUITE"

// Ed25519PublicKey is the public key implementation for the Ed25519 cipher suite.
type Ed25519PublicKey struct {
	data ed25519.PublicKey
}

// Name returns the name of the cipher suite.
func (pk *Ed25519PublicKey) Name() Name {
	return Ed25519CipherSuiteName
}

func (pk *Ed25519PublicKey) String() string {
	return pk.Raw().String()
}

// Raw returns the raw public key.
func (pk *Ed25519PublicKey) Raw() *RawPublicKey {
	return NewRawPublicKey(pk.Name(), pk.data)
}

// Equal returns true when both public key are equal.
func (pk *Ed25519PublicKey) Equal(other PublicKey) bool {
	return pk.Raw().Equal(other.Raw())
}

// Ed25519SecretKey is the secret key implementation for the Ed25519 cipher suite.
type Ed25519SecretKey struct {
	data ed25519.PrivateKey
}

// Name returns the cipher suite name.
func (sk *Ed25519SecretKey) Name() Name {
	return Ed25519CipherSuiteName
}

func (sk *Ed25519SecretKey) String() string {
	return sk.Raw().String()
}

// Raw returns the raw secret key.
func (sk *Ed25519SecretKey) Raw() *RawSecretKey {
	return NewRawSecretKey(sk.Name(), sk.data)
}

// Ed25519Signature is the signature implementation for the Ed25519 cipher suite.
type Ed25519Signature struct {
	data []byte
}

// Name returns the name of the cipher suite.
func (sig *Ed25519Signature) Name() Name {
	return Ed25519CipherSuiteName
}

func (sig *Ed25519Signature) String() string {
	return sig.Raw().String()
}

// Raw returns the raw signature.
func (sig *Ed25519Signature) Raw() *RawSignature {
	return NewRawSignature(sig.Name(), sig.data)
}

// Ed25519CipherSuite is a cipher suite implementation using the Ed25519 scheme.
type Ed25519CipherSuite struct{}

// NewEd25519CipherSuite returns an instance of the cipher suite.
func NewEd25519CipherSuite() *Ed25519CipherSuite {
	return &Ed25519CipherSuite{}
}

// Name returns the name of the suite.
func (s *Ed25519CipherSuite) Name() Name {
	return Ed25519CipherSuiteName
}

// PublicKey takes a raw public key and converts it to a public key.
func (s *Ed25519CipherSuite) PublicKey(raw *RawPublicKey) (PublicKey, error) {
	if raw.Name() != s.Name() {
		return nil, xerrors.New(errNotEd25519CipherSuite)
	}

	if len(raw.Data) != ed25519.PublicKeySize {
		return nil, xerrors.New(errInvalidBufferSize)
	}

	return &Ed25519PublicKey{data: raw.Data}, nil
}

// SecretKey takes a raw secret key and converts it to a secret key.
func (s *Ed25519CipherSuite) SecretKey(raw *RawSecretKey) (SecretKey, error) {
	if raw.Name() != s.Name() {
		return nil, xerrors.New(errNotEd25519CipherSuite)
	}

	if len(raw.Data) != ed25519.PrivateKeySize {
		return nil, xerrors.New(errInvalidBufferSize)
	}

	return &Ed25519SecretKey{data: raw.Data}, nil
}

// Signature takes a raw signature and converts it to a signature.
func (s *Ed25519CipherSuite) Signature(raw *RawSignature) (Signature, error) {
	if raw.Name() != s.Name() {
		return nil, xerrors.New(errNotEd25519CipherSuite)
	}

	if len(raw.Data) != ed25519.SignatureSize {
		return nil, xerrors.New(errInvalidBufferSize)
	}

	return &Ed25519Signature{data: raw.Data}, nil
}

// GenerateKeyPair generates a secret key and its associated public key. When
// reader is nil, it will use the default randomness source.
func (s *Ed25519CipherSuite) GenerateKeyPair(reader io.Reader) (PublicKey, SecretKey, error) {
	pk, sk, err := ed25519.GenerateKey(reader)
	if err != nil {
		return nil, nil, xerrors.Errorf("generate ed25519 key: %v", err)
	}

	return &Ed25519PublicKey{data: pk}, &Ed25519SecretKey{data: sk}, nil
}

// Sign signs the message with the secret key and returns the signature that
// can be verified with the public key.
func (s *Ed25519CipherSuite) Sign(sk SecretKey, msg []byte) (Signature, error) {
	secretKey, err := s.unpackSecretKey(sk)
	if err != nil {
		return nil, xerrors.Errorf("unpacking secret key: %v", err)
	}
	sigbuf := ed25519.Sign(secretKey.data, msg)

	return &Ed25519Signature{data: sigbuf}, nil
}

// Verify returns nil when the signature of the message can be verified by
// the public key.
func (s *Ed25519CipherSuite) Verify(pk PublicKey, sig Signature, msg []byte) error {
	publicKey, err := s.unpackPublicKey(pk)
	if err != nil {
		return xerrors.Errorf("unpacking public key: %v", err)
	}

	signature, err := s.unpackSignature(sig)
	if err != nil {
		return xerrors.Errorf("unpacking signature: %v", err)
	}

	if !ed25519.Verify(publicKey.data, msg, signature.data) {
		return xerrors.New("signature not verified")
	}

	return nil
}

func (s *Ed25519CipherSuite) unpackPublicKey(pk PublicKey) (*Ed25519PublicKey, error) {
	if data, ok := pk.(*RawPublicKey); ok {
		var err error
		pk, err = s.PublicKey(data)
		if err != nil {
			return nil, xerrors.Errorf("unmarshaling raw public key: %v", err)
		}
	}

	if publicKey, ok := pk.(*Ed25519PublicKey); ok {
		return publicKey, nil
	}

	return nil, xerrors.New("wrong type of public key")
}

func (s *Ed25519CipherSuite) unpackSecretKey(sk SecretKey) (*Ed25519SecretKey, error) {
	if data, ok := sk.(*RawSecretKey); ok {
		var err error
		sk, err = s.SecretKey(data)
		if err != nil {
			return nil, xerrors.Errorf("unmarshaling raw secret key: %v", err)
		}
	}

	if secretKey, ok := sk.(*Ed25519SecretKey); ok {
		return secretKey, nil
	}

	return nil, xerrors.New("wrong type of secret key")
}

func (s *Ed25519CipherSuite) unpackSignature(sig Signature) (*Ed25519Signature, error) {
	if data, ok := sig.(*RawSignature); ok {
		var err error
		sig, err = s.Signature(data)
		if err != nil {
			return nil, xerrors.Errorf("unmarshaling raw signature: %v", err)
		}
	}

	if signature, ok := sig.(*Ed25519Signature); ok {
		return signature, nil
	}

	return nil, xerrors.New("wrong type of signature")
}
