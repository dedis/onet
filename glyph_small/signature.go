package glyph_small

import (
	"context"
	"errors"
	"time"

	"github.com/AidosKuneen/numcpu"
	"github.com/lca1/lattigo/newhope"
)

/*
*Glyph signature algorithm
*
 */

func NewSignature(z1, z2, c *newhope.Poly) *Signature {
	return &Signature{
		z1: z1,
		z2: z2,
		c:  c,
	}
}

func (pk *PrivateKey) Sign(m []byte) (*Signature, error) {
	notify := make(chan *Signature, numcpu.NumCPU())
	ringCtx := GetCtx()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for i := 0; i < numcpu.NumCPU(); i++ {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				y1, y2 := ringCtx.NewUniformPoly(), ringCtx.NewUniformPoly()
				y1Temp := make([]uint32, len(y1.Coeffs))
				for j, v2 := range y1.Coeffs {
					v := v2
					for {
						v &= ^(^0 << (bBits + 1))
						if v <= 2*constB+1 {
							break
						}
					}
					if v > constB {
						v = constQ - (y1.Coeffs[j] - constB)
					}
					y1Temp[j] = v
				}
				y1.Coeffs = y1Temp
				y2Temp := make([]uint32, len(y2.Coeffs))
				for j, v2 := range y2.Coeffs {
					v := v2
					for {
						v &= ^(^0 << (bBits + 1))
						if v <= 2*constB+1 {
							break
						}
					}
					if v > constB {
						v = constQ - (y2.Coeffs[j] - constB)
					}
					y2Temp[j] = v
				}
				y2.Coeffs = y2Temp
				sig, err := pk.deterministicSign(y1, y2, m)
				if err == nil {
					notify <- sig
					return
				}
			}
		}()
	}
	select {
	case r := <-notify:
		return r, nil
	case <-time.After(15 * time.Minute):
		return nil, errors.New("timeout while signing")
	}
}

func (pk *PrivateKey) deterministicSign(y1, y2 *newhope.Poly, message []byte) (*Signature, error) {
	ctx := GetCtx()
	a := GetA(ctx)
	ctx.NTT(y1, y1)
	ctx.NTT(y2, y2)
	//a * y1 + y2
	t := ctx.NewPoly()
	ctx.MulCoeffs(a, y1, t)
	ctx.Add(t, y2, t)
	ctx.InvNTT(t, t)
	//done making t
	//floored coefficients
	ay1y2 := kfloor(t.Coeffs)
	rounded := ctx.NewPoly()
	//rounded.Coeffs = ay1y2
	copy(rounded.Coeffs, ay1y2)
	//fmt.Println(rounded.Coeffs)
	h := hash(rounded, message, ctx.N())
	c := encodeSparsePolynomial(ctx, omega, h)
	ctx.NTT(c, c)
	//making z1 = s*c + y1
	s := ctx.NewPoly()
	z1 := ctx.NewPoly()
	//s.Copy(pk.GetS())
	copy(s.Coeffs, pk.GetS().Coeffs)
	//fmt.Println(s.Coeffs)
	//ctx.NTT(s, s)
	ctx.MulCoeffs(s, c, z1)
	ctx.Add(z1, y1, z1)
	ctx.InvNTT(z1, z1)
	//done
	Q := constQ
	for _, coeff := range z1.Coeffs {
		if abs(coeff, Q) > (constB - omega) {
			//fmt.Println("J: ", j, "ABS: ", (constB - omega))
			return nil, errors.New("Rejected")
		}
	}

	e := ctx.NewPoly()
	z2 := ctx.NewPoly()
	copy(e.Coeffs, pk.GetE().Coeffs)
	ctx.MulCoeffs(e, c, z2)
	ctx.Add(z2, y2, z2)
	ctx.InvNTT(z2, z2)
	for _, coeff := range z2.Coeffs {
		if abs(coeff, Q) > (constB - omega) {
			return nil, errors.New("Rejected")
		}
	}

	/*z2compressed := make([][]uint64, len(ctx.Modulus))
	subCoeffs := az1tc.GetCoefficients()
	for j, coeffs := range subCoeffs {
		var e error
		Q := ctx.Modulus[j]
		N := ctx.N
		z2compressed[j], e = compress(coeffs, z2.GetCoefficients()[j], N, Q)
		if e != nil {
			return nil, e
		}
	}

	z2.SetCoefficients(z2compressed)*/
	ctx.InvNTT(c, c)
	sig := &Signature{
		z1: z1,
		z2: z2,
		c:  c,
	}
	return sig, nil
}

func encodeSparsePolynomial(ctx *newhope.Context, omega uint32, h [32]byte) *newhope.Poly {
	N := ctx.N()
	usedIndexes := make([]uint32, 0)
	Q := constQ
	sparse := make([]uint32, N)
	stream := hashToRand(0, h) //Create a stream for a specific hash of a message
	r64 := nextRandUint64(stream)
	bitsUsed := uint32(0)
	for i := uint32(0); i < omega && i < N; i++ {
		for {
			if bitsUsed+nBits+1 > 64 {
				r64 = nextRandUint64(stream)
				bitsUsed = 0
			}
			sign := r64 & 1
			r64 >>= 1
			bitsUsed++
			pos := uint32(r64 & (^((^0) << nBits)))
			r64 >>= nBits
			bitsUsed += nBits
			if pos < N {
				success := true
				for j := 0; j < len(usedIndexes); j++ {
					if pos == usedIndexes[j] {
						success = false
					}
				}
				if success {
					usedIndexes = append(usedIndexes, pos)
					if sign == 1 {
						sparse[pos] = 1
					} else {
						sparse[pos] = Q - 1
					}
					break
				}
			}
		}
	}
	p := ctx.NewPoly()
	p.Coeffs = sparse
	return p
}
