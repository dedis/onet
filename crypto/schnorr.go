package crypto

import (
	"gopkg.in/dedis/crypto.v0/abstract"
	"gopkg.in/dedis/crypto.v0/sign"
)

// SchnorrSig is a signature created using the Schnorr Signature scheme.
type SchnorrSig struct {
	Challenge abstract.Point
	Response  abstract.Scalar
}

// SignSchnorr creates a Schnorr signature from a msg and a private key
func SignSchnorr(suite abstract.Suite, private abstract.Scalar, msg []byte) (SchnorrSig, error) {
	ss := SchnorrSig{
		Challenge: suite.Point(),
		Response:  suite.Scalar(),
	}
	buf, err := sign.Schnorr(suite, private, msg)
	if err != nil {
		return ss, err
	}
	scalarLen := suite.ScalarLen()
	if err := ss.Challenge.UnmarshalBinary(buf[0:scalarLen]); err != nil {
		return ss, err
	}
	if err := ss.Response.UnmarshalBinary(buf[scalarLen:]); err != nil {
		return ss, err
	}
	return ss, nil
}

// VerifySchnorr verifies a given Schnorr signature. It returns nil iff the given signature is valid.
func VerifySchnorr(suite abstract.Suite, public abstract.Point, msg []byte, sig SchnorrSig) error {
	ch, err := sig.Challenge.MarshalBinary()
	if err != nil {
		return err
	}
	re, err := sig.Response.MarshalBinary()
	if err != nil {
		return err
	}
	return sign.VerifySchnorr(suite, public, msg, append(ch, re...))
}
