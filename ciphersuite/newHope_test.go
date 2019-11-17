package ciphersuite

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"go.dedis.ch/onet/v3/newHope"
)

func TestNewHopePublicKey(t *testing.T) {
	pk, _, e := newHope.GenerateKey(nil)
	require.NoError(t, e)
	publicKey := &NewHopePublicKey{data: pk}

	require.Equal(t, newHope.NewHopeName, publicKey.Name())
	require.NotNil(t, publicKey.Raw())
	require.Equal(t, newHope.NewHopePublicKeySize, len(publicKey.String())/2)
	suite := NewNewHopeCipherSuite()
	publicKey2, err := suite.PublicKey(publicKey.Raw())
	require.Nil(t, err)
	require.Equal(t, publicKey, publicKey2)
}

func TestNewHopePublicKey_Equal(t *testing.T) {
	pk, _, e := newHope.GenerateKey(nil)
	require.NoError(t, e)
	publicKey := &NewHopePublicKey{data: pk}

	pk2, _, err := newHope.GenerateKey(nil)
	require.NoError(t, err)
	publicKey2 := &NewHopePublicKey{data: pk2}

	require.True(t, publicKey.Equal(publicKey))
	require.False(t, publicKey.Equal(publicKey2))
}

func TestNewHopeSignature(t *testing.T) {
	_, sk, err := newHope.GenerateKey(nil)
	require.NoError(t, err)
	sig, e := newHope.Sign(sk, []byte{})
	require.Nil(t, e)
	signature := &NewHopeSignature{data: sig}
	require.Equal(t, newHope.NewHopeName, signature.Name())
	require.NotNil(t, signature.Raw())
	require.Equal(t, newHope.NewHopeSignatureSize, len(signature.String())/2)
}

func TestNewHopeSecretKey(t *testing.T) {
	_, sk, err := newHope.GenerateKey(nil)
	require.NoError(t, err)

	secretKey := &NewHopePrivateKey{data: sk}

	require.Equal(t, newHope.NewHopeName, secretKey.Name())
	require.NotNil(t, secretKey.Raw())
	require.Equal(t, newHope.NewHopePrivateKeySize, len(secretKey.String())/2)
}

func TestNewHopeCipherSuite_BasicUsage(t *testing.T) {
	suite := NewNewHopeCipherSuite()

	pk, sk, err := suite.GenerateKeyPair(nil)
	require.NoError(t, err)

	msg := []byte("deadbeef")

	sig, err := suite.Sign(sk, msg)
	require.NoError(t, err)

	err = suite.Verify(pk, sig, msg)
	require.NoError(t, err)
}

type testPublicKeyNewHope struct {
	*NewHopePublicKey
}

type testSecretKeyNewHope struct {
	*NewHopePrivateKey
}

type testSignatureNewHope struct {
	*NewHopeSignature
}

func TestNewHopeCipherSuite_unpacking(t *testing.T) {
	suite := NewNewHopeCipherSuite()
	rawPk := &RawPublicKey{CipherData: &CipherData{CipherName: "abc"}}
	_, err := suite.PublicKey(rawPk)
	require.Error(t, err)
	require.Contains(t, err.Error(), errNotNewHopeCipherSuite)

	_, err = suite.unpackPublicKey(&testPublicKey{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "wrong type of public key")

	rawPk.CipherName = newHope.NewHopeName
	rawPk.Data = []byte{}
	_, err = suite.PublicKey(rawPk)
	require.Error(t, err)
	//fmt.Println(err.Error())
	require.Contains(t, err.Error(), errInvalidBufferSize)

	// Secret Keys
	rawSk := &RawSecretKey{CipherData: &CipherData{CipherName: "abc"}}
	_, err = suite.SecretKey(rawSk)
	require.Error(t, err)
	require.Contains(t, err.Error(), errNotNewHopeCipherSuite)

	_, err = suite.unpackSecretKey(&testSecretKey{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "wrong type of secret key")

	rawSk.CipherName = newHope.NewHopeName
	rawSk.Data = []byte{}
	_, err = suite.SecretKey(rawSk)
	require.Error(t, err)
	require.Contains(t, err.Error(), errInvalidBufferSize)

	// Signatures
	rawSig := &RawSignature{CipherData: &CipherData{CipherName: "abc"}}
	_, err = suite.Signature(rawSig)
	require.Error(t, err)
	require.Contains(t, err.Error(), errNotNewHopeCipherSuite)

	_, err = suite.unpackSignature(&testSignature{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "wrong type of signature")

	rawSig.CipherName = newHope.NewHopeName
	rawSig.Data = []byte{}
	_, err = suite.Signature(rawSig)
	require.Error(t, err)
	require.Contains(t, err.Error(), errInvalidBufferSize)
}

type badReaderNewHope struct{}

func (br *badReaderNewHope) Read(p []byte) (int, error) {
	return 0, xerrors.New("oops")
}

func TestNewHopeCipherSuite_GenerateKey(t *testing.T) {
	suite := NewNewHopeCipherSuite()

	_, _, err := suite.GenerateKeyPair(&badReaderNewHope{})
	require.Error(t, err)

	pk, sk, err := suite.GenerateKeyPair(nil)
	require.NoError(t, err)
	require.NotNil(t, pk)
	require.NotNil(t, sk)
}

func TestNewHopeCipherSuite_Sign(t *testing.T) {
	suite := NewNewHopeCipherSuite()

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

func TestNewHopeCipherSuite_Verify(t *testing.T) {
	suite := NewNewHopeCipherSuite()

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
