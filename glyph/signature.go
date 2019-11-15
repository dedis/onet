package glyph

import (
	"context"
	"errors"
	"time"

	"github.com/AidosKuneen/numcpu"
	"github.com/ldsec/lattigo/ring"
)

/*
*Glyph signature algorithm
*
 */

func (k *PrivateKey) Sign(m []byte) (*Signature, error) {
	pk := k.pols
	notify := make(chan *Signature, numcpu.NumCPU())
	ringCtx := pk.GetCtx()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for i := 0; i < numcpu.NumCPU(); i++ {
		go func() {
			modulus := ringCtx.Modulus
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				y1, y2 := ringCtx.NewUniformPoly(), ringCtx.NewUniformPoly()
				y1Temp := make([][]uint64, len(y1.GetCoefficients()))
				for i, pol := range y1.GetCoefficients() {
					Q := modulus[i]
					l := len(pol)
					temp := make([]uint64, l)
					for j := 0; j < l; j++ {
						v := pol[j]
						for {
							v &= ^(^0 << (bBits + 1))
							if v <= 2*constB+1 {
								break
							}
						}
						if v > constB {
							v = Q - (pol[j] - constB)
						}
						temp[j] = v
					}
					y1Temp[i] = temp
				}
				y1.SetCoefficients(y1Temp)
				y2Temp := make([][]uint64, len(y2.GetCoefficients()))
				for i, pol := range y2.GetCoefficients() {
					Q := modulus[i]
					l := len(pol)
					temp := make([]uint64, l)
					for j := 0; j < l; j++ {
						v := pol[j]
						for {
							v &= ^(^0 << (bBits + 1))
							if v <= 2*constB+1 {
								break
							}
						}
						if v > constB {
							v = Q - (pol[j] - constB)
						}
						temp[j] = v
					}
					y2Temp[i] = temp
				}
				y2.SetCoefficients(y2Temp)
				sig, err := k.deterministicSign(y1, y2, m)
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

func (key *PrivateKey) deterministicSign(y1, y2 *ring.Poly, message []byte) (*Signature, error) {
	pk := key.pols
	a := pk.GetA()
	ctx := pk.GetCtx()
	y1fft := ctx.NewPoly()
	y2fft := ctx.NewPoly()
	ctx.NTT(y1, y1fft)
	ctx.NTT(y2, y2fft)
	//a * y1 + y2
	t := ctx.NewPoly()
	mul := ctx.NewPoly()
	ctx.MulCoeffs(a, y1fft, mul)
	ctx.Add(mul, y2fft, t)
	ctx.InvNTT(t, t)
	//done making t
	//floored coefficients
	ay1y2 := kfloor(t.GetCoefficients())
	rounded := ctx.NewPoly()
	rounded.SetCoefficients(ay1y2)
	h := hash(rounded, message, ctx.N)
	c := encodeSparsePolynomial(ctx, omega, h)
	ctx.NTT(c, c)
	//making z1 = s*c + y1
	sc := ctx.NewPoly()
	s := ctx.NewPoly()
	z1 := ctx.NewPoly()
	s.Copy(pk.GetS())
	ctx.NTT(s, s)
	ctx.MulCoeffs(s, c, sc)
	ctx.Add(sc, y1fft, z1)
	ctx.InvNTT(z1, z1)
	//done
	for i, pol := range z1.GetCoefficients() {
		Q := ctx.Modulus[i]
		for _, coeff := range pol {
			if abs(coeff, Q) > (constB - omega) {
				//fmt.Println("J: ", j, "ABS: ", (constB - omega))
				return nil, errors.New("Rejected")
			}
		}
	}
	//making z2 = e*c + y2
	ec := ctx.NewPoly()
	e := ctx.NewPoly()
	z2 := ctx.NewPoly()
	e.Copy(pk.GetE())
	ctx.NTT(e, e)
	ctx.MulCoeffs(e, c, ec)
	ctx.Add(ec, y2fft, z2)
	ctx.InvNTT(z2, z2)
	for i, pol := range z2.GetCoefficients() {
		Q := ctx.Modulus[i]
		for _, coeff := range pol {
			if abs(coeff, Q) > (constB - omega) {
				return nil, errors.New("Rejected")
			}

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
	//fmt.Println("C: ", c)
	sig := &Signature{
		z1: z1,
		z2: z2,
		c:  c,
	}
	return sig, nil
}

func encodeSparsePolynomial(ctx *ring.Context, omega uint64, h [][32]byte) *ring.Poly {
	l := len(h)
	N := ctx.N
	modulus := ctx.Modulus
	newCoeffs := make([][]uint64, len(modulus))
	usedIndexes := make([]uint64, 0)
	for k := uint64(0); k < uint64(l); k++ {
		Q := modulus[k]
		m := h[k]
		sparse := make([]uint64, N)
		stream := hashToRand(k, m) //Create a stream for a specific hash of a message
		r64 := nextRandUint64(stream)
		bitsUsed := uint64(0)
		for i := uint64(0); i < omega && i < N; i++ {
			for {
				if bitsUsed+nBits+1 > 64 {
					r64 = nextRandUint64(stream)
					bitsUsed = 0
				}
				sign := r64 & 1
				r64 >>= 1
				bitsUsed++
				pos := uint64(r64 & (^((^0) << nBits)))
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
		newCoeffs[k] = sparse
	}
	p := ctx.NewPoly()
	e := p.SetCoefficients(newCoeffs)
	if e != nil {
		panic(e)
	}
	return p
}
