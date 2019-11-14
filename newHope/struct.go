package newHope

import "github.com/ldsec/lattigo/ring"

const NewHopeName = "New Hope"

const sizeOfCoefficient = 8 //Assuming the coefficients are uint64 so 64 / 8

const NewHopePublicKeySize = numberOfModulie * numberOfCoefficients * sizeOfCoefficient

const numberOfModulie = 1

const numberOfCoefficients = 1024

const NewHopePrivateKeySize = numberOfModulie * 2 * numberOfCoefficients * sizeOfCoefficient

const NewHopeSignatureSize = numberOfModulie * 3 * numberOfCoefficients * sizeOfCoefficient

type PublicKey []byte

type PrivateKey []byte

type PrivateKeyPoly struct {
	s   *ring.Poly
	e   *ring.Poly
	a   *ring.Poly
	ctx *ring.Context
}

func (pk *PrivateKeyPoly) GetS() *ring.Poly {
	return pk.s
}

func (pk *PrivateKeyPoly) GetE() *ring.Poly {
	return pk.e
}

func (pk *PrivateKeyPoly) GetA() *ring.Poly {
	return pk.a
}

type SignaturePoly struct {
	z1 *ring.Poly
	z2 *ring.Poly
	c  *ring.Poly
}

type PublicKeyPoly struct {
	t   *ring.Poly
	a   *ring.Poly
	ctx *ring.Context
}
