package network

import (
	"crypto/rand"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	var idS MessageID
	var ty = reflect.TypeOf(TestRegisterS{})
	if i := registry.msgID(ty); i != ErrorID {
		t.Error("TestRegister should not yet be there")
	}

	idS = RegisterMessage("testRegister", &TestRegisterS{})
	assert.Equal(t, registry.msgType(idS), ty)
	assert.Equal(t, registry.msgID(ty), idS)

	fn := func() { RegisterMessage("testRegister", &TestRegisterR{}) }
	assert.True(t, catchDefer(fn))
}

func TestRegisterMarshalling(t *testing.T) {
	var idS MessageID
	idS = RegisterMessage("testRegister", &TestRegisterS{})
	buff, err := Marshal(&TestRegisterS{10})
	require.Nil(t, err)

	id, b, err := Unmarshal(buff)
	assert.Nil(t, err)
	assert.Equal(t, idS, id)
	assert.Equal(t, 10, b.(*TestRegisterS).I)

	var randType [4]byte
	rand.Read(randType[:])
	buff = append(randType[:], buff[4:]...)
	id, b, err = Unmarshal(buff)
	assert.NotNil(t, err)
	assert.Equal(t, ErrorID, id)
}
