package network

import (
	"testing"

	_ "go.dedis.ch/kyber/v4/group/edwards25519"
	"go.dedis.ch/kyber/v4/suites"
	"go.dedis.ch/onet/v4/log"
)

var tSuite = suites.MustFind("Ed25519")

func TestMain(m *testing.M) {
	log.MainTest(m)
}
