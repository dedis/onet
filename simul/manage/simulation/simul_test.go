package main

import (
	"testing"

	"io/ioutil"

	"strings"

	"github.com/stretchr/testify/assert"
	"go.dedis.ch/onet/v3/log"
	"go.dedis.ch/onet/v3/simul"
)

func TestSimulation(t *testing.T) {
	simul.Start(builder, "count.toml", "csv1.toml", "csv2.toml")
}

func TestSimulation_IndividualStats(t *testing.T) {
	simul.Start(builder, "individualstats.toml")
	csv, err := ioutil.ReadFile("test_data/individualstats.csv")
	log.ErrFatal(err)
	// header + 5 rounds + final newline
	assert.Equal(t, 7, len(strings.Split(string(csv), "\n")))

	simul.Start(builder, "csv1.toml")
	csv, err = ioutil.ReadFile("test_data/csv1.csv")
	log.ErrFatal(err)
	// header + 2 experiments + final newline
	assert.Equal(t, 4, len(strings.Split(string(csv), "\n")))
}
