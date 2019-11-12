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
	return s1, s2
}
