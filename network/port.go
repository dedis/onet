package network

import (
	"flag"
	"sync"
)

const rangePort = 3000

var flagPort int
var latestPort int
var lockPort sync.Mutex

func init() {
	flag.IntVar(&flagPort, "baseport", 2000, "first value when generating network hosts")
	flag.Parse()
	latestPort = flagPort
}

// GetFreePort returns the next available port. When running multiple test at the same time,
// you need to change the base used to generate hosts by using:
// 	go test -args -baseport=[0-9]+
//
// Note that this is not thread-safe and the tests should run using the -p parameter
func GetFreePort() int {
	lockPort.Lock()
	defer lockPort.Unlock()
	defer incrementPort()

	return latestPort
}

func incrementPort() {
	latestPort += 10
	latestPort %= flagPort + rangePort
}
