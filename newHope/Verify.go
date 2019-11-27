package newHope

import (
	"github.com/lca1/lattigo/newhope"
	"github.com/ldsec/lattigo/ring"
	"go.dedis.ch/onet/v3/glyph"
	"go.dedis.ch/onet/v3/glyph_small"
	"golang.org/x/xerrors"
)

//InvalidPolynomialError is when the polynomial can't be unmarshalled
const InvalidPolynomialError = "Invalid polynomial"

//InvalidSignature is when an invalid tuple is asked to be verified
const InvalidSignature = "Invalid signature: Signature could not be verified with the public key"

//InvalidPrivateKey is if the marshalled private key is not of the right size
const InvalidPrivateKey = "Invalid size of secret key"

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

func checkSmallPublicKey(pk []byte, ctx *newhope.Context) (*glyph_small.PublicKey, error) {
	size := glyph_small.PublicKeySize
	l := len(pk)
	if size != l {
		return nil, xerrors.New("Invalid public key size")
	}
	t := ctx.NewPoly()
	key, e := t.UnMarshalBinary(pk)
	if e != nil {
		return nil, e
	}

	return glyph_small.NewPublicKey(key), nil
}

func checkSignature(sig []byte, ctx *ring.Context) (*glyph.Signature, error) {
	size := glyph.SignatureSize
	l := uint64(len(sig))
	if l != size {
		return nil, xerrors.New("Invalid signature length")
	}
	polySize := glyph.PolySize
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
	c, e3 = glyph.UnmarshallSignature(sig[2*polySize : l])
	if e3 != nil {
		return nil, e3
	}
	return glyph.NewSignature(z1, z2, c), nil
}

func checkSmallSignature(sig []byte, ctx *newhope.Context) (*glyph_small.Signature, error) {
	size := glyph_small.SignatureSize
	l := len(sig)
	if l != size {
		return nil, xerrors.New("Invalid signature length")
	}
	polySize := glyph_small.PolySize
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
	c, e3 = glyph_small.UnmarshallSignature(sig[2*polySize : l])
	if e3 != nil {
		return nil, e3
	}
	return glyph_small.NewSignature(z1, z2, c), nil
}

func checkPrivateKey(sk []byte, ctx *ring.Context) (*glyph.PrivateKey, error) {
	polySize := glyph.PolySize
	l := len(sk)
	if l != 2*polySize {
		return nil, xerrors.New(InvalidPrivateKey)
	}
	d1 := sk[0:polySize]
	d2 := sk[polySize : 2*polySize]
	z1, z2 := ctx.NewPoly(), ctx.NewPoly()
	p1, e1 := z1.UnMarshalBinary(d1)
	if e1 != nil {
		return nil, e1
	}
	p2, e2 := z2.UnMarshalBinary(d2)
	if e2 != nil {
		return nil, e2
	}
	return glyph.ConstructPrivateKey(p1, p2, ctx), nil
}

func checkSmallPrivateKey(sk []byte, ctx *newhope.Context) (*glyph_small.PrivateKey, error) {
	polySize := glyph_small.PolySize
	l := len(sk)
	if l != 2*polySize {
		return nil, xerrors.New(InvalidPrivateKey)
	}
	d1 := sk[0:polySize]
	d2 := sk[polySize : 2*polySize]
	z1, z2 := ctx.NewPoly(), ctx.NewPoly()
	p1, e1 := z1.UnMarshalBinary(d1)
	if e1 != nil {
		return nil, e1
	}
	p2, e2 := z2.UnMarshalBinary(d2)
	if e2 != nil {
		return nil, e2
	}
	return glyph_small.ConstructPrivateKey(p1, p2, ctx), nil
}

//Verify takes in a public key, message and signature
//and checks that this is a valid tuple
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
	verified := public.Verify(signature, msg)
	if !verified {
		return xerrors.New(InvalidSignature)
	}
	return nil
}

//Verify takes in public key, message, signature tuple
//and tries to validate it
func (g *GlyphSuite) Verify(pk PublicKey, msg, sig []byte) error {
	ctx := glyph.GetCtx()
	public, e := checkPublicKey(pk, ctx)
	if e != nil {
		return e
	}
	signature, esig := checkSignature(sig, ctx)
	if esig != nil {
		return esig
	}
	verified := public.Verify(signature, msg)
	if !verified {
		return xerrors.New(InvalidSignature)
	}
	return nil
}

//Verify takes in a public key, message, signature tle
//and tries to validate it
func (g *GlyphSmallSuite) Verify(pk PublicKey, msg, sig []byte) error {
	ctx := glyph_small.GetCtx()
	public, e := checkSmallPublicKey(pk, ctx)
	if e != nil {
		return e
	}
	signature, esig := checkSmallSignature(sig, ctx)
	if esig != nil {
		return esig
	}
	verified := public.Verify(signature, msg)
	if !verified {
		return xerrors.New(InvalidSignature)
	}
	return nil
}
