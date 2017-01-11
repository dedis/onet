package network

import (
	"crypto/rand"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var idS MessageID = 52

type TestRegisterS struct {
	I int
}

type TestRegisterR struct {
	I int
}

// returns true if defer happened or false otherwise
func catchDefer(f func()) (b bool) {
	defer func() {
		if e := recover(); e != nil {
			b = true
		}
	}()
	f()
	return
}

func TestRegister(t *testing.T) {
	var ty = reflect.TypeOf(TestRegisterS{})
	if i := registry.msgID(ty); i != ErrorID {
		t.Error("TestRegister should not yet be there")
	}

	RegisterMessage(idS, &TestRegisterS{})
	assert.Equal(t, registry.msgType(idS), ty)
	assert.Equal(t, registry.msgID(ty), idS)

	if tt := registry.msgType(idS); tt != ty {
		t.Error("TestRegister is different now")
	}

	fn := func() { RegisterMessage(idS, &TestRegisterR{}) }
	assert.True(t, catchDefer(fn))
}

func TestRegisterMarshalling(t *testing.T) {
	RegisterMessage(idS, &TestRegisterS{})
	buff, err := Marshal(&TestRegisterS{10})
	require.Nil(t, err)

	ty, b, err := Unmarshal(buff)
	assert.Nil(t, err)
	assert.Equal(t, idS, ty)
	assert.Equal(t, 10, b.(*TestRegisterS).I)

	var randType [2]byte
	rand.Read(randType[:])
	buff = append(randType[:], buff[2:]...)
	ty, b, err = Unmarshal(buff)
	assert.NotNil(t, err)
	assert.Equal(t, ErrorID, ty)
}
