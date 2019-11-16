package glyph

import (
	"github.com/ldsec/lattigo/ring"
)

type PrivateKey struct {
	s   *ring.Poly
	e   *ring.Poly
	a   *ring.Poly
	ctx *ring.Context
}

type Signature struct {
	z1 *ring.Poly
	z2 *ring.Poly
	c  *ring.Poly
}

type PublicKey struct {
	t   *ring.Poly
	a   *ring.Poly
	ctx *ring.Context
}

func NewPublicKey(t *ring.Poly) *PublicKey {
	ctx := GetCtx()
	a := GetA(ctx)
	return &PublicKey{
		t:   t,
		a:   a,
		ctx: GetCtx(),
	}
}

func (pk *PublicKey) GetT() *ring.Poly {
	return pk.t
}

func (pk *PrivateKey) GetS() *ring.Poly {
	return pk.s
}

func (pk *PrivateKey) GetE() *ring.Poly {
	return pk.e
}

func (pk *PrivateKey) GetA() *ring.Poly {
	return pk.a
}

func (pk *PublicKey) GetA() *ring.Poly {
	return pk.a
}

func (pk *PrivateKey) GetCtx() *ring.Context {
	return pk.ctx
}

func GetA(ctx *ring.Context) *ring.Poly {
	a := ctx.NewPoly()
	a.SetCoefficients([][]uint64{constA[:]})
	return a
}

func GetCtx() *ring.Context {
	N := constN
	Q := constQ
	contextT := ring.NewContext()
	contextT.SetParameters(N, []uint64{Q})
	contextT.GenNTTParams()
	return contextT
}
