package tracing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// The traceWrapper and spanWrapper structures simulate a real tracing
// environment using the calls from onet/log.
// They are the same as defined by honeycomb:
// A trace contains one or more spans.
// A trace represents one specific interaction from the user.
// The spans are the different steps done to fulfil that interaction.
//
// In a real tracing environment,
// context.Context is handled down from the entry point to the sub-modules.
// This context is then used to link the spans together into traces.
// As we don't want to rewrite all of onet and cothority to use tracing,
// these Wrappers use a trick to simulate the contexts:
// The golang's stack-trace has enough information to guess quite accurately
// (not 100%, but 95% of the time) if the call to onet/log comes from a new
// trace or not.
// Using the information in the golang stack-trace,
// a context.Context environment is created and then passed to the tracing
// library.
//
// Currently there are two implementations of the Trace and Span:
//  - simul for tests that collect all traces in a simulCollector
//  - honeycomb that sends the traces to honeycomb
// It should be possible to add other tracing modules,
// like prometheus.io or zipkin.io.
// The implementation of these tracers is left as an exercise to the reader...

// Trace interfaces the methods that are used by the wrappers,
// for easier testing and eventual usage of other tools.
type Trace interface {
	// AddFiled adds a key/val to the trace.
	//This key/val pair will be set to all spans contained in this trace.
	AddField(key string, val interface{})
	// There is at least one span for each trace, which is the root span.
	GetRootSpan() Span
	// Finishes the trace and sends it to the main service.
	// Once Send is called,
	//further interactions with the Trace structure are not defined.
	Send()
}

// Span interfaces the methods that are used by the wrappers,
// for easier testing and eventual usage of other tools than HoneyComb.
type Span interface {
	AddField(key string, val interface{})
	CreateChild(ctx context.Context) (context.Context, Span)
	Send()
}

// traceWrapper keeps track of the stackEntries and creates / sends Spans as
// needed.
type traceWrapper struct {
	ctx     context.Context
	root    *spanWrapper
	hcTrace Trace
}

func newTraceWrapper(ctx context.Context, tr Trace,
	ses []stackEntry) (tw *traceWrapper, child *spanWrapper) {
	tw = &traceWrapper{
		ctx:     ctx,
		hcTrace: tr,
	}
	tw.root, child = newRootSpanWrapper(tw, ses)
	return
}

func (tw *traceWrapper) add(ns string, val interface{}) {
	structToFields(tw.hcTrace.AddField, ns, val)
}

func (tw *traceWrapper) stackToSpan(stack []stackEntry) *spanWrapper {
	return tw.root.stackToSpan(stack)
}

func (tw *traceWrapper) send() {
	tw.hcTrace.Send()
}

func (tw *traceWrapper) ses() (ses []stackEntry) {
	sw := tw.root
	for {
		ses = append(ses, sw.se)
		if sw.child == nil {
			return
		}
		sw = sw.child
	}
}

func (tw *traceWrapper) findGoroutine(in []stackEntry) []stackEntry {
	twSes := tw.ses()
	for t, set := range twSes {
		if in[0].samePkgMethodStr(set) {
			merged := append(twSes[0:t+1], in...)
			for m := range merged[t+1:] {
				merged[t+m].traceID = twSes[0].traceID
			}
			return merged
		}
	}
	return nil
}

// spanWrapper keeps track of the contexts created for the different spans
// and closes children as new spans arrive.
type spanWrapper struct {
	se     stackEntry
	ctx    context.Context
	trace  *traceWrapper
	parent *spanWrapper
	child  *spanWrapper
	hcSpan Span
}

func newSpanWrapper(ctx context.Context, t *traceWrapper, hcs Span, se stackEntry) *spanWrapper {
	newSpan := &spanWrapper{
		ctx:    ctx,
		trace:  t,
		hcSpan: hcs,
		se:     se,
	}
	se.addFields(hcs)
	return newSpan
}

func newRootSpanWrapper(t *traceWrapper, stack []stackEntry) (root,
	child *spanWrapper) {
	root = newSpanWrapper(t.ctx, t, t.hcTrace.GetRootSpan(), stack[0])
	if len(stack) > 1 {
		child = root.stackToSpan(stack)
	} else {
		child = root
	}
	return
}

func (sw *spanWrapper) add(ns string, val interface{}) {
	structToFields(sw.hcSpan.AddField, ns, val)
}

func (sw *spanWrapper) log(lvl int, msg string) {
	if sw.child != nil {
		sw.child.done()
	}
	_, sLog := sw.hcSpan.CreateChild(sw.ctx)
	msgStruct := struct {
		Lvl int
		Msg string
	}{lvl, msg}
	// This value is unfortunately not exported from onet/log/lvl.go
	if lvl > -17 {
		structToFields(sLog.AddField, "log", msgStruct)
	} else {
		structToFields(sLog.AddField, "error", msgStruct)
	}
	sw.se.addFields(sLog)
	sLog.Send()
}

func (sw *spanWrapper) done() {
	if sw.child != nil {
		sw.child.done()
	}
	sw.parent.child = nil
	sw.hcSpan.Send()
}

func (sw *spanWrapper) updateSE(se stackEntry) {
	sw.se = se
	se.addFields(sw.hcSpan)
}

func (sw *spanWrapper) stackToSpan(stack []stackEntry) *spanWrapper {
	if len(stack) == 1 {
		sw.updateSE(stack[0])
		return sw
	}
	if sw.child != nil {
		if sw.child.se.traceID != stack[1].traceID ||
			stack[0].line != sw.se.line {
			sw.child.done()
		}
	}
	if sw.child == nil {
		sw.child = sw.createChild(stack[1])
		sw.updateSE(stack[0])
	}
	return sw.child.stackToSpan(stack[1:])
}

