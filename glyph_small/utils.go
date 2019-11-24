package glyph_small

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"
	"errors"

	"github.com/lca1/lattigo/newhope"
	"github.com/ldsec/lattigo/ring"
)

const digestSize uint32 = 32

//hash is a hash function that takes in a polynomial
//and returns a digest for that given polynomial.
func hash(u *newhope.Poly, mu []byte, N uint32) [digestSize]byte {
	bytesPerPoly := N * 2
	coeffs := u.Coeffs
	hashInput := make([]byte, bytesPerPoly+uint32(len(mu)))
	for i, x := range coeffs {
		binary.LittleEndian.PutUint16(hashInput[2*i:], uint16(x))
	}
	copy(hashInput[bytesPerPoly:], mu)
	return sha256.Sum256(hashInput)
}

func abs(x, q uint32) uint32 {
	qdiv2 := q / 2
	if x <= qdiv2 {
		return x
	}
	return q - x
}

//kfloor TODO: check how it is supposed to work with multiple sets
//of coefficients.
func kfloor(f []uint32) []uint32 {
	/*integer division by  2*K+1 where K = B - omega */
	buf := make([]uint32, len(f))
	for i, vf := range f {
		buf[i] = vf / (2*(constB-omega) + 1)
	}
	return buf
}

func hashToRand(iv uint64, h [32]byte) cipher.Stream {
	little := make([]byte, 8)
	big := make([]byte, 8)
	IV := make([]byte, len(little)+len(big))
	binary.LittleEndian.PutUint64(little, iv)
	binary.BigEndian.PutUint64(big, iv)
	copy(IV[:8], little)
	copy(IV[8:], big)
	c, e := aes.NewCipher(h[:])
	if e != nil {
		panic(e)
	}
	return cipher.NewCTR(c, IV)
}

var zero8 = make([]byte, 8)

func nextRandUint64(stream cipher.Stream) uint32 {
	out := make([]byte, 8)
	stream.XORKeyStream(out, zero8)
	return binary.LittleEndian.Uint32(out)
}

func compressElements(u, v, Q uint32) (uint32, error) {
	k := constB - omega
	if abs(v, Q) > k {
		return 0, errors.New("invalid v")
	}
	kfloorUV := ((u + v) % Q) / (2*k + 1)
	kfloorU := u / (2*k + 1)

	if kfloorUV == kfloorU {
		return 0, nil
	}
	if u < k {
		return Q - k, nil
	}
	if (u >= Q-k) && sign(v, Q) > 0 {
		return k, nil
	}
	if kfloorUV < kfloorU {
		return Q - k, nil
	}
	return k, nil
}

func compress(p1, p2 []uint32, N, Q uint32) ([]uint32, error) {
	p3 := make([]uint32, N)
	for i := uint32(0); i < N; i++ {
		ele, err := compressElements(p1[i], p2[i], Q)
		if err != nil {
			return nil, errors.New("Couldn't compress the polynomial")
		}
		p3[i] = ele
	}
	return p3, nil
}

func sign(x, Q uint32) int {
	if x == 0 {
		return 0
	}
	if 2*x <= Q {
		return 1
	}
	return -1
}

//mulAndSubtract return a*z1 - t*c
func mulAndSubtract(ctx *ring.Context, a, z1, t, c *ring.Poly) *ring.Poly {
	mul := ctx.NewPoly()
	ctx.MulCoeffs(a, z1, mul)
	mul2 := ctx.NewPoly()
	ctx.MulCoeffs(t, c, mul2)
	sub := ctx.NewPoly()
	ctx.Sub(mul, mul2, sub)
	return sub
}
