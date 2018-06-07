// +build vartime

package main

import (
	"github.com/dedis/cothority"
	"github.com/dedis/kyber/suites"
)

func init() {
	cothority.Suite = suites.MustFind("bn256.g1")
}
