package network

import (
	"testing"

	"github.com/dedis/onet/log"
	_ "gopkg.in/dedis/kyber.v1/group/edwards25519"
	"gopkg.in/dedis/kyber.v1/suites"
)

var tSuite = suites.MustFind("Ed25519")

func TestMain(m *testing.M) {
	log.MainTest(m)
}
