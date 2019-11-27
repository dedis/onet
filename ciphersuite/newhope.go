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
	suite newHope.NewHope
}

func (s *NewHopeCipherSuite) Name() string {
	return newHope.NewHopeName
}

func (s *NewHopeCipherSuite) PublicKey(raw *RawPublicKey) (PublicKey, error) {
	suite := s.suite
	if raw.Name() != s.Name() {
		return nil, xerrors.New(errNotNewHopeCipherSuite)
	}

	if len(raw.Data) != suite.SizeOfPublicKey() {
		return nil, xerrors.New(errInvalidBufferSize)
	}

	return &NewHopePublicKey{data: raw.Data}, nil
}

func (s *NewHopeCipherSuite) PrivateKey(raw *RawSecretKey) (SecretKey, error) {
	suite := s.suite
	if raw.Name() != s.Name() {
		return nil, xerrors.New(errNotNewHopeCipherSuite)
	}

	if len(raw.Data) != suite.SizeOfPrivateKey() {
		return nil, xerrors.New(errInvalidBufferSize)
	}

	return &NewHopePrivateKey{data: raw.Data}, nil
}

func (s *NewHopeCipherSuite) Signature(raw *RawSignature) (Signature, error) {
	suite := s.suite
	if raw.Name() != s.Name() {
		return nil, xerrors.New(errNotNewHopeCipherSuite)
	}

	if len(raw.Data) != suite.SizeOfSignature() {
		return nil, xerrors.New(errInvalidBufferSize)
	}

	return &NewHopeSignature{data: raw.Data}, nil
}

func (s *NewHopeCipherSuite) GenerateKeyPair(reader io.Reader) (PublicKey, SecretKey, error) {
	suite := s.suite
	pk, sk, err := suite.GenerateKey(reader)
	if err != nil {
		return nil, nil, err
	}
	return &NewHopePublicKey{data: pk}, &NewHopePrivateKey{data: sk}, nil
}

func (s *NewHopeCipherSuite) SecretKey(raw *RawSecretKey) (SecretKey, error) {
	suite := s.suite
	if raw.Name() != s.Name() {
		return nil, xerrors.New(errNotNewHopeCipherSuite)
	}

	if len(raw.Data) != suite.SizeOfPrivateKey() {
		return nil, xerrors.New(errInvalidBufferSize)
	}

	return &NewHopePrivateKey{data: raw.Data}, nil
}

func (s *NewHopeCipherSuite) unpackSecretKey(sk SecretKey) (*NewHopePrivateKey, error) {
	if data, ok := sk.(*RawSecretKey); ok {
		var err error
		sk, err = s.SecretKey(data)
		if err != nil {
			return nil, err
		}
	}

	if secretKey, ok := sk.(*NewHopePrivateKey); ok {
		return secretKey, nil
	}

	return nil, xerrors.New("wrong type of secret key")
}

func (s *NewHopeCipherSuite) Sign(sk SecretKey, msg []byte) (Signature, error) {
	suite := s.suite
	secretKey, err := s.unpackSecretKey(sk)
	if err != nil {
		return nil, err
	}
	sigbuf, e := suite.Sign(secretKey.data, msg)
	if e != nil {
		return nil, e
	}
	return &NewHopeSignature{data: sigbuf}, nil
}

// Verify returns nil when the signature of the message can be verified by
// the public key.
func (s *NewHopeCipherSuite) Verify(pk PublicKey, sig Signature, msg []byte) error {
	suite := s.suite
	publicKey, err := s.unpackPublicKey(pk)
	if err != nil {
		return xerrors.Errorf("unpacking public key: %v", err)
	}

	signature, err := s.unpackSignature(sig)
	if err != nil {
		return xerrors.Errorf("unpacking signature: %v", err)
	}

	e := suite.Verify(publicKey.data, msg, signature.data)
	if e != nil {
		return e
	}

	return nil
}

