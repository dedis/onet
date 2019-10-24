package ciphersuite

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

var anotherCipherSuiteName = "another_cipher_suite"

type anotherCipherSuite struct {
	*UnsecureCipherSuite
}

func (a *anotherCipherSuite) Name() Name {
	return anotherCipherSuiteName
}

type anotherSignature struct {
	*UnsecureSignature
}

func (s *anotherSignature) Name() Name {
	return anotherCipherSuiteName
}

// Test the basic usage of the registry.
func TestCipherRegistry_BasicUsage(t *testing.T) {
	r := NewRegistry()
	r.RegisterCipherSuite(&UnsecureCipherSuite{})

	pk, sk := r.KeyPair(UnsecureCipherSuiteName)

	sig, err := r.Sign(sk, []byte{1, 2, 3})
	require.NoError(t, err)

	err = r.Verify(pk.Raw(), sig, []byte{})
	require.Error(t, err)

	err = r.Verify(pk, sig, []byte{1, 2, 3})
}

func TestCipherRegistry_WithContext(t *testing.T) {
	r := NewRegistry()
	r.RegisterCipherSuite(&UnsecureCipherSuite{})

	pk, _ := r.KeyPair(UnsecureCipherSuiteName)

	err := r.WithContext(pk, func(suite CipherSuite) error {
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

func TestCipherRegistry_FailingSignature(t *testing.T) {
	r := NewRegistry()
	r.RegisterCipherSuite(&UnsecureCipherSuite{})

	_, sk := r.KeyPair(UnsecureCipherSuiteName)

	_, err := r.Sign(sk, []byte{})
	require.Error(t, err)
}

func TestCipherRegistry_SuiteNotFound(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expect a panic")
		}
	}()

	r := NewRegistry()
	r.RegisterCipherSuite(&anotherCipherSuite{})

	sk := newUnsecureSecretKey()
	_, err := r.Sign(sk, []byte{})
	require.Error(t, err)

	pk := newUnsecurePublicKey()
	err = r.Verify(pk, newUnsecureSignature([]byte{}), []byte{})
	require.Error(t, err)

	r.KeyPair(UnsecureCipherSuiteName)
}

func TestCipherRegistry_InvalidType(t *testing.T) {
	r := NewRegistry()
	r.RegisterCipherSuite(&UnsecureCipherSuite{})

	pk := newUnsecurePublicKey()
	sig := &anotherSignature{}
	err := r.Verify(pk, sig, []byte{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "mismatch")
}

func TestCipherRegistry_Registration(t *testing.T) {
	r := NewRegistry()

	suite := &UnsecureCipherSuite{}

	r.RegisterCipherSuite(suite)
	require.Equal(t, suite, r.RegisterCipherSuite(&UnsecureCipherSuite{}))
	require.NotEqual(t, suite, r.RegisterCipherSuite(&anotherCipherSuite{}))

	require.Equal(t, 2, len(r.ciphers))
}

func TestCipherRegistry_Unpack(t *testing.T) {
	r := NewRegistry()

	pk := newUnsecurePublicKey().Raw()
	_, err := r.UnpackPublicKey(pk)
	require.Error(t, err)
	sk := newUnsecureSecretKey().Raw()
	_, err = r.UnpackSecretKey(sk)
	require.Error(t, err)
	sig := newUnsecureSignature([]byte{}).Raw()
	_, err = r.UnpackSignature(sig)
	require.Error(t, err)

	r.RegisterCipherSuite(&UnsecureCipherSuite{})

	pk.Data = []byte{}
	_, err = r.UnpackPublicKey(pk)
	require.Error(t, err)

	sk.Data = []byte{}
	_, err = r.UnpackSecretKey(sk)
	require.Error(t, err)

	sig.Data = []byte{}
	_, err = r.UnpackSignature(sig)
	require.Error(t, err)

	pk2, err := r.UnpackPublicKey(newUnsecurePublicKey().Raw())
	require.NoError(t, err)
	require.NotNil(t, pk2)

	sk2, err := r.UnpackSecretKey(newUnsecureSecretKey().Raw())
	require.NoError(t, err)
	require.NotNil(t, sk2)

	sig2, err := r.UnpackSignature(newUnsecureSignature([]byte{1, 2, 3}).Raw())
	require.NoError(t, err)
	require.NotNil(t, sig2)
}
