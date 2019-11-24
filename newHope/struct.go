package newHope

const NewHopeName = "New Hope"

const sizeOfCoefficient = 8 //Assuming the coefficients are uint64 so 64 / 8

const NewHopePublicKeySize = 2 + numberOfModulie*numberOfCoefficients*sizeOfCoefficient

const numberOfModulie = 1

const numberOfCoefficients = 1024

const NewHopePrivateKeySize = numberOfModulie * 2 * NewHopePolySize

const NewHopeSignatureSize = numberOfModulie * 3 * NewHopePolySize

const NewHopePolySize = numberOfCoefficients*numberOfModulie*sizeOfCoefficient + 2

type PublicKey []byte

type PrivateKey []byte

type GlyphSuite struct {
	// This struct signs with Glyph
	// using larger ring elements
}

type GlyphSmallSuite struct {
	// This struct signs with Glyph
	// using smaller ring elements
}
