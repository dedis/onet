package main

import (
	"testing"

	"go.dedis.ch/onet/v4/simul"
)

func TestSimulation(t *testing.T) {
	simul.Start("count.toml")
}
