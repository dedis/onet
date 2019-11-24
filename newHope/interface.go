package newHope

import "io"

//NewHope is a signing interface
type NewHope interface {
	Sign(sk PrivateKey, msg []byte) ([]byte, error)
	Verify(pk PublicKey, msg, sig []byte) error
	GenerateKey(rand io.Reader) (PublicKey, PrivateKey, error)
}
