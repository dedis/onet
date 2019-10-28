package ciphersuite

import (
	"io"

	"golang.org/x/xerrors"
)

// Registry stores the cipher suites by name and provides the functions
// to unpack elements and use the cipher suite primitives.
type Registry struct {
	ciphers map[Name]CipherSuite
}

// NewRegistry creates a new empty registry.
func NewRegistry() *Registry {
	return &Registry{
		ciphers: make(map[Name]CipherSuite),
	}
}

// RegisterCipherSuite stores the cipher if it does not exist. It returns the
// the suite stored for this name if it already exists, or it returns the
// provided suite.
func (cr *Registry) RegisterCipherSuite(suite CipherSuite) CipherSuite {
	name := suite.Name()
	if suite := cr.ciphers[name]; suite != nil {
		// Cipher suite already registered so we return it so it can be reused.
		return suite
	}

	cr.ciphers[name] = suite

	return suite
}

func (cr *Registry) get(name Name) (CipherSuite, error) {
	c, _ := cr.ciphers[name]
	if c == nil {
		return nil, xerrors.New("cipher not found")
	}
	return c, nil
}

// UnpackPublicKey takes generic cipher data and tries to convert it
// into a public key of the associated implementation. The cipher suite
// must be registered beforehand.
func (cr *Registry) UnpackPublicKey(raw *RawPublicKey) (PublicKey, error) {
	c, err := cr.get(raw.Name())
	if err != nil {
		return nil, xerrors.Errorf("cipher suite: %v", err)
	}

	pk, err := c.PublicKey(raw)
	if err != nil {
		return nil, xerrors.Errorf("unpacking: %v", err)
	}

	return pk, nil
}

// UnpackSecretKey takes generic cipher data and tries to convert it
// into a secret key of the associated implementation. The cipher suite
// must be registered beforehand.
func (cr *Registry) UnpackSecretKey(raw *RawSecretKey) (SecretKey, error) {
	c, err := cr.get(raw.Name())
	if err != nil {
		return nil, xerrors.Errorf("cipher suite: %v", err)
	}

	sk, err := c.SecretKey(raw)
	if err != nil {
		return nil, xerrors.Errorf("unpacking: %v", err)
	}

	return sk, nil
}

// UnpackSignature takes generic cipher data and tries to convert it
// into a signature of the associated implementation. The cipher suite
// must be registered beforehand.
func (cr *Registry) UnpackSignature(raw *RawSignature) (Signature, error) {
	c, err := cr.get(raw.Name())
	if err != nil {
		return nil, xerrors.Errorf("cipher suite: %v", err)
	}

	sig, err := c.Signature(raw)
	if err != nil {
		return nil, xerrors.Errorf("unpacking: %v", err)
	}

	return sig, nil
}

// WithContext executes the fn by passing the cipher suite that is assigned
// to the nameable parameter.
func (cr *Registry) WithContext(n Nameable, fn func(CipherSuite) error) error {
	suite, err := cr.get(n.Name())
	if err != nil {
		return xerrors.Errorf("looking up cipher suite: %v", err)
	}

	return fn(suite)
}

// GenerateKeyPair returns a random secret key and its associated public key. This
// function will panic in case of error which means the cipher suite should
// be known and the default randomness should not trigger an error. If it
// happens, that means something is wrong with the configuration.
func (cr *Registry) GenerateKeyPair(name Name, reader io.Reader) (PublicKey, SecretKey, error) {
	c, err := cr.get(name)
	if err != nil {
		return nil, nil, xerrors.Errorf("searching for cipher suite: %v", err)
	}

	pk, sk, err := c.GenerateKeyPair(reader)
	if err != nil {
		return nil, nil, xerrors.Errorf("generating key pair: %v", err)
	}

	return pk, sk, nil
}

// Sign takes a secret key and a message and produces a signature. It will
// return an error if the signature is not known.
func (cr *Registry) Sign(sk SecretKey, msg []byte) (Signature, error) {
	c, err := cr.get(sk.Name())
	if err != nil {
		return nil, xerrors.Errorf("cipher suite: %v", err)
	}

	sig, err := c.Sign(sk, msg)
	if err != nil {
		return nil, xerrors.Errorf("signing: %v", err)
	}

	return sig, nil
}

// Verify takes a public key, a signature and a message and performs a verification
// that will return an error if the signature does not match the message. It
// will also return an error if the cipher suite is unknown.
func (cr *Registry) Verify(pk PublicKey, sig Signature, msg []byte) error {
	if pk.Name() != sig.Name() {
		return xerrors.New("mismatching cipher names")
	}

	c, err := cr.get(pk.Name())
	if err != nil {
		return xerrors.Errorf("cipher suite: %v", err)
	}

	err = c.Verify(pk, sig, msg)
	if err != nil {
		return xerrors.Errorf("verifying signature: %v", err)
	}

	return nil
}
