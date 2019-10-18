package ciphersuite

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnsecurePublicKey(t *testing.T) {
	pk := newUnsecurePublicKey()

	require.Equal(t, UnsecureCipherSuiteName, pk.Name())
	require.NotNil(t, pk.Pack())

	pk.data = []byte{1}
	require.Equal(t, "01", pk.String())
}

func TestUnsecurePrivateKey(t *testing.T) {
	sk := newUnsecureSecretKey()

	require.Equal(t, UnsecureCipherSuiteName, sk.Name())
	require.NotNil(t, sk.Pack())

	sk.data = []byte{2}
	require.Equal(t, "02", sk.String())
}

func TestUnsecureSignature(t *testing.T) {
	sig := newUnsecureSignature([]byte{3})

	require.Equal(t, UnsecureCipherSuiteName, sig.Name())
	require.NotNil(t, sig.Pack())
	require.Equal(t, "03", sig.String())
}

func TestUnsecureCipherSuite(t *testing.T) {
	suite := &UnsecureCipherSuite{}

	require.Equal(t, UnsecureCipherSuiteName, suite.Name())
	require.NotNil(t, suite.PublicKey())
	require.NotNil(t, suite.SecretKey())
	require.NotNil(t, suite.Signature())

	pk, sk := suite.KeyPair()
	require.NotNil(t, pk)
	require.NotNil(t, sk)

	sig, err := suite.Sign(sk, []byte{255})
	require.NoError(t, err)

	err = suite.Verify(pk, sig, []byte{255})
	require.NoError(t, err)

	require.Error(t, suite.Verify(pk, &anotherSignature{}, []byte{255}))
}
