// +build vartime

package main

import (
	"github.com/dedis/cothority"
	"gopkg.in/dedis/kyber.v2/suites"
)

func init() {
	cothority.Suite = suites.MustFind("bn256.g1")
}
