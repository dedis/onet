package ciphersuite

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

var anotherCipherSuiteName = "another_cipher_suite"

type anotherCipherSuite struct {
	*Ed25519CipherSuite
}

func (a *anotherCipherSuite) Name() Name {
	return anotherCipherSuiteName
}

func (a *anotherCipherSuite) Sign(sk SecretKey, msg []byte) (Signature, error) {
	return nil, xerrors.New("test error")
}

type anotherSignature struct {
	*Ed25519Signature
}

func (s *anotherSignature) Name() Name {
	return anotherCipherSuiteName
}

// Test the basic usage of the registry.
func TestCipherRegistry_BasicUsage(t *testing.T) {
	r := NewRegistry()
	r.RegisterCipherSuite(NewEd25519CipherSuite())

	pk, sk, err := r.GenerateKeyPair(Ed25519CipherSuiteName, nil)
	require.NoError(t, err)

	sig, err := r.Sign(sk, []byte{1, 2, 3})
	require.NoError(t, err)

	err = r.Verify(pk.Raw(), sig, []byte{})
	require.Error(t, err)

	rawPk := pk.Raw()
	rawPk.CipherName = anotherCipherSuiteName
	rawSig := sig.Raw()
	rawSig.CipherName = anotherCipherSuiteName
	err = r.Verify(rawPk, rawSig, []byte{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cipher suite:")

	err = r.Verify(pk, sig, []byte{1, 2, 3})
	require.NoError(t, err)
}

func TestCipherRegistry_WithContext(t *testing.T) {
	r := NewRegistry()
	r.RegisterCipherSuite(NewEd25519CipherSuite())

	pk, _, err := r.GenerateKeyPair(Ed25519CipherSuiteName, nil)
	require.NoError(t, err)

	err = r.WithContext(pk, func(suite CipherSuite) error {
		return nil
	})
	require.NoError(t, err)

	errExample := xerrors.New("oops")
	err = r.WithContext(pk, func(suite CipherSuite) error {
		return errExample
	})
	require.True(t, xerrors.Is(err, errExample))

	err = r.WithContext(&anotherCipherSuite{}, func(suite CipherSuite) error {
		return nil
	})
	require.Error(t, err)
}

func TestCipherRegistry_GenerateKeyPair(t *testing.T) {
	r := NewRegistry()

	_, _, err := r.GenerateKeyPair(anotherCipherSuiteName, nil)
	require.Error(t, err)

	r.RegisterCipherSuite(NewEd25519CipherSuite())
	_, _, err = r.GenerateKeyPair(Ed25519CipherSuiteName, &badReader{})
	require.Error(t, err)

	pk, sk, err := r.GenerateKeyPair(Ed25519CipherSuiteName, nil)
	require.NoError(t, err)
	require.NotNil(t, pk)
	require.NotNil(t, sk)
}

func TestCipherRegistry_FailingSignature(t *testing.T) {
	r := NewRegistry()
	r.RegisterCipherSuite(&anotherCipherSuite{})

	_, sk, err := r.GenerateKeyPair(anotherCipherSuiteName, nil)
	require.NoError(t, err)

	rawSk := sk.Raw()
	rawSk.CipherName = anotherCipherSuiteName

	_, err = r.Sign(rawSk, []byte{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "signing:")
}

func TestCipherRegistry_SuiteNotFound(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expect a panic")
		}
	}()

	r := NewRegistry()
	r.RegisterCipherSuite(&anotherCipherSuite{})

	ed := NewEd25519CipherSuite()
	pk, sk, err := ed.GenerateKeyPair(nil)
	require.NoError(t, err)

	sig, err := r.Sign(sk, []byte{})
	require.Error(t, err)

	err = r.Verify(pk, sig, []byte{})
	require.Error(t, err)

	r.GenerateKeyPair(Ed25519CipherSuiteName, nil)
}

func TestCipherRegistry_InvalidType(t *testing.T) {
	r := NewRegistry()
	r.RegisterCipherSuite(NewEd25519CipherSuite())

	ed := NewEd25519CipherSuite()
	pk, _, err := ed.GenerateKeyPair(nil)
	require.NoError(t, err)

	sig := &anotherSignature{}
	err = r.Verify(pk, sig, []byte{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "mismatch")
}

func TestCipherRegistry_Registration(t *testing.T) {
	r := NewRegistry()

	suite := NewEd25519CipherSuite()

	r.RegisterCipherSuite(suite)
	require.Equal(t, suite, r.RegisterCipherSuite(NewEd25519CipherSuite()))
	require.NotEqual(t, suite, r.RegisterCipherSuite(&anotherCipherSuite{}))

	require.Equal(t, 2, len(r.ciphers))
}

func TestCipherRegistry_Unpack(t *testing.T) {
	r := NewRegistry()

	pk := (&Ed25519PublicKey{data: []byte{}}).Raw()
	_, err := r.UnpackPublicKey(pk)
	require.Error(t, err)
	sk := (&Ed25519SecretKey{data: []byte{}}).Raw()
	_, err = r.UnpackSecretKey(sk)
	require.Error(t, err)
	sig := (&Ed25519Signature{data: []byte{}}).Raw()
	_, err = r.UnpackSignature(sig)
	require.Error(t, err)

	r.RegisterCipherSuite(NewEd25519CipherSuite())

	pk.Data = []byte{}
	_, err = r.UnpackPublicKey(pk)
	require.Error(t, err)

	sk.Data = []byte{}
	_, err = r.UnpackSecretKey(sk)
	require.Error(t, err)

	sig.Data = []byte{}
	_, err = r.UnpackSignature(sig)
	require.Error(t, err)

	pk2, sk2, err := r.GenerateKeyPair(Ed25519CipherSuiteName, nil)
	require.NoError(t, err)
	pk2, err = r.UnpackPublicKey(pk2.Raw())
	require.NoError(t, err)
	require.NotNil(t, pk2)

	sk2, err = r.UnpackSecretKey(sk2.Raw())
	require.NoError(t, err)
	require.NotNil(t, sk2)

	sig2, err := r.Sign(sk2, []byte{})
	require.NoError(t, err)
	sig2, err = r.UnpackSignature(sig2.Raw())
	require.NoError(t, err)
	require.NotNil(t, sig2)
}
