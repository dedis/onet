// +build vartime

package network

import (
	"testing"

	"gopkg.in/dedis/kyber.v2/suites"
)

func TestTLS_bn256g1(t *testing.T) {
	testTLS(t, suites.MustFind("bn256.g1"))
}
