package onet

import (
	"testing"

	"go.dedis.ch/onet/v3/ciphersuite"
	"go.dedis.ch/onet/v3/log"
)

var testSuite = ciphersuite.NewEd25519CipherSuite()
var testRegistry = ciphersuite.NewRegistry()

// To avoid setting up testing-verbosity in all tests
func TestMain(m *testing.M) {
	testRegistry.RegisterCipherSuite(testSuite)
	log.MainTest(m)
}
