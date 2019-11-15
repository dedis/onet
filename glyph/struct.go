package glyph

import (
	"github.com/ldsec/lattigo/ring"
	"go.dedis.ch/onet/v3/newHope"
)

type PrivateKey struct {
	pols *newHope.PrivateKeyPoly
}

type Signature struct {
	z1 *ring.Poly
	z2 *ring.Poly
	c  *ring.Poly
}

type PublicKey struct {
	ctx  *ring.Context
	pols *newHope.PublicKeyPoly
}
