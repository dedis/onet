package glyph

func (pk *PublicKey) Verify(sig *Signature, msg []byte) bool {
	ctx := pk.ctx
	z1 := ctx.NewPoly()
	z2 := ctx.NewPoly()
	z1.SetCoefficients(sig.z1.GetCoefficients())
	z2.SetCoefficients(sig.z2.GetCoefficients())
	a := ctx.NewPoly()
	a.Copy(GetA(ctx))
	ctx.NTT(z1, z1)
	ctx.NTT(z2, z2)
	c := ctx.NewPoly()
	c.Copy(sig.c)
	ctx.NTT(c, c)
	az1z2 := ctx.NewPoly()
	az1 := ctx.NewPoly()
	ctx.MulCoeffs(a, z1, az1)
	ctx.Add(az1, z2, az1z2)
	//ctx.InvNTT(az1z2, az1z2)
	//calculating a * z1 + z2 - tc
	az1z2tc := ctx.NewPoly()
	//this is a *s + e the public key
	t := ctx.NewPoly()
	t.SetCoefficients(pk.GetT().GetCoefficients())
	ctx.NTT(t, t)
	tc := ctx.NewPoly()
	ctx.MulCoeffs(t, c, tc)
	ctx.Sub(az1z2, tc, az1z2tc)
	ctx.InvNTT(az1z2tc, az1z2tc)
	az1z2tc.SetCoefficients(kfloor(az1z2tc.GetCoefficients()))
	dp := hash(az1z2tc, msg, ctx.N)
	d := encodeSparsePolynomial(ctx, omega, dp)
	ctx.InvNTT(c, c)
	for i, coeffs := range d.GetCoefficients() {
		c2 := sig.c.GetCoefficients()[i]
		for j, cof1 := range coeffs {
			cof2 := c2[j]
			if cof1 != cof2 {
				return false
			}
		}
	}
	return true
}
