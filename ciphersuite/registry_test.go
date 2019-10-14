package ciphersuite

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCipherRegistry_BasicUsage(t *testing.T) {
	r := NewRegistry()
	r.RegisterCipherSuite(&UnsecureCipherSuite{})

	pk, sk, err := r.KeyPair(UnsecureCipherSuiteName)
	require.NoError(t, err)

	sig, err := r.Sign(sk, []byte{1, 2, 3})
	require.NoError(t, err)

	err = r.Verify(pk, sig, []byte{})
	require.NoError(t, err)
}
