package newHope

import (
	"fmt"
	"testing"

	"go.dedis.ch/onet/v3/glyph"
)

func TestPolyMarshall(t *testing.T) {
	ctx := glyph.GetCtx()
	p := ctx.NewUniformPoly()
	pub := glyph.NewPublicKey(p)
	pd, e := pub.Marshall()
	if e != nil {
		fmt.Println("Bjozzi")
		t.FailNow()
	}
	pk, e2 := checkPublicKey(pd, ctx)
	if e2 != nil {
		fmt.Println("Bjoggi")
		t.FailNow()
	}
	if !ctx.Equal(pk.GetT(), pub.GetT()) {
		fmt.Println("WTF")
		t.FailNow()
	}
}

func TestSecretMarshall(t *testing.T) {
	ctx := glyph.GetCtx()
	sk, e := glyph.NewPrivateKey(ctx, glyph.GetA(ctx))
	if e != nil {
		fmt.Println("Could not generate sk")
		t.FailNow()
	}
	z1 := sk.GetS()
	z2 := sk.GetE()
	m, e2 := sk.Marshall()
	if e2 != nil {
		fmt.Println("Bjorninn")
		t.FailNow()
	}
	sk2, e3 := checkPrivateKey(m, ctx)
	if e3 != nil {
		fmt.Println("Lukas")
		t.FailNow()
	}
	z12 := sk2.GetS()
	z22 := sk2.GetE()
	if !ctx.Equal(z1, z12) {
		t.Log("Z1 did not equal")
		t.Fail()
	}

	if !ctx.Equal(z2, z22) {
		t.Log("Z2 did not equal")
		t.Fail()
	}

	pk1 := sk.PK()
	pk2 := sk2.PK()
	if !ctx.Equal(pk1.GetT(), pk2.GetT()) {
		t.Log("PK did not equal")
		t.Fail()
	}
	fmt.Println("Bjo")
}

func TestMarshall(t *testing.T) {
	pub, priv, e := GenerateKey(nil)
	if e != nil {
		t.Log(e)
		t.FailNow()
		return
	}
	if priv == nil || pub == nil {
		t.Log("Either key was nil")
		t.FailNow()
	}
	ctx := glyph.GetCtx()
	publicKey, epublicKey := checkPublicKey(pub, ctx)
	if epublicKey != nil {
		t.Log(epublicKey)
		t.FailNow()
	}
	if publicKey == nil {
		t.Log("Public key was nil")
		t.FailNow()
	}

	private, eprivate := checkPrivateKey(priv, ctx)
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
	if !ctx.Equal(testPublic.GetT(), publicKey.GetT()) {
		fmt.Println(testPublic.GetT().GetCoefficients()[0][:20], publicKey.GetT().GetCoefficients()[0][:20])
		t.Log("Unmarshalled public key is not equal to public key from the unmarshalled private key")
		t.FailNow()
	}
}
