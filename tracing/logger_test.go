package tracing

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"go.dedis.ch/onet/v3/log"

	"go.dedis.ch/kyber/v3/suites"

	"github.com/stretchr/testify/require"
)

var tSuite = suites.MustFind("Ed25519")

var doneMeasuring = "done tracing"

func TestStack(t *testing.T) {
	sc, tr := newSimulLogger()
	defer log.UnregisterLogger(tr.loggerID)
	tr.PrintSingleSpans = 10
	tr.AddEntryPoints("go.dedis.ch/onet/v3/tracing.one")
	tr.AddDoneMsgs(doneMeasuring)
	one(tr, 1)
	one(tr, 2)
	sc.Wg.Wait()
	require.Equal(t, 2, len(sc.Traces))
	for i, methods := range [][]string{
		{"one", "two", "two", "two", "one", "one", "one"},
		{"one", "two", "two", "two", "two", "two", "one", "one", "one"}} {
		for j, method := range methods {
			require.Equal(t, method,
				strings.Trim(sc.Traces[i][j]["method"], `""`),
				fmt.Sprintf("%d / %d", i, j))
		}
	}
}

func one(tr *TraceLogger, i int) {
	tr.getTraceSpan(1, "one-1", 3)
	for j := 0; j < i; j++ {
		two(tr, j)
	}
	tr.getTraceSpan(1, "one-2", 3)
	tr.getTraceSpan(1, doneMeasuring, 3)
}

func two(tr *TraceLogger, i int) {
	tr.getTraceSpan(1, "two-1", 3)
	tr.getTraceSpan(1, "two-2", 3)
}

func TestGoroutines(t *testing.T) {
	sc, tr := newSimulLogger()
	defer log.UnregisterLogger(tr.loggerID)
	//tr.PrintSingleSpans = 10
	tr.AddEntryPoints("go.dedis.ch/onet/v3/tracing.goroutines")
	tr.AddDoneMsgs("done goroutine")
	goroutines(0)
	sc.waitAndPrint()
}

func goroutines(i int) {
	log.Lvl2("new goroutine", i)
	wg := sync.WaitGroup{}
	wg.Add(2)
	go subgo(&wg)
	go subgo(&wg)
	wg.Wait()
	log.Lvl2("done goroutine")
}

func subgo(wg *sync.WaitGroup) {
	log.Lvl3("sub-goroutine")
	wg.Done()
}