func (sw *spanWrapper) createChild(se stackEntry) *spanWrapper {
	ctx, newHCS := sw.hcSpan.CreateChild(sw.ctx)
	newChild := newSpanWrapper(ctx, sw.trace, newHCS, se)
	newChild.parent = sw
	return newChild
}

type stackEntry struct {
	line      int
	path      string
	file      string
	pkgPath   string
	method    string
	params    string
	traceID   string
	createdBy bool
}

var stackFileLine = regexp.MustCompile(
	`\s*(.*):([0-9]*)( \+0x[a-f0-9]*$)?\s*`)
var stackPkgMethod = regexp.MustCompile(
	`\s*(created by )?(.*)\.(.*?\w)(\(.*\))?\s*$`)
var methodParams = regexp.MustCompile(`\(\)$`)

func newStackEntry(method, file, traceID string) (se stackEntry, err error) {
	se.traceID = traceID
	fileLine := stackFileLine.FindStringSubmatch(file)
	if len(fileLine) < 2 {
		return se, errors.New("didn't find file and line number")
	}
	se.line, err = strconv.Atoi(fileLine[2])
	if err != nil {
		se.line = -1
	}
	se.path, se.file = path.Split(fileLine[1])

	pmp := stackPkgMethod.FindStringSubmatch(method)
	if len(pmp) < 4 {
		return se, errors.New("didn't find pkg and method")
	}
	se.createdBy = pmp[1] != ""
	se.pkgPath = pmp[2]
	se.method = pmp[3]
	if len(pmp) == 5 {
		se.params = methodParams.ReplaceAllString(pmp[4], "")
	}
	return se, nil
}

var stackGoID = regexp.MustCompile(`goroutine (.*) \[(.*)\]`)

func getGoID(stack string) string {
	id := stackGoID.FindStringSubmatch(strings.Split(stack, "\n")[0])
	if len(id) != 3 {
		return "unknownID"
	}
	return id[1]
}

func parseLogs(stack string, idMap map[string]string) (ses []stackEntry) {
	var lvl []string
	// prepend "go " or "id " to differentiate an automatic ID from a real one.
	traceID := getGoID(stack)
	if tid, ok := idMap[traceID]; ok {
		traceID = "id_" + tid
	} else {
		traceID = "go_" + traceID
	}
	stackLines := strings.Split(stack, "\n")
	for _, s := range stackLines[1:] {
		lvl = append(lvl, s)
		if len(lvl) == 2 {
			se, err := newStackEntry(lvl[0], lvl[1], traceID)
			if err == nil {
				ses = append([]stackEntry{se}, ses...)
			}
			lvl = []string{}
		}
	}
	return
}

func mergeLogs(known map[string]*traceWrapper,
	in []stackEntry) (out []stackEntry) {
	// First check if there is a common traceID - else check if there is a
	// go-routine we can attach to.
	for idIn, seIn := range in {
		for _, tw := range known {
			if seIn.traceID == tw.root.se.traceID {
				if out = tw.findGoroutine(in[idIn:]); out != nil {
					return
				}
			}
		}
		// No common traceID found - check goroutine calls
		for id, tw := range known {
			if strings.HasPrefix(tw.root.se.traceID, "id_") ||
				strings.HasPrefix(seIn.traceID, "id_") {
				continue
			}
			if out = tw.findGoroutine(in[idIn:]); out != nil {
				fmt.Println("merged go-routines", id, in)
				return
			}
		}
	}
	return in
}

func (se stackEntry) pkgMethod() string {
	return fmt.Sprintf("%s.%s", se.pkgPath, se.method)
}

func (se stackEntry) checkEntryPoint(eps []string) (entryPoint string) {
	for _, entryPoint = range eps {
		if strings.Contains(se.pkgMethod(), entryPoint) {
			return
		}
	}
	return ""
}

func (se stackEntry) pkgMethodStr() string {
	return fmt.Sprintf("%s.%s", se.pkgPath, se.method)
}

func (se stackEntry) samePkgMethodStr(other stackEntry) bool {
	return se.pkgMethodStr() == other.pkgMethodStr()
}

func (se stackEntry) uniqueID() string {
	return fmt.Sprintf("%s - %s.%s%s", se.traceID, se.pkgPath, se.method,
		se.params)
}

func (se stackEntry) addFields(s Span) {
	if se.params != "" {
		s.AddField("params", se.params)
	}
	if se.method != "" {
		s.AddField("method", se.method)
	}
	if se.pkgPath != "" {
		s.AddField("pkgPath", se.pkgPath)
	}
	if se.file != "" {
		s.AddField("file", se.file)
	}
	if se.path != "" {
		s.AddField("path", se.path)
	}
	if se.line != 0 {
		s.AddField("line", se.line)
	}
}

func (se stackEntry) String() string {
	return fmt.Sprintf("%s:%d", se.method, se.line)
}

type fieldAdder = func(key string, val interface{})

func structToFields(adder fieldAdder, ns string, val interface{}) {
	var mapStrStr map[string]interface{}
	buf, _ := json.Marshal(val)
	json.Unmarshal(buf, &mapStrStr)
	if len(mapStrStr) == 0 {
		// Need to take care when recursively encoding strings,
		// else JSON will happily add " at every level...
		rv := reflect.ValueOf(val)
		if rv.Kind() == reflect.String {
			adder(ns, strings.Trim(rv.String(), `"`))
		} else {
			adder(ns, val)
		}
	} else {
		for k, v := range mapStrStr {
			structToFields(adder, ns+"."+k, v)
		}
	}
}
