package glyph

import (
	"encoding/binary"

	"github.com/ldsec/lattigo/ring"
	"golang.org/x/xerrors"
)

func (pk *PublicKey) Marshall() ([]byte, error) {
	t := pk.GetT()
	return t.MarshalBinary()
}

func (pk *PrivateKey) Marshall() ([]byte, error) {
	z1 := pk.GetS()
	z2 := pk.GetE()
	d1, e1 := z1.MarshalBinary()
	if e1 != nil {
		return nil, e1
	}
	d2, e2 := z2.MarshalBinary()
	if e2 != nil {
		return nil, e2
	}
	l1, l2 := len(d1), len(d2)
	data := make([]byte, l1)
	data2 := make([]byte, l2)
	copy(data, d1)
	copy(data2, d2)
	comb := make([]byte, l1+l2)
	comb = append(data, data2...)
	return comb, nil
}

func (sig *Signature) Marshall() ([]byte, error) {
	z1 := sig.z1
	z2 := sig.z2
	c := sig.c
	d1, e1 := z1.MarshalBinary()
	if e1 != nil {
		return nil, e1
	}
	d2, e2 := z2.MarshalBinary()
	if e2 != nil {
		return nil, e2
	}
	ctx := GetCtx()
	d3 := make([]byte, uint64(len(ctx.Modulus))*2*omega)
	for j, coeffs := range c.GetCoefficients() {
		i := uint64(j)
		copy(d3[i:i+2*omega], marshallSparsePolynomial(coeffs, uint16(ctx.N), uint16(omega)))
	}
	l1, l2, l3 := len(d1), len(d2), int(2*omega)
	data := make([]byte, l1+l2+l3)
	copy(data[0:l1], d1)
	copy(data[l1:l1+l2], d2)
	copy(data[l1+l2:l1+l2+l3], d3)
	return data, nil
}

func UnmarshallSignature(data []byte) (*ring.Poly, error) {
	ctx := GetCtx()
	nbModules := uint64(len(ctx.Modulus))
	w := omega
	l := uint64(len(data))
	if l%(2*w) != 0 || l/nbModules != (2*w) {
		return nil, xerrors.New("data of invalid size")
	}
	coeffs := make([][]uint64, nbModules)
	N := uint16(ctx.N)
	for i := uint64(0); i < nbModules; i++ {
		p, e := unmarshallSparsePolynomial(data[i:i+2*w], N, uint16(w), ctx.Modulus[i])
		if e != nil {
			return nil, e
		}
		coeffs[i] = p
	}
	c := ctx.NewPoly()
	c.SetCoefficients(coeffs)
	return c, nil
}

func unmarshallSparsePolynomial(data []byte, N, w uint16, Q uint64) ([]uint64, error) {
	l := uint16(len(data))
	if l != 2*w {
		return nil, xerrors.New("Incorrect size of marshalled signature")
	}
	var exists struct{}
	used := make(map[uint16]struct{})
	pol := make([]uint64, N)
	for i := uint16(0); i < l; i += 2 {
		c := binary.LittleEndian.Uint16(data[i : i+2])
		pos := c % N
		if _, ok := used[pos]; ok {
			return nil, xerrors.New("Same position of coefficient used twice")
		}
		if c >= N && c < 2*N {
			pol[pos] = Q - 1
		} else if c >= 0 && c < N {
			pol[pos] = uint64(1)
		} else {
			return nil, xerrors.New("Invalid byte sequence: Invalid index")
		}
		used[pos] = exists
	}

	return pol, nil
}

func checkIfSparse(data []uint32, N, w uint16, Q uint32) error {
	if len(data) != int(N) {
		return xerrors.New("Not enough coefficients")
	}
	count := uint16(0)
	for _, c := range data {
		if c == 0 {
			continue
		}
		if c == Q-1 {
			count++
		} else if c == 1 {
			count++
		} else {
			return xerrors.New("Polynomial has to be sparse: Invalid value of coefficient")
		}
	}
	if count > w {
		return xerrors.New("Too many non-zero coefficients")
	}
	return nil
}

func marshallSparsePolynomial(coeffs []uint64, N, w uint16) []byte {
	data := make([]byte, 2*w)
	index := uint32(0)
	for i := uint16(0); i < N; i++ {
		coeff := coeffs[i]
		if coeff == 0 {
			continue
		}
		if coeff == 1 {
			binary.LittleEndian.PutUint16(data[index:index+2], i)
		} else {
			binary.LittleEndian.PutUint16(data[index:index+2], i+N)
		}
		index += 2
		if index == 2*uint32(w) {
			break
		}
	}
	return data
}
