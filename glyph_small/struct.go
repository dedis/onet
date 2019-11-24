package glyph_small

import (
	"github.com/lca1/lattigo/newhope"
)

type PrivateKey struct {
	s   *newhope.Poly
	e   *newhope.Poly
	a   *newhope.Poly
	ctx *newhope.Context
}

type Signature struct {
	z1 *newhope.Poly
	z2 *newhope.Poly
	c  *newhope.Poly
}

type PublicKey struct {
	t   *newhope.Poly
	a   *newhope.Poly
	ctx *newhope.Context
}

func NewPublicKey(t *newhope.Poly) *PublicKey {
	ctx := GetCtx()
	a := GetA(ctx)
	return &PublicKey{
		t:   t,
		a:   a,
		ctx: GetCtx(),
	}
}

func (pk *PublicKey) GetT() *newhope.Poly {
	return pk.t
}

func (pk *PrivateKey) GetS() *newhope.Poly {
	return pk.s
}

func (pk *PrivateKey) GetE() *newhope.Poly {
	return pk.e
}

func (pk *PrivateKey) GetA() *newhope.Poly {
	return pk.a
}

func (pk *PublicKey) GetA() *newhope.Poly {
	return pk.a
}

func (pk *PrivateKey) GetCtx() *newhope.Context {
	return pk.ctx
}

func GetA(ctx *newhope.Context) *newhope.Poly {
	a := ctx.NewPoly()
	A := make([]uint32, len(constA))
	copy(A, constA[:])
	a.Coeffs = A
	return a
}

func GetCtx() *newhope.Context {
	N := constN
	Q := constQ
	contextT, e := newhope.NewContext(N, Q)
	if e != nil {
		panic(e)
	}
	return contextT
}
