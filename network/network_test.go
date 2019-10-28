package network

import (
	"testing"

	"go.dedis.ch/onet/v4/ciphersuite"
	"go.dedis.ch/onet/v4/log"
)

var testSuite = ciphersuite.NewEd25519CipherSuite()
var testRegistry = ciphersuite.NewRegistry()

func TestMain(m *testing.M) {
	testRegistry.RegisterCipherSuite(testSuite)
	log.MainTest(m)
}
