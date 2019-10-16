package onet

import (
	"testing"

	"go.dedis.ch/onet/v4/ciphersuite"
	"go.dedis.ch/onet/v4/log"
)

var testSuite = &ciphersuite.UnsecureCipherSuite{}
var testRegistry = ciphersuite.NewRegistry()

// To avoid setting up testing-verbosity in all tests
func TestMain(m *testing.M) {
	testRegistry.RegisterCipherSuite(testSuite)
	log.MainTest(m)
}
