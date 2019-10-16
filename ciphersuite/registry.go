package ciphersuite

import "golang.org/x/xerrors"

// Registry stores the ciphers the server is aware of. It provides
// the primitives to make and verify signatures.
type Registry struct {
	ciphers map[string]CipherSuite
}

// NewRegistry creates a new empty registry.
func NewRegistry() *Registry {
	return &Registry{
		ciphers: make(map[string]CipherSuite),
	}
}

// RegisterCipherSuite stores the cipher if it does not exist yet and return an
// error otherwise.
func (cr *Registry) RegisterCipherSuite(suite CipherSuite) error {
	name := suite.Name()
	if _, ok := cr.ciphers[name]; ok {
		return xerrors.New("already existing cipher")
	}

	cr.ciphers[name] = suite

	return nil
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
func (cr *Registry) UnpackPublicKey(p *CipherData) (PublicKey, error) {
	c, err := cr.get(p.Name)
	if err != nil {
		return nil, err
	}

	pk := c.PublicKey()
	err = pk.Unpack(p)
	return pk, err
}

// UnpackSecretKey takes generic cipher data and tries to convert it
// into a secret key of the associated implementation. The cipher suite
// must be registered beforehand.
func (cr *Registry) UnpackSecretKey(p *CipherData) (SecretKey, error) {
	c, err := cr.get(p.Name)
	if err != nil {
		return nil, err
	}

	sk := c.SecretKey()
	err = sk.Unpack(p)
	return sk, err
}

// UnpackSignature takes generic cipher data and tries to convert it
// into a signature of the associated implementation. The cipher suite
// must be registered beforehand.
func (cr *Registry) UnpackSignature(p *CipherData) (Signature, error) {
	c, err := cr.get(p.Name)
	if err != nil {
		return nil, err
	}

	sig := c.Signature()
	err = sig.Unpack(p)
	return sig, err
}

// KeyPair returns a random secret key and its associated public key.
func (cr *Registry) KeyPair(name Name) (PublicKey, SecretKey) {
	c, err := cr.get(name)
	if err != nil {
		panic(err)
	}

	return c.KeyPair()
}

// Sign takes a secret and a message and produces a signature.
func (cr *Registry) Sign(sk SecretKey, msg []byte) (Signature, error) {
	c, err := cr.get(sk.Name())
	if err != nil {
		return nil, err
	}

	return c.Sign(sk, msg)
}

// Verify takes a public, a signature and a message and performs a verification
// that will return an error if the signature does not match the message.
func (cr *Registry) Verify(pk PublicKey, sig Signature, msg []byte) error {
	if pk.Name() != sig.Name() {
		return xerrors.New("mismatching cipher names")
	}

	c, err := cr.get(pk.Name())
	if err != nil {
		return err
	}

	return c.Verify(pk, sig, msg)
}
