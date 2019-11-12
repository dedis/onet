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

type PrivateKey struct {
	s   *ring.Poly
	e   *ring.Poly
	a   *ring.Poly
	ctx *ring.Context
}

type Signature struct {
	z1 *ring.Poly
	z2 *ring.Poly
	c  *ring.Poly
}

type PublicKey struct {
	t   *ring.Poly
	a   *ring.Poly
	ctx *ring.Context
}

func (psk *PublicKey) GetT() *ring.Poly {
	return psk.t
}

func (pk *PrivateKey) GetCtx() *ring.Context {
	return pk.ctx
}

func (pk *PrivateKey) GetPublicParameter() *ring.Poly {
	return pk.a
}

func NewPrivateKey(ctx *ring.Context, a *ring.Poly) (*PrivateKey, error) {
	s, e := getSigningPair(ctx)
	return &PrivateKey{
		s:   s,
		e:   e,
		a:   a,
		ctx: ctx,
	}, nil
}

func (sk *PrivateKey) PK() *PublicKey {
	ctx := sk.GetCtx()
	s1 := sk.s
	s2 := sk.e
	ctx.NTT(s1, s1)
	ctx.NTT(s2, s2)
	a := sk.GetPublicParameter()
	pkPol := ctx.NewPoly()
	ctx.MulCoeffs(a, s1, pkPol)
	ctx.Add(pkPol, s2, pkPol)
	ctx.InvNTT(pkPol, pkPol)
	return &PublicKey{
		t:   pkPol,
		a:   a,
		ctx: ctx,
	}
}

func (pk *PrivateKey) Sign(m []byte) (*Signature, error) {
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

func (pk *PrivateKey) deterministicSign(y1, y2 *ring.Poly, message []byte) (*Signature, error) {
	a := pk.GetPublicParameter()
	ctx := pk.GetCtx()
	y1fft := ctx.NewPoly()
	y2fft := ctx.NewPoly()
	ctx.NTT(y1, y1fft)
	ctx.NTT(y2, y2fft)
	//a * y1 + y2
	t := ctx.NewPoly()
	t.Copy(y2fft)
	mul := ctx.NewPoly()
	ctx.MulCoeffs(a, y1fft, mul)
	ctx.Add(mul, t, t)
	ctx.InvNTT(t, t)
	ay1y2 := make([][]uint64, len(t.GetCoefficients()))
	copy(ay1y2, t.GetCoefficients())
	ay1y2 = kfloor(ay1y2)
	rounded := ctx.NewPoly()
	rounded.SetCoefficients(ay1y2)
	h := hash(rounded, message, ctx.N)
	c := encodeSparsePolynomial(ctx, omega, h)
	z1 := sparseMul(c, pk.s, ctx, omega)
	ctx.Add(z1, y1, z1)
	for i, pol := range z1.GetCoefficients() {
		Q := ctx.Modulus[i]
		for _, coeff := range pol {
			if abs(coeff, Q) > (constB - omega) {
				//fmt.Println("J: ", j, "ABS: ", (constB - omega))
				return nil, errors.New("Rejected")
			}
		}
	}
	z2 := sparseMul(c, pk.e, ctx, omega)
	ctx.Add(z2, y1, z2)
	for i, pol := range z2.GetCoefficients() {
		Q := ctx.Modulus[i]
		for _, coeff := range pol {
			if abs(coeff, Q) > (constB - omega) {
				return nil, errors.New("Rejected")
			}

		}
	}

	az1tc := ctx.NewPoly()
	ctx.Sub(t, z1, az1tc)

	z2compressed := make([][]uint64, len(ctx.Modulus))
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

	z2.SetCoefficients(z2compressed)
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
	if l != len(modulus) {
		panic("lol")
	}
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
