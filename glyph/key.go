package glyph

import (
	"github.com/ldsec/lattigo/ring"
	"go.dedis.ch/onet/v3/newHope"
)

func getSigningPair(ctx *ring.Context) (*ring.Poly, *ring.Poly) {
	sampler := ctx.NewTernarySampler()
	s1 := ctx.NewPoly()
	s2 := ctx.NewPoly()
	sampler.SampleUniform(s1)
	sampler.SampleUniform(s2)
	return s1, s2
}

func NewPrivateKey(ctx *ring.Context, a *ring.Poly) (*PrivateKey, error) {
	antt := ctx.NewPoly()
	antt.Copy(a)
	pk, err := newHope.NewPrivateKey(ctx, antt)
	if err != nil {
		return nil, err
	}
	return &PrivateKey{
		pols: pk,
	}, nil
}

func (pk *PrivateKey) PK() *PublicKey {
	k := pk.pols.PK()
	key := &PublicKey{
		pols: k,
		ctx:  pk.pols.GetCtx(),
	}
	return key
}
