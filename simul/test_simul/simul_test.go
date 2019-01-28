package main

import (
	"testing"

	"go.dedis.ch/onet/v3/simul"
)

func TestSimulation(t *testing.T) {
	simul.Start("count.toml")
}