func (s *NewHopeCipherSuite) unpackPublicKey(pk PublicKey) (*NewHopePublicKey, error) {
	if data, ok := pk.(*RawPublicKey); ok {
		var err error
		pk, err = s.PublicKey(data)
		if err != nil {
			return nil, err
		}
	}

	if publicKey, ok := pk.(*NewHopePublicKey); ok {
		return publicKey, nil
	}

	return nil, xerrors.New("wrong type of public key")
}

func (s *NewHopeCipherSuite) unpackSignature(sig Signature) (*NewHopeSignature, error) {
	if data, ok := sig.(*RawSignature); ok {
		var err error
		sig, err = s.Signature(data)
		if err != nil {
			return nil, err
		}
	}

	if signature, ok := sig.(*NewHopeSignature); ok {
		return signature, nil
	}

	return nil, xerrors.New("wrong type of signature")
}

//NewNewHopeCipherSuite Returns the default cipher suite
func NewNewHopeCipherSuite() *NewHopeCipherSuite {
	return &NewHopeCipherSuite{
		suite: newHope.NewSignSuite(),
	}
}

//NewNewHopeCipherSuiteSmall returns a newhpe cipher suite that
//utilizes small coefficients
func NewNewHopeCipherSuiteSmall() *NewHopeCipherSuite {
	return &NewHopeCipherSuite{
		suite: newHope.NewSignSuiteSmall(),
	}
}

func (s *NewHopeCipherSuite) getSuite() newHope.NewHope {
	return nil
}

type NewHopeSmaller struct {
}

func (nh *NewHopeSmaller) Sign(sk SecretKey, msg []byte) (Signature, error) {
	suite := NewNewHopeCipherSuiteSmall()
	return suite.Sign(sk, msg)
}

func (nh *NewHopeSmaller) Name() string {
	return NewNewHopeCipherSuiteSmall().Name()
}

func (nh *NewHopeSmaller) Verify(pk PublicKey, signature Signature, msg []byte) error {
	return NewNewHopeCipherSuiteSmall().Verify(pk, signature, msg)
}

func (nh *NewHopeSmaller) Signature(raw *RawSignature) (Signature, error) {
	return NewNewHopeCipherSuiteSmall().Signature(raw)
}

func (nh *NewHopeSmaller) GenerateKeyPair(reader io.Reader) (PublicKey, SecretKey, error) {
	return NewNewHopeCipherSuiteSmall().GenerateKeyPair(reader)
}

func (nh *NewHopeSmaller) SecretKey(raw *RawSecretKey) (SecretKey, error) {
	return NewNewHopeCipherSuiteSmall().SecretKey(raw)
}

func (nh *NewHopeSmaller) PublicKey(raw *RawPublicKey) (PublicKey, error) {
	return NewNewHopeCipherSuiteSmall().PublicKey(raw)
}

type NewHopeRing struct {
}

func (nh *NewHopeRing) Sign(sk SecretKey, msg []byte) (Signature, error) {
	suite := NewNewHopeCipherSuite()
	return suite.Sign(sk, msg)
}

func (nh *NewHopeRing) Name() string {
	return NewNewHopeCipherSuite().Name()
}

func (nh *NewHopeRing) Verify(pk PublicKey, signature Signature, msg []byte) error {
	return NewNewHopeCipherSuite().Verify(pk, signature, msg)
}

func (nh *NewHopeRing) Signature(raw *RawSignature) (Signature, error) {
	return NewNewHopeCipherSuite().Signature(raw)
}

func (nh *NewHopeRing) GenerateKeyPair(reader io.Reader) (PublicKey, SecretKey, error) {
	return NewNewHopeCipherSuite().GenerateKeyPair(reader)
}

func (nh *NewHopeRing) SecretKey(raw *RawSecretKey) (SecretKey, error) {
	return NewNewHopeCipherSuite().SecretKey(raw)
}

func (nh *NewHopeRing) PublicKey(raw *RawPublicKey) (PublicKey, error) {
	return NewNewHopeCipherSuite().PublicKey(raw)
}

func NewHope() *NewHopeRing {
	return &NewHopeRing{}
}

func NewHopeSmall() *NewHopeSmaller {
	return &NewHopeSmaller{}
}
