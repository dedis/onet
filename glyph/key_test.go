package glyph

import (
	"fmt"
	"testing"

	"github.com/ldsec/lattigo/ring"
)

/*
	Test to make sure that the key generation is
	working like is expected.
*/
func TestKey(t *testing.T) {
	fmt.Println("TestKey")
	for i := uint64(0); i < 1; i++ {
		N := uint64(2 << (12 + i))
		T := uint64(65537)
		//sigma := 3.19
		contextT := ring.NewContext()
		contextT.SetParameters(N, []uint64{T})
		contextT.GenNTTParams()
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
	i := 1
	N := uint64(2 << (12 + i))
	T := uint64(65537)
	//sigma := 3.19
	contextT := ring.NewContext()
	contextT.SetParameters(N, []uint64{T, T})
	contextT.GenNTTParams()
	var h1 [32]byte
	h := [][32]byte{h1, h1}
	w := uint64(16)
	sparse := encodeSparsePolynomial(contextT, w, h)
	sparse2 := encodeSparsePolynomial(contextT, w, h)
	for i, c1 := range sparse.GetCoefficients() {
		c2 := sparse2.GetCoefficients()[i]
		for j := range c1 {
			if c1[j] != c2[j] {
				t.Fatal()
			}
		}
	}
	coeffs := sparse.GetCoefficients()
	check := func(coeff []uint64, Q, omega uint64) bool {
		counter := uint64(0)
		for _, v := range coeff {
			if v == 1 || v == Q-1 {
				counter++
			}
		}
		return omega == counter
	}
	for j, c := range coeffs {
		q := contextT.Modulus[j]
		if !check(c, q, w) {
			t.Fail()
		}

	}
}

func TestSign1(t *testing.T) {
	message := []byte("Bjorn")
	contextT := GetCtx()
	a := contextT.NewPoly()
	a.SetCoefficients([][]uint64{constA[:]})
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
		fmt.Println("Bjo")
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
