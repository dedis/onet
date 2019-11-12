package glyph

import "fmt"

func (pk *PublicKey) Verify(sig *Signature, msg []byte) bool {
	ctx := pk.ctx
	z1 := ctx.NewPoly()
	z2 := ctx.NewPoly()
	z1.Copy(sig.z1)
	z2.Copy(sig.z2)
	a := pk.a
	ctx.NTT(z1, z1)
	ctx.NTT(z2, z2)
	h := ctx.NewPoly()
	ctx.MulCoeffs(a, z1, h)
	ctx.Add(h, z2, h)
	ctx.InvNTT(h, h)
	tc := sparseMul(sig.c, pk.t, ctx, omega)
	ctx.Sub(h, tc, h)
	sss := kfloor(h.GetCoefficients())
	h.SetCoefficients(sss)
	hashOutput := hash(h, msg, ctx.N)
	sparse := encodeSparsePolynomial(ctx, omega, hashOutput)
	sparseCoefficients := sparse.GetCoefficients()
	coeffs := sig.c.GetCoefficients()
	l1 := len(coeffs)
	l2 := len(sparseCoefficients)
	if l1 != l2 {
		return false
	}
	for i := 0; i < l1; i++ {
		c1 := coeffs[i]
		c2 := sparseCoefficients[i]
		l11 := len(c1)
		l22 := len(c2)
		if l11 != l22 {
			fmt.Println("Yo")
			return false
		}
		for j := 0; j < l11; j++ {
			if c1[j] != c2[j] {
				fmt.Println("J: ", j)
				return false
			}
		}
	}
	return true
}
