package tracing

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/honeycombio/beeline-go"

	"github.com/honeycombio/beeline-go/trace"
)

// The HoneyComb implementation is mostly a wrapper to convince go that it's
// fine returning structures as interfaces.
// A Honeycomb logger can either be initialised using
//  - NewHoneycombLoggerDebug to output on stdout all that would be sent to
//    honeycomb
//  - NewHoneycombLogger if you know the api key and the dataset
//  - NewHoneycombLoggerFromEnv if you want the user to chose them using an
//    environment variable.

type honeycombTrace struct {
	*trace.Trace
	ctx context.Context
}

func (ht *honeycombTrace) GetRootSpan() Span {
	return &honeycombSpan{ht.Trace.GetRootSpan()}
}

type honeycombSpan struct {
	*trace.Span
}

func (hs *honeycombSpan) CreateChild(ctx context.Context) (context.Context,
	Span) {
	newCtx, sp := hs.Span.CreateChild(ctx)
	return newCtx, &honeycombSpan{sp}
}

func newHoneycombTrace(ctx context.Context, str string) (context.Context,
	Trace) {
	hct := &honeycombTrace{}
	hct.ctx, hct.Trace = trace.NewTrace(ctx, str)
	return hct.ctx, hct
}

// NewHoneycombLoggerDebug sets up a new honeycomb logger that will output
// all its data traces to stdout. Perfect for debugging...
func NewHoneycombLoggerDebug() *TraceLogger {
	beeline.Init(beeline.Config{
		WriteKey: "1234",
		Dataset:  "test",
		STDOUT:   true,
	})
	return NewLogger(newHoneycombTrace)
}

// NewHoneycombLogger sets up a new logger that is connected to a honeycomb
// trace-storage.
// The API can be found when signing up for a free account.
func NewHoneycombLogger(api, dataset string) *TraceLogger {
	beeline.Init(beeline.Config{
		WriteKey: api,
		Dataset:  dataset,
	})
	return NewLogger(newHoneycombTrace)
}

// NewHoneycombLoggerFromEnv checks the HONEYCOMB_API_KEY and extracts the
// api and dataset from it.
func NewHoneycombLoggerFromEnv() (*TraceLogger, error) {
	hcenv := os.Getenv("HONEYCOMB_API_KEY")
	if hcenv == "" {
		return nil, nil
	}
	keyData := strings.Split(hcenv, ":")
	if len(keyData) != 2 {
		return nil, errors.New("need 'api_key:dataset' in HONEYCOMB_API_KEY")
	}
	return NewHoneycombLogger(keyData[0], keyData[1]), nil
}
