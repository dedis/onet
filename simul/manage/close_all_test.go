package manage

import (
	"testing"
	"time"

	"go.dedis.ch/onet/v3"
	"go.dedis.ch/onet/v3/ciphersuite"
	"go.dedis.ch/onet/v3/log"
)

var testBuilder = onet.NewDefaultBuilder()

func TestMain(m *testing.M) {
	testBuilder.SetSuite(ciphersuite.NewEd25519CipherSuite())
	log.MainTest(m)
}

// Tests a 2-node system
func TestCloseAll(t *testing.T) {
	local := onet.NewLocalTest(testBuilder)
	nbrNodes := 2
	_, _, tree := local.GenTree(nbrNodes, true)
	defer local.CloseAll()

	pi, err := local.CreateProtocol("CloseAll", tree)
	if err != nil {
		t.Fatal("Couldn't start protocol:", err)
	}
	done := make(chan bool)
	go func() {
		pi.Start()
		done <- true
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Didn't finish in 10 seconds")
	}
}
