package ciphersuite

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

func TestCipherData_String(t *testing.T) {
	data := &CipherData{
		Name: "A",
		Data: []byte{255},
	}

	require.Equal(t, "41ff", data.String())
}

func TestCipherData_Equal(t *testing.T) {
	a := &CipherData{Name: "abc", Data: []byte{1, 2, 3}}
	b := &CipherData{Name: "abc", Data: []byte{1, 2, 3}}

	require.True(t, a.Equal(b))
	require.True(t, b.Equal(a))

	b.Name = "oops"
	require.False(t, a.Equal(b))

	b.Name = a.Name
	b.Data = []byte{}
	require.False(t, a.Equal(b))
}

type badWriter struct{}

func (w *badWriter) Write(b []byte) (int, error) {
	return 0, xerrors.New("this is a bad writer")
}

func TestCipherData_WriteTo(t *testing.T) {
	data := &CipherData{
		Name: "abc",
		Data: []byte{1, 2, 3},
	}

	buf := new(bytes.Buffer)
	n, err := data.WriteTo(buf)

	require.NoError(t, err)
	require.Equal(t, len(data.Name)+len(data.Data), int(n))
	require.Equal(t, []byte{0x61, 0x62, 0x63, 0x1, 0x2, 0x3}, buf.Bytes())

	n, err = data.WriteTo(new(badWriter))
	require.Error(t, err)
}

func TestCipherData_MarshalText(t *testing.T) {
	data := &CipherData{
		Name: "abc",
		Data: []byte{1, 2, 3},
	}

	buf, err := data.MarshalText()
	require.NoError(t, err)

	data2 := &CipherData{}
	err = data2.UnmarshalText(buf)
	require.NoError(t, err)
	require.Equal(t, data, data2)
}

func TestCipherData_UnmarshalText(t *testing.T) {
	data := &CipherData{Name: "abc", Data: []byte{1, 2, 3}}
	err := data.UnmarshalText([]byte{255})
	require.Error(t, err)
	require.Contains(t, err.Error(), "decoding hex:")

	buf, err := data.MarshalText()
	require.NoError(t, err)

	err = data.UnmarshalText(buf[:2])
	require.Error(t, err)
	require.Contains(t, err.Error(), "data is too small")

	err = data.UnmarshalText(buf[:sizeLength*2+2])
	require.Error(t, err)
	require.Contains(t, err.Error(), "data is too small")
}
