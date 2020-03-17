package tracing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"go.dedis.ch/onet/v3/log"
)

// The simulCollector is a testing structure that allows to collect all sent
// messages.
// It is mainly used to test the Trace, and *Wrapper structures.
// It simulates a honeycomb integration by simply collecting all fields set,
// and then passing them up when Send is called.
type simulCollector struct {
	Traces    [][]map[string]string
	tracesMux sync.Mutex
	Wg        sync.WaitGroup
}

func (s *simulCollector) newTrace(ctx context.Context, str string) (context.Context,
	Trace) {
	s.Wg.Add(1)
	newCtx, tr := newSimulTrace(ctx, str)
	go func() {
		<-newCtx.Done()
		s.tracesMux.Lock()
		s.Traces = append(s.Traces, tr.sent)
		s.tracesMux.Unlock()
		s.Wg.Done()
	}()
	return newCtx, tr
}

func (s *simulCollector) waitAndPrint() {
	s.Wg.Wait()
	for i, c := range s.Traces {
		for j, l := range c {
			fmt.Printf("%d / %d / %s:%s[%s] - %s\n", i, j,
				l["file"], l["method"], l["line"], l["log.Msg"])
			//fmt.Printf("%d/%d %+v\n", i, j, l)
		}
	}
}

type simulTrace struct {
	ctx    context.Context
	cancel context.CancelFunc
	kvs    map[string]string
	root   *simulSpan
	sent   []map[string]string
}

func newSimulTrace(ctx context.Context, s string) (context.Context, *simulTrace) {
	newCtx, cancel := context.WithCancel(ctx)
	tt := &simulTrace{
		kvs:    make(map[string]string),
		root:   newSimulSpan(),
		ctx:    newCtx,
		cancel: cancel,
	}
	tt.root.trace = tt
	return newCtx, tt
}

func (tt *simulTrace) AddField(key string, val interface{}) {
	tt.kvs[key] = valToJSON(val)
}

func (tt *simulTrace) GetRootSpan() Span {
	return tt.root
}
func (tt *simulTrace) Send() {
	tt.root.Send()
	for i := range tt.sent {
		for k, v := range tt.kvs {
			tt.sent[i][k] = v
		}
	}
	tt.cancel()
}

func (tt *simulTrace) printTrace() {
	tt.root.printTrace()
}

type simulSpan struct {
	kvs    map[string]string
	child  *simulSpan
	parent *simulSpan
	trace  *simulTrace
}

func newSimulSpan() *simulSpan {
	return &simulSpan{
		kvs: make(map[string]string),
	}
}

func (ts *simulSpan) AddField(key string, val interface{}) {
	ts.kvs[key] = valToJSON(val)
}
func (ts *simulSpan) CreateChild(ctx context.Context) (context.Context, Span) {
	ts.child = newSimulSpan()
	ts.child.parent = ts
	ts.child.trace = ts.trace
	return ctx, ts.child
}
func (ts *simulSpan) Send() {
	if ts.child != nil {
		ts.child.Send()
	}
	ts.trace.sent = append(ts.trace.sent, ts.kvs)
	if ts.parent != nil {
		ts.parent.child = nil
	}
}

func (ts *simulSpan) printTrace() {
	if ts.child != nil {
		ts.child.printTrace()
	}
}

func valToJSON(val interface{}) string {
	var buf = &bytes.Buffer{}
	var enc = json.NewEncoder(buf)
	log.ErrFatal(enc.Encode(val))
	return strings.TrimSpace(buf.String())
}

// newSimulLogger returns a new simulationCollector and a TraceLogger.
// You can wait on the simulCollector.
// Wg and get all traces in the simulCollector.Traces
func newSimulLogger() (*simulCollector, *TraceLogger) {
	sc := &simulCollector{}
	return sc, NewLogger(sc.newTrace)
}
