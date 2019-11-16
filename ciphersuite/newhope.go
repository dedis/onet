package ciphersuite

import (
	"encoding/hex"
	"io"

	"go.dedis.ch/onet/v3/newHope"
	"golang.org/x/xerrors"
)

const errNotNewHopeCipherSuite = "invalid cipher suite"

type NewHopePublicKey struct {
	data newHope.PublicKey
}

func (pk *NewHopePublicKey) Name() string {
	return newHope.NewHopeName
}

func (pk *NewHopePublicKey) String() string {
	return hex.EncodeToString(pk.data)
}

func (pk *NewHopePublicKey) Raw() *RawPublicKey {
	return NewRawPublicKey(pk.Name(), pk.data)
}

func (pk *NewHopePublicKey) Equal(other PublicKey) bool {
	return pk.Raw().Equal(other.Raw())
}

type NewHopePrivateKey struct {
	data newHope.PrivateKey
}

func (pk *NewHopePrivateKey) Name() string {
	return newHope.NewHopeName
}

func (pk *NewHopePrivateKey) String() string {
	return hex.EncodeToString(pk.data)
}

func (pk *NewHopePrivateKey) Raw() *RawSecretKey {
	return NewRawSecretKey(pk.Name(), pk.data)
}

type NewHopeSignature struct {
	data []byte
}

func (pk *NewHopeSignature) Name() string {
	return newHope.NewHopeName
}

func (pk *NewHopeSignature) String() string {
	return hex.EncodeToString(pk.data)
}

func (pk *NewHopeSignature) Raw() *RawSignature {
	return NewRawSignature(pk.Name(), pk.data)
}

type NewHopeCipherSuite struct {
	//Nothing
}

func (s *NewHopeCipherSuite) Name() string {
	return newHope.NewHopeName
}

func (s *NewHopeCipherSuite) PublicKey(raw *RawPublicKey) (PublicKey, error) {
	if raw.Name() != s.Name() {
		return nil, xerrors.New(errNotNewHopeCipherSuite)
	}

	if len(raw.Data) != newHope.NewHopePublicKeySize {
		return nil, xerrors.New(errInvalidBufferSize)
	}

	return &NewHopePublicKey{data: raw.Data}, nil
}

func (s *NewHopeCipherSuite) PrivateKey(raw *RawSecretKey) (SecretKey, error) {
	if raw.Name() != s.Name() {
		return nil, xerrors.New(errNotNewHopeCipherSuite)
	}

	if len(raw.Data) != newHope.NewHopePrivateKeySize {
		return nil, xerrors.New(errInvalidBufferSize)
	}

	return &NewHopePrivateKey{data: raw.Data}, nil
}

func (s *NewHopeCipherSuite) Signature(raw *RawSignature) (Signature, error) {
	if raw.Name() != s.Name() {
		return nil, xerrors.New(errNotNewHopeCipherSuite)
	}

	if len(raw.Data) != newHope.NewHopePrivateKeySize {
		return nil, xerrors.New(errInvalidBufferSize)
	}

	return &NewHopeSignature{data: raw.Data}, nil
}

func (s *NewHopeCipherSuite) GenerateKeyPair(reader io.Reader) (PublicKey, SecretKey, error) {
	pk, sk, err := newHope.GenerateKey(reader)
	if err != nil {
		return nil, nil, err
	}
	return &NewHopePublicKey{data: pk}, &NewHopePrivateKey{data: sk}, nil
}
