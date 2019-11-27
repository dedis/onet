package newHope

import (
	"go.dedis.ch/onet/v3/glyph"
	"go.dedis.ch/onet/v3/glyph_small"
)

const NewHopeName = "New Hope"

const sizeOfCoefficient = 8 //Assuming the coefficients are uint64 so 64 / 8

const NewHopePublicKeySize = 2 + numberOfModulie*numberOfCoefficients*sizeOfCoefficient

const numberOfModulie = 1

const numberOfCoefficients = 1024

const NewHopePrivateKeySize = numberOfModulie * 2 * NewHopePolySize

const NewHopeSignatureSize = numberOfModulie * (2*NewHopePolySize + int(2*omega))

const NewHopePolySize = numberOfCoefficients*numberOfModulie*sizeOfCoefficient + 2

//PublicKey marshalled
type PublicKey []byte

//PrivateKey marshalled
type PrivateKey []byte

//GlyphSuite uses large ring elements
type GlyphSuite struct {
	// This struct signs with Glyph
	// using larger ring elements
}

//GlyphSmallSuite uses small ring elements
type GlyphSmallSuite struct {
	// This struct signs with Glyph
	// using smaller ring elements
}

//NewSignSuite returns the default suite to be used.
func NewSignSuite() NewHope {
	return &GlyphSuite{} //This will be the default suite then.
}

//NewSignSuiteSmall returns a suite that
//utilizes smaller coefficients
func NewSignSuiteSmall() NewHope {
	return &GlyphSmallSuite{}
}

func (g *GlyphSuite) SizeOfPolynomial() int {
	return glyph.PolySize
}

func (g *GlyphSuite) SizeOfSignature() int {
	return int(glyph.SignatureSize)
}

func (g *GlyphSuite) SizeOfPublicKey() int {
	return glyph.PublicKeySize
}
func (g *GlyphSuite) SizeOfPrivateKey() int {
	return glyph.PrivateKeySize
}

func (g *GlyphSmallSuite) SizeOfPolynomial() int {
	return glyph_small.PolySize
}

func (g *GlyphSmallSuite) SizeOfSignature() int {
	return glyph_small.SignatureSize
}

func (g *GlyphSmallSuite) SizeOfPublicKey() int {
	return glyph_small.PublicKeySize
}
func (g *GlyphSmallSuite) SizeOfPrivateKey() int {
	return glyph_small.PrivateKeySize
}
