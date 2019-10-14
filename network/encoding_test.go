package network

import (
	"crypto/rand"
	"go.dedis.ch/kyber/v4/pairing"
	"go.dedis.ch/kyber/v4/pairing/bn256"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/kyber/v4"
	"go.dedis.ch/kyber/v4/suites"
)

type TestRegisterS1 struct {
	I int64
}

type TestRegisterS2 struct {
	I int64
}

type TestRegisterS3 struct {
	P1 kyber.Point
	P2 kyber.Point
	C1 TestContainer1
	C2 TestContainer2
}

type TestContainer1 struct {
	P kyber.Point
	S kyber.Scalar
}

type TestContainer2 struct {
	P kyber.Point
	S kyber.Scalar
}

func TestRegisterMessage(t *testing.T) {
	if !MessageType(&TestRegisterS1{}).Equal(ErrorType) {
		t.Fatal("TestRegister should not yet be there")
	}

	trType := RegisterMessage(&TestRegisterS1{})
	if trType.IsNil() {
		t.Fatal("Couldn't register TestRegister-struct")
	}

	if !MessageType(&TestRegisterS1{}).Equal(trType) {
		t.Fatal("TestRegister is different now")
	}
	if !MessageType(TestRegisterS1{}).Equal(trType) {
		t.Fatal("TestRegister is different now")
	}
}

func TestRegisterMessages(t *testing.T) {
	oldRegistry := registry
	registry = newTypeRegistry()
	types := RegisterMessages(&TestRegisterS1{}, &TestRegisterS2{})
	assert.True(t, MessageType(&TestRegisterS1{}).Equal(types[0]))
	assert.True(t, MessageType(&TestRegisterS2{}).Equal(types[1]))
	registry = oldRegistry
}

func TestUnmarshalRegister(t *testing.T) {
	trType := RegisterMessage(&TestRegisterS1{})
	buff, err := Marshal(&TestRegisterS1{10})
	require.Nil(t, err)

	ty, b, err := Unmarshal(buff, tSuite)
	assert.Nil(t, err)
	assert.Equal(t, trType, ty)
	assert.Equal(t, int64(10), b.(*TestRegisterS1).I)

	var randType [16]byte
	rand.Read(randType[:])
	buff = append(randType[:], buff[16:]...)
	ty, b, err = Unmarshal(buff, tSuite)
	assert.NotNil(t, err)
	assert.Equal(t, ErrorType, ty)
}

func TestMarshalKyberTypes(t *testing.T) {
	RegisterMessages(&TestRegisterS3{})
	testMKT(t, pairing.NewSuiteBn256())
	testMKT(t, bn256.NewSuiteG1())
	testMKT(t, bn256.NewSuiteG2())
	testMKT(t, bn256.NewSuiteGT())
}

func testMKT(t *testing.T, s kyber.Group) {
	ed25519 := suites.MustFind("Ed25519")

	obj := &TestRegisterS3{
		P1: ed25519.Point(),
		P2: s.Point(),
		C1: TestContainer1{P: ed25519.Point(), S: ed25519.Scalar()},
		C2: TestContainer2{P: s.Point(), S: s.Scalar()},
	}

	buff, err := Marshal(obj)
	require.Nil(t, err)

	_, msg, err := Unmarshal(buff, ed25519)
	require.Nil(t, err)
	obj2 := msg.(*TestRegisterS3)
	require.NotNil(t, obj2.C1)
	require.NotNil(t, obj2.C1.P)
	require.NotNil(t, obj2.C2)
	require.NotNil(t, obj2.C2.P)
	require.NotNil(t, obj2.P1)
	require.NotNil(t, obj2.P2)
	require.Equal(t, obj2.P1.String(), obj.P1.String())
	require.Equal(t, obj2.P2.String(), obj.P2.String())
	require.Equal(t, obj2.C2.P.String(), obj.C2.P.String())
}
