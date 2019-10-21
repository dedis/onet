package ciphersuite

import (
	"testing"

	"github.com/stretchr/testify/require"
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

	err = r.Verify(pk, sig, []byte{})
	require.Error(t, err)

	err = r.Verify(pk, sig, []byte{1, 2, 3})
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

	_, err := r.UnpackPublicKey(&CipherData{Name: ""})
	require.Error(t, err)
	_, err = r.UnpackSecretKey(&CipherData{Name: ""})
	require.Error(t, err)
	_, err = r.UnpackSignature(&CipherData{Name: ""})
	require.Error(t, err)

	r.RegisterCipherSuite(&UnsecureCipherSuite{})

	pk, err := r.UnpackPublicKey(newUnsecurePublicKey().Pack())
	require.NoError(t, err)
	require.NotNil(t, pk)

	sk, err := r.UnpackSecretKey(newUnsecureSecretKey().Pack())
	require.NoError(t, err)
	require.NotNil(t, sk)

	sig, err := r.UnpackSignature(newUnsecureSignature([]byte{}).Pack())
	require.NoError(t, err)
	require.NotNil(t, sig)
}
