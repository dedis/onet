package network

import (
	"testing"

	"github.com/dedis/onet/log"
	_ "go.dedis.ch/kyber/group/edwards25519"
	"go.dedis.ch/kyber/suites"
)

var tSuite = suites.MustFind("Ed25519")

func TestMain(m *testing.M) {
	log.MainTest(m)
}
