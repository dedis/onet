package glyph

import (
	"github.com/ldsec/lattigo/ring"
)

func getSigningPair(ctx *ring.Context) (*ring.Poly, *ring.Poly) {
	sampler := ctx.NewTernarySampler()
	s1 := ctx.NewPoly()
	s2 := ctx.NewPoly()
	sampler.SampleUniform(s1)
	sampler.SampleUniform(s2)
	ctx.NTT(s1, s1)
	ctx.NTT(s2, s2)
	return s1, s2
}

func ConstructPrivateKey(s, e *ring.Poly, ctx *ring.Context) *PrivateKey {
	return &PrivateKey{
		s: s,
		e: e,
		a: GetA(ctx),
	}
}

func NewPrivateKey(ctx *ring.Context, a *ring.Poly) (*PrivateKey, error) {
	s, e := getSigningPair(ctx)
	antt := ctx.NewPoly()
	antt.Copy(a)
	return &PrivateKey{
		s:   s,
		e:   e,
		a:   antt,
		ctx: ctx,
	}, nil
}

func (sk *PrivateKey) PK() *PublicKey {
	ctx := GetCtx()
	s1 := sk.GetS()
	s2 := sk.GetE()
	s1.Copy(sk.GetS())
	s2.Copy(sk.GetE())
	a := GetA(ctx)
	pkPol := ctx.NewPoly()
	ctx.MulCoeffs(a, s1, pkPol)
	ctx.Add(pkPol, s2, pkPol)
	return &PublicKey{
		t:   pkPol,
		a:   a,
		ctx: ctx,
	}
}
