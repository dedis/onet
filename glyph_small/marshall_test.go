package glyph_small

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMarshallSparse(t *testing.T) {
	msg := []byte("Bjorn")
	ctx := GetCtx()
	h := hash(ctx.NewPoly(), msg, ctx.N())
	sparse := encodeSparsePolynomial(ctx, omega, h)
	data := marshallSparsePolynomial(sparse.Coeffs, uint16(ctx.N()), uint16(omega))
	sparse2, e := unmarshallSparsePolynomial(data, uint16(ctx.N()), uint16(omega), ctx.Modulus())
	require.Nil(t, e)
	for j, c := range sparse.Coeffs {
		c2 := sparse2[j]
		require.True(t, c == c2)
	}

}
