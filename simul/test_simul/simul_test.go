package main

import (
	"testing"

	"go.dedis.ch/onet/v4/ciphersuite"
	"go.dedis.ch/onet/v4/simul"
)

var testSuite = &ciphersuite.UnsecureCipherSuite{}

func TestSimulation(t *testing.T) {
	simul.Start(testSuite, "count.toml")
}
