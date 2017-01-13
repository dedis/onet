package onet

import (
	"testing"

	"gopkg.in/dedis/onet.v1/log"
)

// To avoid setting up testing-verbosity in all tests
func TestMain(m *testing.M) {
	log.MainTest(m)
}
