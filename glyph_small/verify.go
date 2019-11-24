package glyph_small

func (pk *PublicKey) Verify(sig *Signature, msg []byte) bool {
	ctx := GetCtx()
	z1 := ctx.NewPoly()
	z2 := ctx.NewPoly()
	//z1.Coeffs = sig.z1.Coeffs
	//z2.Coeffs = sig.z2.Coeffs
	copy(z1.Coeffs, sig.z1.Coeffs)
	copy(z2.Coeffs, sig.z2.Coeffs)
	a := GetA(ctx)
	ctx.NTT(z1, z1)
	ctx.NTT(z2, z2)
	c := ctx.NewPoly()
	copy(c.Coeffs, sig.c.Coeffs)
	//c.Coeffs = sig.c.Coeffs
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
	//t.Coeffs = pk.GetT().Coeffs
	copy(t.Coeffs, pk.GetT().Coeffs)
	ctx.NTT(t, t)
	tc := ctx.NewPoly()
	ctx.MulCoeffs(t, c, tc)
	ctx.Sub(az1z2, tc, az1z2tc)
	ctx.InvNTT(az1z2tc, az1z2tc)
	az1z2tc.Coeffs = kfloor(az1z2tc.Coeffs)
	dp := hash(az1z2tc, msg, ctx.N())
	d := encodeSparsePolynomial(ctx, omega, dp)
	ctx.InvNTT(c, c)
	//fmt.Println(c.Coeffs)
	//fmt.Println(d.Coeffs)
	for j, coeff := range sig.c.Coeffs {
		coeff2 := d.Coeffs[j]
		if coeff != coeff2 {
			return false
		}
	}
	return true
}
