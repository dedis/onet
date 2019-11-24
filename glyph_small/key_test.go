package glyph_small

import (
	"fmt"
	"testing"
)

/*
	Test to make sure that the key generation is
	working like is expected.
*/
func TestKey(t *testing.T) {
	fmt.Println("TestKey")
	for i := uint64(0); i < 1; i++ {
		//Qi := Qi60[uint64(len(Qi60))-2<<i:]
		//Pi := Pi60[uint64(len(Pi60))-((2<<i)+1):]
		//sigma := 3.19
		contextT := GetCtx()

		//contextQ := ring.NewContext()
		//contextQ.SetParameters(N, Qi)
		//contextQ.GenNTTParams()
		//
		//contextP := ring.NewContext()
		//contextP.SetParameters(N, Pi)
		//contextP.GenNTTParams()
		pk, e := NewPrivateKey(contextT, contextT.NewPoly())
		if e != nil {
			t.Fail()
		}
		pub := pk.PK()
		if pub == nil {
			t.Fail()
		}
	}
}

func TestSparsePolyGeneration(t *testing.T) {
	contextT := GetCtx()
	var h [32]byte
	w := uint32(16)
	sparse := encodeSparsePolynomial(contextT, w, h)
	sparse2 := encodeSparsePolynomial(contextT, w, h)

	for j, coeff := range sparse.Coeffs {
		coeff2 := sparse2.Coeffs[j]
		if coeff != coeff2 {
			t.FailNow()
		}
	}
	check := func(coeff []uint32, Q, omega uint32) bool {
		counter := uint32(0)
		for _, v := range coeff {
			if v == 1 || v == Q-1 {
				counter++
			}
		}
		return omega == counter
	}
	if !check(sparse.Coeffs, constQ, w) {
		t.FailNow()
	}
}

func TestSign1(t *testing.T) {
	message := []byte("Bjorn")
	contextT := GetCtx()
	a := GetA(contextT)
	pk, e := NewPrivateKey(contextT, a)
	if e != nil {
		t.Log("Failed to make private key")
		t.Fail()
	}
	sig, err := pk.Sign(message)
	if err != nil {
		t.Fail()
	}
	if sig == nil {
		t.Log("Signature failed")
		t.Fail()
	}
	pub := pk.PK()
	pass := pub.Verify(sig, message)
	if !pass {
		t.Log("Signature does not match")
		t.Fail()
	}
	fmt.Println("Sign")
}
