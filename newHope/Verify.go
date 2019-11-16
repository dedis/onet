package newHope

import (
	"fmt"

	"github.com/ldsec/lattigo/ring"
	"go.dedis.ch/onet/v3/glyph"
	"golang.org/x/xerrors"
)

const InvalidPolynomialError = "Invalid polynomial"

const InvalidSignature = "Invalid signature: Signature could not be verified with the public key"

func checkPublicKey(pk []byte, ctx *ring.Context) (*glyph.PublicKey, error) {
	size := NewHopePublicKeySize
	l := len(pk)
	if size != l {
		return nil, xerrors.New("Invalid public key size")
	}
	t := ctx.NewPoly()
	key, e := t.UnMarshalBinary(pk)
	if e != nil {
		return nil, e
	}

	return glyph.NewPublicKey(key), nil
}

func checkSignature(sig []byte, ctx *ring.Context) (*glyph.Signature, error) {
	size := NewHopeSignatureSize
	l := len(sig)
	if l != size {
		return nil, xerrors.New("Invalid signature length")
	}
	polySize := NewHopePolySize
	if l != polySize*3 {
		return nil, xerrors.New("Signature has to be the size of three polynomials")
	}
	z1, z2, c := ctx.NewPoly(), ctx.NewPoly(), ctx.NewPoly()
	var e1, e2, e3 error
	z1, e1 = z1.UnMarshalBinary(sig[0:polySize])
	if e1 != nil {
		return nil, xerrors.New(InvalidPolynomialError)
	}
	z2, e2 = z2.UnMarshalBinary(sig[polySize : 2*polySize])
	if e2 != nil {
		return nil, xerrors.New(InvalidPolynomialError)
	}
	c, e3 = c.UnMarshalBinary(sig[2*polySize : l])
	if e3 != nil {
		return nil, xerrors.New(InvalidPolynomialError)
	}
	return glyph.NewSignature(z1, z2, c), nil
}

func Verify(pk, msg, sig []byte) error {
	ctx := glyph.GetCtx()
	public, e := checkPublicKey(pk, ctx)
	if e != nil {
		return e
	}
	signature, esig := checkSignature(sig, ctx)
	if esig != nil {
		return esig
	}
	fmt.Println(public, signature)
	verified := public.Verify(signature, msg)
	if !verified {
		return xerrors.New(InvalidSignature)
	}
	return nil
}
