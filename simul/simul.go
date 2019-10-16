/*
Package simul allows for easy simulation on different platforms. THe following platforms
are available:

	- localhost - for up to 100 nodes
	- mininet - for up to 1'000 nodes
	- deterlab - for up to 50'000 nodes

Usually you start small, then work your way up to the full potential of your
protocol!
*/
package simul

import (
	"flag"
	"os"

	"go.dedis.ch/onet/v4/ciphersuite"
	"go.dedis.ch/onet/v4/log"
	"go.dedis.ch/onet/v4/simul/platform"
)

// The address of this server - if there is only one server in the config
// file, it will be derived from it automatically
var serverAddress string

// ip addr of the logger to connect to
var monitorAddress string

// Simul is != "" if this node needs to start a simulation of that protocol
var simul string

// suite is Ed25519 by default
var suite string

// Initialize before 'init' so we can directly use the fields as parameters
// to 'Flag'
func init() {
	flag.StringVar(&serverAddress, "address", "", "our address to use")
	flag.StringVar(&simul, "simul", "", "start simulating that protocol")
	flag.StringVar(&monitorAddress, "monitor", "", "remote monitor")
	flag.StringVar(&suite, "suite", "Ed25519", "cryptographic suite to use")

}

// Start has to be called by the main-file that imports the protocol and/or the
// service. If a user calls the simulation-file, `simul` is empty, and the
// build is started.
// Only the platform will call this binary with a simul-flag set to the name of the
// simulation to run.
// If given an array of rcs, each element will be interpreted as a .toml-file
// to load and simulate.
func Start(suite ciphersuite.CipherSuite, rcs ...string) {
	wd, err := os.Getwd()
	if len(rcs) > 0 {
		log.ErrFatal(err)
		for _, rc := range rcs {
			log.Lvl1("Running toml-file:", rc)
			os.Args = []string{os.Args[0], rc}
			Start(suite)
		}
		return
	}
	flag.Parse()
	if simul == "" {
		startBuild()
	} else {
		err := platform.Simulate(suite, serverAddress, simul, monitorAddress)
		log.ErrFatal(err)
	}
	os.Chdir(wd)
}
