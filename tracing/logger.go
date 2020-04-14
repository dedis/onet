package tracing

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/load"
	"github.com/shirou/gopsutil/net"

	"github.com/shirou/gopsutil/cpu"
	"go.dedis.ch/onet/v3/network"

	"go.dedis.ch/onet/v3"

	"go.dedis.ch/onet/v3/log"
)

// The TraceLogger implements the interface Logger to be registered in onet/log.
// It has also some convenience methods to connect to a standard cothority
//installation, including putting the nodeName in the logging information.
type TraceLogger struct {
	// Don't create spans for calls that are not in entryPoints
	NoSingleSpans bool
	// Print spans that are not in entryPoints so that the slice can be updated.
	PrintSingleSpans int
	// As the TraceLogger cannot use onet/log, turn on/off debug messages here.
	TraceDebug bool
	// currently active traces - the keys are the traceID or the go-routine
	// id of the current call
	traces map[string]*traceWrapper
	// used to create a new trace,
	// so the TraceLogger can work with simulation, honeycomb,
	// or your own extension.
	hcn newTrace
	// which pkgMath/methods are interpreted as entry-points that can have
	// other go-routines and/or traceIDs linked to them.
	entryPoints []string
	// which log-message should be interpreted as closing the trace
	doneMsgs []string
	// these fields will be set in all traces
	defaultFields map[string]string
	// maps goroutine-ids to traceID, filled up by TraceLogger.TraceID
	goToTraceID map[string]string
	// telling onet/log what this TraceLogger needs
	logInfo log.LoggerInfo
	// tells the automatic stats-service to shut down
	statsDone chan bool
	// protects common fields
	logMutex sync.Mutex
	// our entry in onet/log
	loggerID int
}

type newTrace = func(context.Context, string) (context.Context, Trace)

// Log implements the Logger interface.
// It calls the getTraceSpan with the appropriate callLvl to remove the stack
// trace due to onet/log.
func (logger *TraceLogger) Log(level int, msg string) {
	logger.getTraceSpan(level, msg, 7)
}

// Close implements the Logger interface.
func (logger *TraceLogger) Close() {
	close(logger.statsDone)
}

// GetLoggerInfo implements the Logger interface.
func (logger *TraceLogger) GetLoggerInfo() *log.LoggerInfo {
	return &logger.logInfo
}

// AddOnetDefaults sets a number of entry-points that are useful when
//running a project using onet.
// These entry-points allow to track quite accurately what is happening,
//both for the servie-calls over websockets, as well as the protocol-messages.
func (logger *TraceLogger) AddOnetDefaults(si *network.ServerIdentity) {
	logger.AddEntryPoints(
		"go.dedis.ch/onet/v3.wsHandler.ServeHTTP",
		"go.dedis.ch/onet/v3/network.(*BlockingDispatcher).Dispatch",
		"go.dedis.ch/onet/v3/network.(*RoutineDispatcher).Dispatch",
		"go.dedis.ch/cothority/v3/blscosi/protocol.(*SubBlsCosi).dispatchLeaf",
		"go.dedis.ch/onet/v3.(*TreeNodeInstance).dispatchMsgToProtocol",
		"go.dedis.ch/onet/v3.(*Overlay).TransmitMsg",
		"go.dedis.ch/onet/v3.(*TreeNodeInstance).dispatchMsgReader")
	logger.AddDoneMsgs("ws close", "done tracing")

	logger.defaultFields["nodeName"] = si.String()
	logger.defaultFields["nodeDescription"] = si.Description
	logger.PrintSingleSpans = 10
}

// AddEntryPoints takes all given entry points and adds them to the internal
// list.
// Empty entry points are discarded.
func (logger *TraceLogger) AddEntryPoints(eps ...string) {
	for _, ep := range eps {
		if len(ep) > 0 {
			logger.entryPoints = append(logger.entryPoints, ep)
		}
	}
}

// AddDoneMsgs takes all given done messages and adds them to the internal list.
// Empty done messages are discarded.
func (logger *TraceLogger) AddDoneMsgs(dms ...string) {
	for _, dm := range dms {
		if len(dm) > 0 {
			logger.doneMsgs = append(logger.doneMsgs, dm)
		}
	}
}

// AddStats sends statistics of the current node on a regular basis to the
//tracing service.
// The sending stops once Logger.Close is called.
func (logger *TraceLogger) AddStats(c *onet.Context, repeat time.Duration) {
	go func() {
		for {
			select {
			case <-time.After(repeat):
				// Create a new trace that points to a dummy stackEntry,
				//so the status can be sent to the service.
				t, _ := logger.newTrace(context.TODO(), "",
					stackEntry{pkgPath: "go.dedis.ch/onet/v3/honeycomb",
						method: "stats"})
				t.add("status", c.ReportStatus())
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				t.add("memstats", m)
				var gc debug.GCStats
				debug.ReadGCStats(&gc)
				t.add("gcstats", gc)
				ld, err := load.Avg()
				if err == nil {
					t.add("load", ld)
				}
				us, err := disk.Usage(".")
				if err == nil {
					t.add("disk-usage", us)
				}
				ioc, err := disk.IOCounters(".")
				if err == nil {
					t.add("disk-iostat", ioc)
				}
				netio, err := net.IOCounters(false)
				if err == nil {
					t.add("network-stat", netio)
				}

				t.send()
			case <-logger.statsDone:
				return
			}
		}
	}()
}

