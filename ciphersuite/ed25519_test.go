package ciphersuite

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/xerrors"
)

func TestEd25519PublicKey(t *testing.T) {
	pk, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	publicKey := &Ed25519PublicKey{data: pk}
	require.Equal(t, Ed25519CipherSuiteName, publicKey.Name())
	require.NotNil(t, publicKey.Raw())
	require.Equal(t, ed25519.PublicKeySize, len(publicKey.String())/2)

	suite := NewEd25519CipherSuite()
	publicKey2, err := suite.PublicKey(publicKey.Raw())
	require.Equal(t, publicKey, publicKey2)
}

func TestEd25519PublicKey_Equal(t *testing.T) {
	pk, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	publicKey := &Ed25519PublicKey{data: pk}

	pk2, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	publicKey2 := &Ed25519PublicKey{data: pk2}

	require.True(t, publicKey.Equal(publicKey))
	require.False(t, publicKey.Equal(publicKey2))
}

func TestEd25519SecretKey(t *testing.T) {
	_, sk, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	secretKey := &Ed25519SecretKey{data: sk}

	require.Equal(t, Ed25519CipherSuiteName, secretKey.Name())
	require.NotNil(t, secretKey.Raw())
	require.Equal(t, ed25519.PrivateKeySize, len(secretKey.String())/2)
}

func TestEd25519Signature(t *testing.T) {
	_, sk, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	sig := ed25519.Sign(sk, []byte{})
	signature := &Ed25519Signature{data: sig}

	require.Equal(t, Ed25519CipherSuiteName, signature.Name())
	require.NotNil(t, signature.Raw())
	require.Equal(t, ed25519.SignatureSize, len(signature.String())/2)
}

func TestEd25519CipherSuite_BasicUsage(t *testing.T) {
	suite := NewEd25519CipherSuite()

	pk, sk, err := suite.GenerateKeyPair(nil)
	require.NoError(t, err)
	msg := []byte("deadbeef")

	sig, err := suite.Sign(sk, msg)
	require.NoError(t, err)

	err = suite.Verify(pk, sig, msg)
	require.NoError(t, err)
}

type testPublicKey struct {
	*Ed25519PublicKey
}

type testSecretKey struct {
	*Ed25519SecretKey
}

type testSignature struct {
	*Ed25519Signature
}

func TestEd25519CipherSuite_Unpacking(t *testing.T) {
	suite := NewEd25519CipherSuite()

	// Public keys
	rawPk := &RawPublicKey{CipherData: &CipherData{CipherName: "abc"}}
	_, err := suite.PublicKey(rawPk)
	require.Error(t, err)
	require.Contains(t, err.Error(), errNotEd25519CipherSuite)

	_, err = suite.unpackPublicKey(&testPublicKey{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "wrong type of public key")

	rawPk.CipherName = Ed25519CipherSuiteName
	rawPk.Data = []byte{}
	_, err = suite.PublicKey(rawPk)
	require.Error(t, err)
	require.Contains(t, err.Error(), errInvalidBufferSize)

	// Secret Keys
	rawSk := &RawSecretKey{CipherData: &CipherData{CipherName: "abc"}}
	_, err = suite.SecretKey(rawSk)
	require.Error(t, err)
	require.Contains(t, err.Error(), errNotEd25519CipherSuite)

	_, err = suite.unpackSecretKey(&testSecretKey{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "wrong type of secret key")

	rawSk.CipherName = Ed25519CipherSuiteName
	rawSk.Data = []byte{}
	_, err = suite.SecretKey(rawSk)
	require.Error(t, err)
	require.Contains(t, err.Error(), errInvalidBufferSize)

	// Signatures
	rawSig := &RawSignature{CipherData: &CipherData{CipherName: "abc"}}
	_, err = suite.Signature(rawSig)
	require.Error(t, err)
	require.Contains(t, err.Error(), errNotEd25519CipherSuite)

	_, err = suite.unpackSignature(&testSignature{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "wrong type of signature")

	rawSig.CipherName = Ed25519CipherSuiteName
	rawSig.Data = []byte{}
	_, err = suite.Signature(rawSig)
	require.Error(t, err)
	require.Contains(t, err.Error(), errInvalidBufferSize)
}

type badReader struct{}

func (br *badReader) Read(p []byte) (int, error) {
	return 0, xerrors.New("oops")
}

func TestEd25519CipherSuite_GenerateKey(t *testing.T) {
	suite := NewEd25519CipherSuite()

	_, _, err := suite.GenerateKeyPair(&badReader{})
	require.Error(t, err)

	pk, sk, err := suite.GenerateKeyPair(nil)
	require.NoError(t, err)
	require.NotNil(t, pk)
	require.NotNil(t, sk)
}

func TestEd25519CipherSuite_Sign(t *testing.T) {
	suite := NewEd25519CipherSuite()

	_, sk, err := suite.GenerateKeyPair(nil)
	require.NoError(t, err)

	sig, err := suite.Sign(sk, []byte{})
	require.NoError(t, err)
	require.NotNil(t, sig)

	rawSk := sk.Raw()
	rawSk.CipherName = "abc"

	_, err = suite.Sign(rawSk, []byte{})
	require.Error(t, err)
}

func TestEd25519CipherSuite_Verify(t *testing.T) {
	suite := NewEd25519CipherSuite()

	pk, sk, err := suite.GenerateKeyPair(nil)
	require.NoError(t, err)

	sig, err := suite.Sign(sk, []byte{})
	require.NoError(t, err)

	err = suite.Verify(pk, sig, []byte{})
	require.NoError(t, err)

	rawSig := sig.Raw()
	rawSig.CipherName = "abc"
	err = suite.Verify(pk, rawSig, []byte{})
	require.Error(t, err)

	rawPk := pk.Raw()
	rawPk.CipherName = "abc"
	err = suite.Verify(rawPk, sig, []byte{})
	require.Error(t, err)
}
