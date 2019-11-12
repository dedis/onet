package glyph

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"
	"errors"

	"github.com/ldsec/lattigo/ring"
)

type sparsePoly struct {
	pos  uint16
	sign bool
}

type sparsePolyST [omega]sparsePoly

const digestSize uint32 = 32

//hash is a hash function that takes in a polynomial
//and returns a digest for that given polynomial.
func hash(u *ring.Poly, mu []byte, N uint64) [][digestSize]byte {
	bytesPerPoly := N * 2
	coeffs := u.GetCoefficients()
	hashes := make([][digestSize]byte, 0)
	for _, pol := range coeffs {
		hashInput := make([]byte, bytesPerPoly+uint64(len(mu)))
		for i, x := range pol {
			binary.LittleEndian.PutUint16(hashInput[2*i:], uint16(x))
		}
		copy(hashInput[bytesPerPoly:], mu)
		hashes = append(hashes, sha256.Sum256(hashInput))
	}
	return hashes
}

func abs(x, q uint64) uint64 {
	qdiv2 := q / 2
	if x <= qdiv2 {
		return x
	}
	return q - x
}

//GLPPoly takes in a set of coefficients that
func GLPPoly(coeffs []uint64, Q uint64) []uint64 {
	l := len(coeffs)
	buffer := make([]uint64, l)
	for i, coeff := range coeffs {
		r := coeff
		for {
			r = uint64(r & 3)
			r >>= 2
			if r != 3 {
				break
			}
		}
		switch r {
		case 0:
			buffer[i] = 0
		case 1:
			buffer[i] = 1
		case 2:
			buffer[i] = Q - 1
		case 3:
			panic("Invalid: Something went wrong")
		}
	}
	return buffer
}

//kfloor TODO: check how it is supposed to work with multiple sets
//of coefficients.
func kfloor(fs [][]uint64) [][]uint64 {
	/*integer division by  2*K+1 where K = B - omega */
	temp := make([][]uint64, len(fs))
	for j, f := range fs {
		buf := make([]uint64, len(f))
		for i, vf := range f {
			buf[i] = vf / (2*(constB-omega) + 1)
		}
		temp[j] = buf
	}
	return temp
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

func nextRandUint64(stream cipher.Stream) uint64 {
	out := make([]byte, 8)
	stream.XORKeyStream(out, zero8)
	return binary.LittleEndian.Uint64(out)
}

func compressElements(u, v, Q uint64) (uint64, error) {
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

func compress(p1, p2 []uint64, N, Q uint64) ([]uint64, error) {
	p3 := make([]uint64, N)
	for i := uint64(0); i < N; i++ {
		ele, err := compressElements(p1[i], p2[i], Q)
		if err != nil {
			return nil, errors.New("Couldn't compress the polynomial")
		}
		p3[i] = ele
	}
	return p3, nil
}

func sign(x, Q uint64) int {
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
