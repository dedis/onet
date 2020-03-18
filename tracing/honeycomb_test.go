package tracing

import (
	"testing"
	"time"

	"go.dedis.ch/onet/v3/log"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/onet/v3"
)

func TestLoggingRealHC(t *testing.T) {
	t.Skip("this test is only useful for manual testing with honeycomb")
	log.AddUserUninterestingGoroutine("worker")
	log.AddUserUninterestingGoroutine("poll.runtime_pollWait")
	log.AddUserUninterestingGoroutine("writeLoop")
	log.SetDebugVisible(1)
	var tl *TraceLogger
	switch 0 {
	case 0:
		// Creates an internal simulation logger for manual debugging
		var sc *simulCollector
		sc, tl = newSimulLogger()
		defer sc.waitAndPrint()
	case 1:
		// Creates a honeycomb logger with stdout debug
		tl = NewHoneycombLoggerDebug()
	case 2:
		// Creates a real honeycomb logger - needs HONEYCOMB_API_KEY set to
		// API_KEY:dataset
		var err error
		tl, err = NewHoneycombLoggerFromEnv()
		require.NoError(t, err)
	}
	tl.NoSingleSpans = true
	l := onet.NewLocalTest(tSuite)
	defer l.CloseAll()
	_, err := onet.RegisterNewService(testService, newTService)
	require.NoError(t, err)
	s := l.GenServers(1)[0]
	tl.AddOnetDefaults(s.ServerIdentity)

	cl := onet.NewClient(tSuite, testService)
	require.NoError(t, cl.SendProtobuf(s.ServerIdentity, &Query{1}, nil))
	require.NoError(t, cl.SendProtobuf(s.ServerIdentity, &Query{1}, nil))

	time.Sleep(time.Second)
}

var testService = "testService"

type tService struct {
	*onet.ServiceProcessor
}

type Query struct {
	A int
}

func (t *tService) Reply(q *Query) (*Query, error) {
	testOne()
	testTwo()
	return q, nil
}

func testOne() {
	log.Lvl2("something")
	testTwo()
}

func testTwo() {
	log.Lvl2("more")
}

func newTService(c *onet.Context) (onet.Service, error) {
	t := &tService{
		ServiceProcessor: onet.NewServiceProcessor(c),
	}
	t.RegisterHandlers(t.Reply)
	return t, nil
}
