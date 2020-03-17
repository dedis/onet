package service

import (
	"fmt"
	"time"

	"go.dedis.ch/onet/v3"
	"go.dedis.ch/onet/v3/log"
	"go.dedis.ch/onet/v3/tracing"
)

// Import this service for most automatic use of the tracing system.
// All you need to do is to set the following environment variable:
//   HONEYCOMB_API_KEY="hex_key:dataset"
// And then this service will send all traces to your honeycomb account.
// It will track automatically what is happening in your code and send it to
//your honeycomb account.
// There are additional environmental variables described in
//   onet/tracing/logger.go

// Name of the service.
var Name = "TracingService"
var loggerCounter = 0

func init() {
	_, err := onet.RegisterNewService(Name, newTracer)
	log.ErrFatal(err)
}

type tracer struct {
	*onet.ServiceProcessor
	tl *tracing.TraceLogger
}

func newTracer(c *onet.Context) (onet.Service, error) {
	if loggerCounter > 0 {
		log.Warn("can only start one service for tracing")
		return nil, nil
	}
	loggerCounter++
	tl, err := tracing.NewHoneycombLoggerFromEnv()
	if err != nil {
		return nil, fmt.Errorf("couldn't init honeycomb: %v", err)
	}
	if tl == nil {
		log.Info("Not starting Honeycomb tracing as HONEYCOMB_API_KEY not" +
			" present")
		return nil, nil
	}
	err = tl.AddEnvironment()
	if err != nil {
		return nil, fmt.Errorf("couldn't interpret environment variables: %v",
			err)
	}
	tl.AddOnetDefaults(c.ServerIdentity())
	tl.AddStats(c, time.Minute)
	log.Info("Tracing with HoneyComb successfully set up")
	return &tracer{
		ServiceProcessor: onet.NewServiceProcessor(c),
		tl:               tl,
	}, nil
}
