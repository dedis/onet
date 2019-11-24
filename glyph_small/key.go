package glyph_small

import (
	"github.com/lca1/lattigo/newhope"
)

func getSigningPair(ctx *newhope.Context) (*newhope.Poly, *newhope.Poly) {
	sampler := ctx.NewTernarySampler()
	s1 := ctx.NewPoly()
	s2 := ctx.NewPoly()
	sampler.Sample(0.33, s1)
	sampler.Sample(0.33, s2)
	//sampler.SampleUniform(s1)
	//sampler.SampleUniform(s2)
	return s1, s2
}

func ConstructPrivateKey(s, e *newhope.Poly, ctx *newhope.Context) *PrivateKey {
	return &PrivateKey{
		s: s,
		e: e,
		a: GetA(ctx),
	}
}

func NewPrivateKey(ctx *newhope.Context, a *newhope.Poly) (*PrivateKey, error) {
	s, e := getSigningPair(ctx)
	//ctx.InvNTT(s, s)
	//ctx.InvNTT(e, e)
	antt := ctx.NewPoly()
	copy(antt.Coeffs, a.Coeffs)
	return &PrivateKey{
		s:   s,
		e:   e,
		a:   antt,
		ctx: ctx,
	}, nil
}

func (sk *PrivateKey) PK() *PublicKey {
	ctx := GetCtx()
	s1 := ctx.NewPoly()
	s2 := ctx.NewPoly()
	//s1 := sk.GetS()
	//s2 := sk.GetE()
	copy(s1.Coeffs, sk.GetS().Coeffs)
	copy(s2.Coeffs, sk.GetE().Coeffs)

	//s1.Copy(sk.GetS())
	//s2.Copy(sk.GetE())
	ctx.NTT(s1, s1)
	ctx.NTT(s2, s2)
	a := GetA(ctx)
	pkPol := ctx.NewPoly()
	ctx.MulCoeffs(a, s1, pkPol)
	ctx.Add(pkPol, s2, pkPol)
	ctx.InvNTT(pkPol, pkPol)
	//ctx.InvNTT(s1, s1)
	//ctx.InvNTT(s2, s2)
	return &PublicKey{
		t:   pkPol,
		a:   a,
		ctx: ctx,
	}
}
