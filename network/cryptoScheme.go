package network

import (
	"github.com/ldsec/lattigo/ring"
	"go.dedis.ch/kyber/v3"
)

const ellipticScheme = "Elliptic Curve"

//CryptoScheme replaces the old group
//interface. The public key can be
//either a point on an Elliptic-Curve(Ed25519 fx)
//
type CryptoScheme interface {
	SecretKey
	Scheme
}

type SecretKey interface {
	Operations

	//GetKey will return the value of the key
	//independent of the encryption scheme.
	GetSecretKey() interface{}

	GetPublicKey(sk interface{}) PublicKey

	GetPublicParameter() interface{}

	//Same as encrypt
	Decrypt(sk interface{}, ct []byte) ([]byte, error)

	//Sign SK tls corresponds to the secret key used for the
	//TLS key exchange while the sk_server is the secret key
	//belonging to the conode.
	Sign(sk_tls, sk_server interface{}, msg []byte) ([]byte, error)

	RandomElement() interface{}
}

type Scheme interface {
	//GetScheme returns which encryption
	//scheme is being used.
	GetScheme() string
}

type Operations interface {
	//Add takes two elements from the same field
	//(This field can be a polynomial, a point on an elliptic curve etc...)
	//and applies the corresponding addition operation on it.
	Add(e1 interface{}, e2 interface{}) (interface{}, error)

	//Mul takes a field element and a corresponding multiplication factor
	//(This field can be a polynomial, a point on an elliptic curve etc...)
	//and applies the corresponding multiplication method on it for that field.
	Mul(e1 interface{}, e2 interface{}) (interface{}, error)
}

type ECScheme struct {
	scalar kyber.Scalar
	point  kyber.Point
	group  kyber.Group
}

func (ec *ECScheme) GetSecretKey() interface{} {
	return ec.scalar
}

func (ec *ECScheme) GetScheme() string {
	return "Elliptic Curve"
}

func (ec *ECScheme) GetPublicKey(sk interface{}) PublicKey {
	return nil
}

func (ec *ECScheme) Decrypt(sk interface{}, ct []byte) ([]byte, error) {
	return nil, nil
}

func test(sk SecretKey) {
	sk.GetPublicKey(sk)
}

func (ec *ECScheme) GetPublicParameter() interface{} {
	scheme := ec.GetScheme()
	if scheme == "Elliptic Curve" {
		pk := ec.group.Point()
		return pk
	} else {
		return nil
	}
}

func (ec *ECScheme) Sign(sk_tls, sk_server interface{}, msg []byte) ([]byte, error) {
	return nil, nil
}

func (ec *ECScheme) Add(e1 interface{}, e2 interface{}) (interface{}, error) {
	return nil, nil
}

func (ec *ECScheme) Mul(e1 interface{}, e2 interface{}) (interface{}, error) {
	return nil, nil
}

func (ec *ECScheme) RandomElement() interface{} {
	return nil
}

func checkIfBuilds() {
	sk := new(ECScheme)
	test(sk)
}

type PublicKey interface {
	Scheme
	Operations
	SetScalar(s kyber.Scalar)

	SetPoly(a ring.Poly)

	//GetPoint returns the public key corresponding
	//to the sclar for this Diffe-Helmann group.
	//It should throw a panic if the crypto scheme being used
	//is not an elliptic curve.
	GetPoint() kyber.Point

	//GetPoly returns a*s +e for a given polynomial a
	//in a given lattice. It should throw a panic if
	//it is not a lattice based cryptoscheme.
	GetPoly() ring.Poly
	//Encrypt encrypts plaintext using the corresponding publickey
	Encrypt(pt []byte) ([]byte, error)
}
