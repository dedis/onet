package newHope

import (
	"go.dedis.ch/onet/v3/glyph"
	"go.dedis.ch/onet/v3/glyph_small"
)

func Sign(sk PrivateKey, msg []byte) ([]byte, error) {
	ctx := glyph.GetCtx()
	secret, e := checkPrivateKey(sk, ctx)
	if e != nil {
		return nil, e
	}
	sig, e := secret.Sign(msg)
	if e != nil {
		return nil, e
	}
	return sig.Marshall()
}

//Sign signs a message using glyph and large coefficients
func (g *GlyphSuite) Sign(sk PrivateKey, msg []byte) ([]byte, error) {
	ctx := glyph.GetCtx()
	secret, e := checkPrivateKey(sk, ctx)
	if e != nil {
		return nil, e
	}
	sig, e := secret.Sign(msg)
	if e != nil {
		return nil, e
	}
	return sig.Marshall()
}

//Sign signs a message using glyph with small coefficients
func (g *GlyphSmallSuite) Sign(sk PrivateKey, msg []byte) ([]byte, error) {
	ctx := glyph_small.GetCtx()
	secret, e := checkSmallPrivateKey(sk, ctx)
	if e != nil {
		return nil, e
	}
	sig, e := secret.Sign(msg)
	if e != nil {
		return nil, e
	}
	return sig.Marshall()
}
