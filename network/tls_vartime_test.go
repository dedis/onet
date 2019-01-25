// +build vartime

package network

import (
	"testing"

	"go.dedis.ch/kyber/suites"
)

func TestTLS_bn256g1(t *testing.T) {
	testTLS(t, suites.MustFind("bn256.g1"))
}
