package newHope

import (
	"fmt"
	"testing"

	"go.dedis.ch/onet/v3/glyph_small"
)

func comparePolies(coeffs1, coeffs2 []uint32, t *testing.T) {
	l1, l2 := len(coeffs1), len(coeffs2)
	if l1 != l2 {
		t.Log("Polynomials are not of the same size", l1, l2)
		t.FailNow()
	}
	for j, c1 := range coeffs1 {
		c2 := coeffs2[j]
		if c1 != c2 {
			t.Log("Not all coefficients match")
			t.FailNow()
		}
	}
}

func TestPolyMarshallSmall(t *testing.T) {
	ctx := glyph_small.GetCtx()
	p := ctx.NewUniformPoly()
	pub := glyph_small.NewPublicKey(p)
	pd, e := pub.Marshall()
	if e != nil {
		t.FailNow()
	}
	pk, e2 := checkSmallPublicKey(pd, ctx)
	if e2 != nil {
		t.FailNow()
	}
	comparePolies(pk.GetT().Coeffs, pub.GetT().Coeffs, t)
}

func TestSecretMarshallSmall(t *testing.T) {
	ctx := glyph_small.GetCtx()
	sk, e := glyph_small.NewPrivateKey(ctx, glyph_small.GetA(ctx))
	if e != nil {
		fmt.Println("Could not generate sk")
		t.FailNow()
	}
	z1 := sk.GetS()
	z2 := sk.GetE()
	m, e2 := sk.Marshall()
	if e2 != nil {
		t.FailNow()
	}
	sk2, e3 := checkSmallPrivateKey(m, ctx)
	if e3 != nil {
		fmt.Println("Lukas")
		t.FailNow()
	}
	z12 := sk2.GetS()
	z22 := sk2.GetE()
	comparePolies(z1.Coeffs, z12.Coeffs, t)
	comparePolies(z2.Coeffs, z22.Coeffs, t)

	pk1 := sk.PK()
	pk2 := sk2.PK()
	comparePolies(pk1.GetT().Coeffs, pk2.GetT().Coeffs, t)
	fmt.Println("Bjo")
}

func TestMarshallSmall(t *testing.T) {
	suite := &GlyphSmallSuite{}
	pub, priv, e := suite.GenerateKey(nil)
	if e != nil {
		t.Log(e)
		t.FailNow()
		return
	}
	if priv == nil || pub == nil {
		t.Log("Either key was nil")
		t.FailNow()
	}
	ctx := glyph_small.GetCtx()
	publicKey, epublicKey := checkSmallPublicKey(pub, ctx)
	if epublicKey != nil {
		t.Log(epublicKey)
		t.FailNow()
	}
	if publicKey == nil {
		t.Log("Public key was nil")
		t.FailNow()
	}

	private, eprivate := checkSmallPrivateKey(priv, ctx)
	if eprivate != nil {
		t.Log(eprivate)
		t.FailNow()
	}
	if private == nil {
		t.Log("Private key was nil")
		t.FailNow()
	}
	testPublic := private.PK()
	if testPublic == nil {
		t.Log("Failed to generate public key")
		t.FailNow()
	}
	comparePolies(testPublic.GetT().Coeffs, publicKey.GetT().Coeffs, t)
}
