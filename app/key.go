package app

import (
	"encoding/base64"
	"encoding/hex"
	"io"

	"errors"

	"strings"

	"github.com/dedis/kyber"
	"github.com/dedis/onet/log"
)

// EncSuite defines the parts of the suite we're needing here.
type EncSuite interface {
	kyber.Group
	kyber.Encoding
}

// Read64Point reads a point from a base64 representation
func Read64Point(suite EncSuite, r io.Reader) (kyber.Point, error) {
	point := suite.Point()
	dec := base64.NewDecoder(base64.StdEncoding, r)
	err := suite.Read(dec, &point)
	return point, err
}

// Read64Pub is deprecated - please use Read64Point
func Read64Pub(suite EncSuite, r io.Reader) (kyber.Point, error) {
	log.Warn("Deprecated - please use Read64Point")
	return Read64Point(suite, r)
}

// Write64Point writes a point to a base64 representation
func Write64Point(suite EncSuite, w io.Writer, point kyber.Point) error {
	enc := base64.NewEncoder(base64.StdEncoding, w)
	return write64(suite, enc, point)
}

// Write64Pub is deprecated - please use Write64Point
func Write64Pub(suite EncSuite, w io.Writer, point kyber.Point) error {
	log.Warn("Deprecated - please use Write64Point")
	return Write64Point(suite, w, point)
}

// Read64Scalar takes a Base64-encoded scalar and returns that scalar,
// optionally an error
func Read64Scalar(suite EncSuite, r io.Reader) (kyber.Scalar, error) {
	s := suite.Scalar()
	dec := base64.NewDecoder(base64.StdEncoding, r)
	err := suite.Read(dec, &s)
	return s, err
}

// Write64Scalar converts a scalar key to a Base64-string
func Write64Scalar(suite EncSuite, w io.Writer, scalar kyber.Scalar) error {
	enc := base64.NewEncoder(base64.StdEncoding, w)
	return write64(suite, enc, scalar)
}

// ReadHexPoint reads a point from a hex representation
func ReadHexPoint(suite EncSuite, r io.Reader) (kyber.Point, error) {
	point := suite.Point()
	buf, err := getHex(r, point.MarshalSize())
	if err != nil {
		return nil, err
	}
	point.UnmarshalBinary(buf)
	return point, err
}

// ReadHexPub is deprecated - please use ReadHexPoint
func ReadHexPub(suite EncSuite, r io.Reader) (kyber.Point, error) {
	log.Warn("Deprecated - please use ReadHexPoint")
	return ReadHexPoint(suite, r)
}

// WriteHexPoint writes a point to a hex representation
func WriteHexPoint(suite EncSuite, w io.Writer, point kyber.Point) error {
	buf, err := point.MarshalBinary()
	if err != nil {
		return err
	}
	out := hex.EncodeToString(buf)
	_, err = w.Write([]byte(out))
	return err
}

// WriteHexPub is deprecated - please use WriteHexPoint
func WriteHexPub(suite EncSuite, w io.Writer, point kyber.Point) error {
	log.Warn("Deprecated - please use WriteHexPoint")
	return WriteHexPoint(suite, w, point)
}

// ReadHexScalar takes a hex-encoded scalar and returns that scalar,
// optionally an error
func ReadHexScalar(suite EncSuite, r io.Reader) (kyber.Scalar, error) {
	s := suite.Scalar()
	buf, err := getHex(r, s.MarshalSize())
	if err != nil {
		return nil, err
	}
	s.UnmarshalBinary(buf)
	return s, nil
}

// WriteHexScalar converts a scalar key to a hex-string
func WriteHexScalar(suite EncSuite, w io.Writer, scalar kyber.Scalar) error {
	buf, err := scalar.MarshalBinary()
	if err != nil {
		return err
	}
	out := hex.EncodeToString(buf)
	_, err = w.Write([]byte(out))
	return err
}

// PointToStringHex converts a point to a hexadecimal representation
func PointToStringHex(suite EncSuite, point kyber.Point) (string, error) {
	pbuf, err := point.MarshalBinary()
	return hex.EncodeToString(pbuf), err
}

// PubToStringHex is deprecated - please use PointToStringHex
func PubToStringHex(suite EncSuite, point kyber.Point) (string, error) {
	log.Warn("Deprecated - please use PointToStringHex")
	return PointToStringHex(suite, point)
}

// StringHexToPoint reads a hexadecimal representation of a point and convert it to the
// right struct
func StringHexToPoint(suite EncSuite, s string) (kyber.Point, error) {
	return ReadHexPoint(suite, strings.NewReader(s))
}

// StringHexToPub is deprecated - please use StringHexToPoint
func StringHexToPub(suite EncSuite, s string) (kyber.Point, error) {
	log.Warn("Deprecated - please use StringHexToPoint")
	return StringHexToPoint(suite, s)
}

// PointToString64 converts a point to a base64 representation
func PointToString64(suite EncSuite, point kyber.Point) (string, error) {
	pbuf, err := point.MarshalBinary()
	return base64.StdEncoding.EncodeToString(pbuf), err
}

// PubToString64 is deprecated - please use PointToStringHex
func PubToString64(suite EncSuite, point kyber.Point) (string, error) {
	log.Warn("Deprecated - please use PointToString64")
	return PointToString64(suite, point)
}

// String64ToPoint reads a base64 representation of a point and converts it
// back to a point.
func String64ToPoint(suite EncSuite, s string) (kyber.Point, error) {
	return Read64Point(suite, strings.NewReader(s))
}

// String64ToPub is deprecated - please use String64ToPoint
func String64ToPub(suite EncSuite, s string) (kyber.Point, error) {
	log.Warn("Deprecated - please use String64ToPoint")
	return String64ToPoint(suite, s)
}

// ScalarToStringHex encodes a scalar to hexadecimal
func ScalarToStringHex(suite EncSuite, scalar kyber.Scalar) (string, error) {
	sbuf, err := scalar.MarshalBinary()
	return hex.EncodeToString(sbuf), err
}

// StringHexToScalar reads a scalar in hexadecimal from string
func StringHexToScalar(suite EncSuite, str string) (kyber.Scalar, error) {
	return ReadHexScalar(suite, strings.NewReader(str))
}

// ScalarToString64 encodes a scalar to a base64
func ScalarToString64(suite EncSuite, scalar kyber.Scalar) (string, error) {
	sbuf, err := scalar.MarshalBinary()
	return base64.StdEncoding.EncodeToString(sbuf), err
}

// String64ToScalar reads a scalar in base64 from a string
func String64ToScalar(suite EncSuite, str string) (kyber.Scalar, error) {
	return Read64Scalar(suite, strings.NewReader(str))
}

func write64(suite EncSuite, wc io.WriteCloser, data ...interface{}) error {
	if err := suite.Write(wc, data); err != nil {
		return err
	}
	return wc.Close()
}

func getHex(r io.Reader, len int) ([]byte, error) {
	bufHex := make([]byte, len*2)
	bufByte := make([]byte, len)
	l, err := r.Read(bufHex)
	if err != nil {
		return nil, err
	}
	if l < len {
		return nil, errors.New("didn't get enough bytes from stream")
	}
	_, err = hex.Decode(bufByte, bufHex)
	if err != nil {
		return nil, err
	}
	return bufByte, nil
}
