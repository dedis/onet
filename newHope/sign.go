package newHope

import (
	"go.dedis.ch/onet/v3/glyph"
)

func Sign(sk PrivateKey, msg []byte) ([]byte, error) {
	ctx := glyph.GetCtx()
	secret, e := checkPrivateKey(sk, ctx)
	if e != nil {
		return nil, e
	}
	sig, e := secret.Sign(msg)
	if e != nil {
		return nil, e
	}
	return sig.Marshall()
}
