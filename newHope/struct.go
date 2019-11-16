package newHope

const NewHopeName = "New Hope"

const sizeOfCoefficient = 8 //Assuming the coefficients are uint64 so 64 / 8

const NewHopePublicKeySize = numberOfModulie * numberOfCoefficients * sizeOfCoefficient

const numberOfModulie = 1

const numberOfCoefficients = 1024

const NewHopePrivateKeySize = numberOfModulie * 2 * NewHopePolySize

const NewHopeSignatureSize = numberOfModulie * 3 * NewHopePolySize

const NewHopePolySize = numberOfCoefficients * numberOfModulie

type PublicKey []byte

type PrivateKey []byte
