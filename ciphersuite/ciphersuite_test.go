package ciphersuite

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

func TestCipherData_String(t *testing.T) {
	data := &CipherData{
		CipherName: "A",
		Data:       []byte{255},
	}

	require.Equal(t, "41ff", data.String())
}

func TestCipherData_Equal(t *testing.T) {
	a := &CipherData{CipherName: "abc", Data: []byte{1, 2, 3}}
	b := &CipherData{CipherName: "abc", Data: []byte{1, 2, 3}}

	require.True(t, a.Equal(b))
	require.True(t, b.Equal(a))

	b.CipherName = "oops"
	require.False(t, a.Equal(b))

	b.CipherName = a.CipherName
	b.Data = []byte{}
	require.False(t, a.Equal(b))
}

func TestCipherData_Clone(t *testing.T) {
	a := &CipherData{CipherName: "abc", Data: []byte{1, 2, 3}}
	b := a.Clone()

	require.True(t, a.Equal(b))

	b.Data[0] = 4
	require.False(t, a.Equal(b))
}

type badWriter struct{}

func (w *badWriter) Write(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, xerrors.New("this is a bad writer")
	}
	return 0, nil
}

func TestCipherData_WriteTo(t *testing.T) {
	data := &CipherData{
		CipherName: "abc",
		Data:       []byte{1, 2, 3},
	}

	buf := new(bytes.Buffer)
	n, err := data.WriteTo(buf)

	require.NoError(t, err)
	require.Equal(t, len(data.CipherName)+len(data.Data), int(n))
	require.Equal(t, []byte{0x61, 0x62, 0x63, 0x1, 0x2, 0x3}, buf.Bytes())

	data.Data = []byte{}
	n, err = data.WriteTo(new(badWriter))
	require.Error(t, err)

	data.CipherName = ""
	n, err = data.WriteTo(new(badWriter))
	require.Error(t, err)
}

func TestCipherData_MarshalText(t *testing.T) {
	data := &CipherData{
		CipherName: "abc",
		Data:       []byte{1, 2, 3},
	}

	buf, err := data.MarshalText()
	require.NoError(t, err)

	data2 := &CipherData{}
	err = data2.UnmarshalText(buf)
	require.NoError(t, err)
	require.Equal(t, data, data2)
}

func TestCipherData_UnmarshalText(t *testing.T) {
	data := &CipherData{CipherName: "abc", Data: []byte{1, 2, 3}}
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

func TestRawPublicKey_Equal(t *testing.T) {
	raw1 := NewRawPublicKey("abc", []byte{1})
	raw2 := NewRawPublicKey("abc", []byte{2})

	require.False(t, raw1.Equal(raw2))
	require.True(t, raw1.Equal(raw1))
	require.True(t, raw1.Raw().Equal(raw1))
	require.True(t, raw2.Clone().Equal(raw2))
}

func TestRawPublicKey_TextMarshaling(t *testing.T) {
	raw := NewRawPublicKey("abc", []byte{1})

	buf, err := raw.MarshalText()
	require.NoError(t, err)

	decoded := NewRawPublicKey("", []byte{})
	require.NoError(t, decoded.UnmarshalText(buf))
	require.True(t, raw.Equal(decoded))

	require.Error(t, decoded.UnmarshalText([]byte{}))
}

func TestRawSecretKey_Clone(t *testing.T) {
	raw := NewRawSecretKey("abc", []byte{1})
	raw2 := raw.Clone()

	require.Equal(t, raw, raw2)

	raw2.Data[0] = 5
	require.NotEqual(t, raw, raw2)
}

func TestRawSecretKey_TextMarshaling(t *testing.T) {
	raw := NewRawSecretKey("abc", []byte{1})

	buf, err := raw.Raw().MarshalText()
	require.NoError(t, err)

	decoded := NewRawSecretKey("", []byte{})
	require.NoError(t, decoded.UnmarshalText(buf))
	require.Equal(t, raw, decoded)

	require.Error(t, decoded.UnmarshalText([]byte{}))
}

func TestRawSignature_Clone(t *testing.T) {
	raw := NewRawSignature("abc", []byte{1})
	raw2 := raw.Clone()

	require.Equal(t, raw, raw2)

	raw2.Data[0] = 5
	require.NotEqual(t, raw, raw2)
}

func TestRawSignature_TextMarshaling(t *testing.T) {
	raw := NewRawSignature("abc", []byte{1})

	buf, err := raw.Raw().MarshalText()
	require.NoError(t, err)

	decoded := NewRawSignature("", []byte{})
	require.NoError(t, decoded.UnmarshalText(buf))
	require.Equal(t, raw, decoded)

	require.Error(t, decoded.UnmarshalText([]byte{}))
}
