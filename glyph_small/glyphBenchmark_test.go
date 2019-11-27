package glyph_small

import "testing"

func BenchmarkEncodeSparsePolynomial(t *testing.B) {
	msg := []byte("deadbeef")
	ctx := GetCtx()
	sampler := ctx.NewTernarySampler()
	p := ctx.NewPoly()
	sampler.Sample(0.33, p)
	for i := 0; i < t.N; i++ {
		h := hash(p, msg, ctx.N())
		encodeSparsePolynomial(ctx, omega, h)
	}
}

func benchmarkSign(n int, t *testing.B) {
	msg := []byte("deadbeef")
	ctx := GetCtx()
	secretkey, e := NewPrivateKey(ctx, GetA(ctx))
	if e != nil {
		t.FailNow()
	}
	for i := 0; i < n; i++ {
		_, e = secretkey.Sign(msg)
		if e != nil {
			t.FailNow()
		}
	}
}

func benchmarkVerify(n int, t *testing.B) {
	msg := []byte("deadbeef")
	ctx := GetCtx()
	secretkey, e := NewPrivateKey(ctx, GetA(ctx))
	if e != nil {
		t.FailNow()
	}
	sig, err := secretkey.Sign(msg)
	if err != nil {
		t.FailNow()
	}
	pk := secretkey.PK()
	for i := 0; i < n; i++ {
		ok := pk.Verify(sig, msg)
		if !ok {
			t.FailNow()
		}
	}
}

func BenchmarkVerify1(t *testing.B)    { benchmarkVerify(1, t) }
func BenchmarkVerify10(t *testing.B)   { benchmarkVerify(10, t) }
func BenchmarkVerify50(t *testing.B)   { benchmarkVerify(50, t) }
func BenchmarkVerify500(t *testing.B)  { benchmarkVerify(500, t) }
func BenchmarkVerify1000(t *testing.B) { benchmarkVerify(1000, t) }

func BenchmarkSign1(t *testing.B)    { benchmarkSign(1, t) }
func BenchmarkSign10(t *testing.B)   { benchmarkSign(10, t) }
func BenchmarkSign50(t *testing.B)   { benchmarkSign(50, t) }
func BenchmarkSign500(t *testing.B)  { benchmarkSign(500, t) }
func BenchmarkSign1000(t *testing.B) { benchmarkSign(1000, t) }

func benchmarkGenerateKeyPair(n int, t *testing.B) {
	ctx := GetCtx()
	a := GetA(ctx)
	for i := 0; i < n; i++ {
		//generate secret key
		sk, e := NewPrivateKey(ctx, a)
		if e != nil {
			t.FailNow()
		}
		//Generate public key
		sk.PK()
	}
}

func BenchmarkGenerateKeyPair1(t *testing.B) { benchmarkGenerateKeyPair(1, t) }
