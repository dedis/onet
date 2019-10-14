package network

import (
	"testing"

	"go.dedis.ch/onet/v4/ciphersuite"
	"go.dedis.ch/onet/v4/log"
)

var unsecureSuite = &ciphersuite.UnsecureCipherSuite{}
var testRegistry = ciphersuite.NewRegistry()

func TestMain(m *testing.M) {
	testRegistry.RegisterCipherSuite(unsecureSuite)
	log.MainTest(m)
}