// AddEnvironment reads the environment variables defined to initialize the
// variables.
// The following environmental variables are available:
//   - TRACING_PRINT_SINGLE_SPANS - output a stack trace of single spans to
//  let you trace where you could/should add a TRACING_ENTRY_POINT
//   - TRACING_CREATE_SINGLE_SPANS - whenever there is no entry point found,
//  the system can create single spans that are not linked together.
//   This is a fallback to regular logging when we cannot simulate traces.
//   - TRACING_ENTRY_POINTS - a "::" separated list of entry points that can
//  be used to refine the tracing.
//  The name of the entry points are the same as given by
//  TRACING_PRINT_SINGLE_SPAN
//   - TRACING_DONE_MSGS - a "::" separated list of done msgs that will close
//  the started trace. This is due to the simulation of actual traces,
//  because we can't really know when the trace should end.
func (logger *TraceLogger) AddEnvironment() error {
	if envPSS := os.Getenv("TRACING_PRINT_SINGLE_SPANS"); envPSS != "" {
		var err error
		logger.PrintSingleSpans, err = strconv.Atoi(envPSS)
		if err != nil {
			return fmt.Errorf("while reading TRACING_PRINT_SINGLE_SPAN: %v",
				err)
		}
	}
	tcss := strings.ToLower(os.Getenv("TRACING_CREATE_SINGLE_SPANS"))
	if tcss != "" {
		if tcss != "true" && tcss != "false" {
			return fmt.Errorf("while reading TRACING_CREATE_SINGLE_SPANS: can" +
				" be only \"true\" or \"false\"")
		}
		logger.NoSingleSpans = tcss == "true"
	}
	logger.AddEntryPoints(strings.Split(os.Getenv("TRACING_ENTRY_POINTS"), "::")...)
	logger.AddDoneMsgs(strings.Split(os.Getenv("TRACING_DONE_MSGS"), "::")...)
	return nil
}

// TraceID stores the go-routine ID / TraceID pair for help in merging new
// go-routines.
func (logger *TraceLogger) TraceID(id []byte) {
	logger.logMutex.Lock()
	defer logger.logMutex.Unlock()
	logger.goToTraceID[getGoID(log.Stack())] = fmt.Sprintf("%x", id)
}

// getTraceSpan checks whether this trace is already known.
// If it is known, it adds a new span to it to create the log.
// If it is unknown, it checks if it should create a new span.
// If it is not a span to be created,
// it checks if it should create a singleTrace.
func (logger *TraceLogger) getTraceSpan(lvl int, msg string, callLvl int) (*traceWrapper,
	*spanWrapper) {
	logger.logMutex.Lock()
	defer logger.logMutex.Unlock()
	ses := parseLogs(log.Stack(), logger.goToTraceID)
	ses = ses[0 : len(ses)-callLvl]
	ses = mergeLogs(logger.traces, ses)
	for i, se := range ses {
		tr, ok := logger.traces[se.traceID]
		if ok {
			for _, done := range logger.doneMsgs {
				if strings.Contains(msg, done) {
					if logger.TraceDebug {
						fmt.Println("-- found done for", se.uniqueID())
					}
					delete(logger.traces, se.traceID)
					tr.send()
					return nil, nil
				}
			}
			if logger.TraceDebug {
				fmt.Println("-- adding log to", se.uniqueID(), msg)
			}
			sw := tr.stackToSpan(ses[i:])
			sw.log(lvl, msg)
			return tr, sw
		}

		// Check if there is a new trace to be generated
		if se.checkEntryPoint(logger.entryPoints) != "" {
			if logger.TraceDebug {
				fmt.Println("-- new trace", se.uniqueID())
				for _, s := range ses[i:] {
					fmt.Println("-- ", s.uniqueID())
				}
			}
			var child *spanWrapper
			tw, sw := logger.newTrace(context.TODO(), "", ses[i:]...)
			logger.addDefaultFields(tw, sw)
			logger.traces[se.traceID], child = tw, sw
			child.log(lvl, msg)
			return logger.traces[se.traceID], child
		}
	}
	if logger.PrintSingleSpans > 0 {
		fmt.Printf("Creating single trace for '%s' from:\n",
			msg)
		for i, se := range ses {
			if i >= logger.PrintSingleSpans {
				break
			}
			fmt.Println("\t", se.pkgMethod())
		}
	}
	if !logger.NoSingleSpans {
		tr, sw := logger.newTrace(context.TODO(), "", ses...)
		tr.hcTrace.AddField("singleTrace", true)
		sw.log(lvl, msg)
		tr.send()
	}
	return nil, nil
}

func (logger *TraceLogger) addDefaultFields(tw *traceWrapper, sw *spanWrapper) {
	for k, v := range logger.defaultFields {
		tw.hcTrace.AddField(k, v)
	}
	ts, err := cpu.Times(false)
	if err == nil {
		for i, t := range ts {
			sw.add("cpustat"+strconv.Itoa(i), t)
		}
	}
}

func (logger *TraceLogger) newTrace(ctx context.Context, str string,
	ses ...stackEntry) (*traceWrapper,
	*spanWrapper) {
	ctx, newTr := logger.hcn(ctx, str)
	return newTraceWrapper(ctx, newTr, ses)
}

// NewLogger returns a new TraceLogger,
//already registered to the logging system.
// You might want to use the NewHoneyCombLogger or newSimulLogger instead,
//which make it easier to set up the tracing logger.
func NewLogger(nt newTrace) *TraceLogger {
	l := &TraceLogger{
		logInfo:       log.LoggerInfo{DebugLvl: 5, RawMessage: true},
		defaultFields: map[string]string{},
		goToTraceID:   map[string]string{},
		statsDone:     make(chan bool),
		hcn:           nt,
		traces:        make(map[string]*traceWrapper),
	}
	l.loggerID = log.RegisterLogger(l)
	return l
}
