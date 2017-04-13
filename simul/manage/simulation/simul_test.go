package main

import (
	"testing"

	"io/ioutil"

	"strings"

	"github.com/dedis/onet/simul"
	"github.com/stretchr/testify/assert"
	"gopkg.in/dedis/onet.v1/log"
)

func TestSimulation(t *testing.T) {
	simul.Start("count.toml", "csv1.toml", "csv2.toml")
}

func TestSimulation_EveryRoundStats(t *testing.T) {
	simul.Start("everyroundstats.toml")
	csv, err := ioutil.ReadFile("test_data/everyroundstats.csv")
	log.ErrFatal(err)
	// header + 3 rounds + final newline
	assert.Equal(t, 5, len(strings.Split(string(csv), "\n")))

	simul.Start("csv1.toml")
	csv, err = ioutil.ReadFile("test_data/csv1.csv")
	log.ErrFatal(err)
	// header + 2 experiments + final newline
	assert.Equal(t, 4, len(strings.Split(string(csv), "\n")))
}
