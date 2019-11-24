package newHope

type NewHope interface {
	Sign(sk PrivateKey, msg []byte) ([]byte, error)
	Verify(pk PublicKey, msg, sig []byte) error
}
