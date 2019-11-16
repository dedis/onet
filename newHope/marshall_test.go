package newHope

import (
	"testing"

	"go.dedis.ch/onet/v3/glyph"
)

func TestMarshall(t *testing.T) {
	priv, pub, e := GenerateKey(nil)
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
	tst, etest := private.Marshall()
	if etest != nil {
		t.Log("PRivate key is not marshalling correctly")
		t.FailNow()
	}
	for i, b := range tst {
		if priv[i] != b {
			t.Log("PRivate key is not marshalling correctly")
			t.FailNow()
		}
	}
	testPublic := private.PK()
	if testPublic == nil {
		t.Log("Failed to generate public key")
		t.FailNow()
	}
	if !ctx.Equal(testPublic.GetT(), publicKey.GetT()) {
		t.Log("Unmarshalled public key is not equal to public key from the unmarshalled private key")
		t.FailNow()
	}
}
