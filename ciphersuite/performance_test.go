package ciphersuite

import (
	"io"
	"testing"
)

func benchmarkGenerateKey(suite CipherSuite, rand io.Reader, b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _, e := suite.GenerateKeyPair(rand)
		if e != nil {
			b.FailNow()
		}
	}
}

func BenchmarkGenerateKeyNewHope(b *testing.B) {
	suite := NewNewHopeCipherSuite()
	benchmarkGenerateKey(suite, nil, b)
}

func BenchmarkGenerateKeyEd25519(b *testing.B) {
	suite := NewEd25519CipherSuite()
	benchmarkGenerateKey(suite, nil, b)
}

func BenchmarkGenerateKeyNewHopeSmall(b *testing.B) {
	suite := NewNewHopeCipherSuiteSmall()
	benchmarkGenerateKey(suite, nil, b)
}

func benchmarkSign(suite CipherSuite, rand io.Reader, msg []byte, b *testing.B) {
	_, sk, err := suite.GenerateKeyPair(rand)
	if err != nil {
		b.FailNow()
	}
	for i := 0; i < b.N; i++ {
		_, e := suite.Sign(sk, msg)
		if e != nil {
			b.FailNow()
		}
	}
}

func BenchmarkSignEd25519(b *testing.B) {
	suite := NewEd25519CipherSuite()
	msg := []byte("deadbeef")
	benchmarkSign(suite, nil, msg, b)
}

func BenchmarkSignNewHope(b *testing.B) {
	suite := NewHope()
	msg := []byte("deadbeef")
	benchmarkSign(suite, nil, msg, b)
}

func BenchmarkSignNewHopeSmall(b *testing.B) {
	suite := NewHopeSmall()
	msg := []byte("deadbeef")
	benchmarkSign(suite, nil, msg, b)
}

func benchmarkVerify(suite CipherSuite, rand io.Reader, msg []byte, b *testing.B) {
	pk, sk, e := suite.GenerateKeyPair(rand)
	if e != nil {
		b.Log(e)
		b.FailNow()
	}
	sig, esign := suite.Sign(sk, msg)
	if esign != nil {
		b.FailNow()
	}

	for i := 0; i < b.N; i++ {
		everify := suite.Verify(pk, sig, msg)
		if everify != nil {
			b.Log(everify)
			b.FailNow()
		}
	}
}

func BenchmarkVerifyEd25519(b *testing.B) {
	suite := NewEd25519CipherSuite()
	msg := []byte("deadbeef")
	benchmarkVerify(suite, nil, msg, b)
}

func BenchmarkVerifyNewHope(b *testing.B) {
	suite := NewHope()
	msg := []byte("deadbeef")
	benchmarkVerify(suite, nil, msg, b)
}

func BenchmarkVerifyNewHopeSmall(b *testing.B) {
	suite := NewHopeSmall()
	msg := []byte("deadbeef")
	benchmarkVerify(suite, nil, msg, b)
}
