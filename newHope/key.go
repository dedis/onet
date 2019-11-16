package newHope

import (
	"io"

	"go.dedis.ch/onet/v3/glyph"

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

func GenerateKey(rand io.Reader) (PublicKey, PrivateKey, error) {
	if rand != nil {
		//TODO: Use it
		secretBuffer := make([]byte, NewHopePrivateKeySize)
		_, e := rand.Read(secretBuffer)
		if e != nil {
			return nil, nil, e
		}
		//TODO: make this an actual thing
		return nil, nil, nil
	}
	ctx := glyph.GetCtx()
	private, e := glyph.NewPrivateKey(ctx, glyph.GetA(ctx))
	if e != nil {
		return nil, nil, e
	}
	public := private.PK()
	publicData, e1 := public.Marshall()
	if e1 != nil {
		return nil, nil, e1
	}
	/*t, ep := checkPublicKey(publicData, ctx)
	if ep != nil {
		return nil, nil, ep
	}*/
	if !ctx.Equal(public.GetT(), private.PK().GetT()) {
		panic("LOL")
	}
	privateData, e2 := private.Marshall()
	if e2 != nil {
		return nil, nil, e2
	}
	return publicData, privateData, nil
}
