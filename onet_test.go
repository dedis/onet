package onet

import (
	"testing"

	"go.dedis.ch/onet/v3/log"
)

// To avoid setting up testing-verbosity in all tests
func TestMain(m *testing.M) {
	log.MainTest(m)
}
