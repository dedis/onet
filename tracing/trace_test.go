package tracing

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"go.dedis.ch/onet/v3/log"

	"github.com/stretchr/testify/require"
)

func TestNewStackEntry(t *testing.T) {
	for i, test := range []struct {
		method string
		file   string
		se     stackEntry
		err    bool
	}{
		{"runtime/debug.Stack(0x11953ba, 0x1089998, 0xad8eee637bfa)",
			"/usr/local/Cellar/go/1.13.3/libexec/src/runtime/debug/stack." +
				"go:24 +0xab",
			stackEntry{
				24, "/usr/local/Cellar/go/1.13.3/libexec/src/runtime/debug/",
				"stack.go",
				"runtime/debug", "Stack",
				"(0x11953ba, 0x1089998, 0xad8eee637bfa)", "", false,
			}, false,
		}, {"go.dedis.ch/onet/v3/log.Stack(...)",
			"/home/foo/onet/log/testutil.go:105",
			stackEntry{
				105, "/home/foo/onet/log/",
				"testutil.go",
				"go.dedis.ch/onet/v3/log", "Stack",
				"(...)", "", false,
			}, false,
		}, {"go.dedis.ch/onet/v3/honeycomb.TestNewTrace(0xc0001b2200)",
			"/home/foo/onet/honeycomb/trace_test.go:19",
			stackEntry{
				line: 19, path: "/home/foo/onet" +
					"/honeycomb/", file: "trace_test.go",
				pkgPath: "go.dedis.ch/onet/v3/honeycomb", method: "TestNewTrace",
				params: "(0xc0001b2200)",
			}, false,
		}, {"testing.tRunner",
			"/usr/local/Cellar/go/1.13.3/libexec/src/testing/testing.go:909 +0x19a",
			stackEntry{
				line: 909, path: "/usr/local/Cellar/go/1.13.3/libexec/src/testing/",
				file:    "testing.go",
				pkgPath: "testing", method: "tRunner", params: "",
			}, false,
		}, {"created by testing.(*T).Run",
			"/usr/local/Cellar/go/1.13.3/libexec/src/testing/testing.go:960 +0x652",
			stackEntry{
				line: 960, path: "/usr/local/Cellar/go/1.13.3/libexec/src/testing/",
				file:    "testing.go",
				pkgPath: "testing.(*T)", method: "Run",
				params: "", createdBy: true,
			}, false,
		},
		{"created by testing.(*T).Run",
			"/usr/local/Cellar/go/1.13.3/libexec/src/testing/testing.go:960 +0x652",
			stackEntry{
				line: 960, path: "/usr/local/Cellar/go/1.13.3/libexec/src/testing/",
				file:    "testing.go",
				pkgPath: "testing.(*T)", method: "Run", params: "",
				createdBy: true,
			}, false,
		},
		{"created by testing.(*T).Run",
			"/usr/local/Cellar/go/1.13.3/libexec/src/testing/testing.go",
			stackEntry{createdBy: true}, true,
		},
		{"created by testing",
			"/usr/local/Cellar/go/1.13.3/libexec/src/testing/testing.go:960 +0x652",
			stackEntry{createdBy: true}, true,
		},
		{
			"go.dedis.ch/onet/v3/honeycomb.(*oneStr).more.(*another).one(" +
				"0x58fe970, 0xc0001f0410, 0x14)",
			"/home/foo/onet/honeycomb/stack_test.go:32 +0x34",
			stackEntry{32, "/home/foo/onet/honeycomb/", "stack_test.go",
				"go.dedis.ch/onet/v3/honeycomb.(*oneStr).more.(*another)",
				"one", "(0x58fe970, 0xc0001f0410, 0x14)", "", false,
			}, false,
		},
	} {
		se, err := newStackEntry(test.method, test.file, "")
		if test.err {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			require.Equal(t, test.se, se, fmt.Sprint("Failed for: ", i))
		}
	}
}

func TestParseLogs(t *testing.T) {
	for _, test := range []struct {
		logs string
		ses  []stackEntry
	}{
		{
			`runtime/debug.Stack(0x11953ba, 0x1009c10, 0xc00004ff18)
			/usr/local/Cellar/go/1.13.3/libexec/src/runtime/debug/stack.go:24 +0xab
			go.dedis.ch/onet/v3/log.Stack(...)
			/home/foo//onet/log/testutil.go:105`,
			[]stackEntry{
				{line: 105, path: "/home/foo//onet/log/",
					file:    "testutil.go",
					pkgPath: "go.dedis.ch/onet/v3/log", method: "Stack", params: "(...)"},
				{line: 24, path: "/usr/local/Cellar/go/1.13.3/libexec/src/runtime/debug/",
					file:    "stack.go",
					pkgPath: "runtime/debug", method: "Stack",
					params: "(0x11953ba, 0x1009c10, 0xc00004ff18)"},
			},
		},
		{
			`runtime/debug.Stack(0x11953ba, 0x1009c10, 0xc00004ff18)
			/usr/local/Cellar/go/1.13.3/libexec/src/runtime/debug/stack.go:24 +0xab
			/home/foo//onet/log/testutil.go:105`,
			[]stackEntry{
				{line: 24, path: "/usr/local/Cellar/go/1.13.3/libexec/src/runtime/debug/",
					file:    "stack.go",
					pkgPath: "runtime/debug", method: "Stack",
					params: "(0x11953ba, 0x1009c10, 0xc00004ff18)"},
			},
		},
		{
			`runtime/debug.Stack(0x11953ba, 0x1009c10, 0xc00004ff18)
			/usr/local/Cellar/go/1.13.3/libexec/src/runtime/debug/stack.go`,
			nil,
		},
	} {
		ses := parseLogs("goroutine t [test]\n"+test.logs, map[string]string{})
		for s := range test.ses {
			test.ses[s].traceID = "go_t"
		}
		require.Equal(t, test.ses, ses, fmt.Sprintf("logs: %s\nses: %+v",
			test.logs, test.ses))
	}
}

func simpleSe(path ...string) (ses []stackEntry) {
	for _, p := range path {
		ses = append(ses, stackEntry{method: p})
	}
	return
}

func TestNewStacked(t *testing.T) {
	ctx, hcTr := newSimulTrace(context.TODO(), "")
	tr, _ := newTraceWrapper(ctx, hcTr, simpleSe("root"))
	stack1 := simpleSe("root", "one")
	stack12 := simpleSe("root", "one", "two")
	stack2 := simpleSe("root", "two")
	stack23 := simpleSe("root", "two", "three")
	stack234 := simpleSe("root", "two", "three", "four")
	tr.stackToSpan(stack1).log(1, "one-1")
	tr.stackToSpan(stack12).log(1, "one/two-1")
	tr.stackToSpan(stack12).log(1, "one/two-2")
	tr.stackToSpan(stack1).log(1, "one-2")
	tr.stackToSpan(stack2).log(1, "two-1")
	tr.stackToSpan(stack23).log(1, "two/three-1")
	tr.stackToSpan(stack2).log(1, "two-2")
	tr.stackToSpan(stack23).log(1, "two/three-2")
	tr.stackToSpan(stack234).log(1, "two/three-2/four")
	tr.stackToSpan(stack2).log(1, "two-3")
	tr.send()
	testSentTrace(t, tr,
		`log.Lvl:1;log.Msg:"one-1";method:"one"`,
		`log.Lvl:1;log.Msg:"one/two-1";method:"two"`,
		`log.Lvl:1;log.Msg:"one/two-2";method:"two"`,
		`method:"two"`,
		`log.Lvl:1;log.Msg:"one-2";method:"one"`,
		`log.Lvl:1;log.Msg:"two-1";method:"two"`,
		`log.Lvl:1;log.Msg:"two/three-1";method:"three"`,
		`method:"three"`,
		`log.Lvl:1;log.Msg:"two-2";method:"two"`,
		`log.Lvl:1;log.Msg:"two/three-2";method:"three"`,
		`log.Lvl:1;log.Msg:"two/three-2/four";method:"four"`,
		`method:"four"`,
		`method:"three"`,
		`log.Lvl:1;log.Msg:"two-3";method:"two"`,
		`method:"two"`, `method:"root"`)
}

func TestTraceWrapper_Add(t *testing.T) {
	skv := &storeKVs{}
	for _, test := range []struct {
		val interface{}
		kv  map[string]string
	}{
		{1, map[string]string{"base": "1"}},
		{ab{`"AA"`, 2},
			map[string]string{"base.A": `"AA"`, "base.B": "2"}},
		{struct {
			C ab
			D int
		}{ab{"E", 3}, 2},
			map[string]string{
				"base.C.A": `"E"`,
				"base.C.B": "3",
				"base.D":   "2"}},
		{struct{ D cab }{
			cab{ab{"CAB", 3}}},
			map[string]string{
				"base.D.C.A": `"CAB"`,
				"base.D.C.B": "3"}},
		{[]ab{
			{"A", 1},
			{"B", 2},
		},
			map[string]string{"base[0].A": `"A"`, "base[0].B": "1",
				"base[1].A": `"B"`, "base[1].B": `2`}},
	} {
		skv.kvs = map[string]string{}
		structToFields(skv.AddField, "base", test.val)
		require.Equal(t, test.kv, skv.kvs)
	}
}

type ab struct {
	A string
	B int
}

type cab struct {
	C ab
}

func TestMerge(t *testing.T) {
	for _, tms := range testMergeStack[1:] {
		ctx, hcTr := newSimulTrace(context.TODO(), "")
		ses := parseLogs(tms[1], map[string]string{})
		known, _ := newTraceWrapper(ctx, hcTr, ses)
		sesNew := known.findGoroutine(parseLogs(tms[0], map[string]string{}))
		require.NotNil(t, sesNew)
		require.NotEqual(t, 0, len(sesNew))
		require.Equal(t, ses[0].traceID, sesNew[0].traceID)
	}
}

var testMergeStack = [][]string{{`goroutine 2 [running]:
go.dedis.ch/onet/v3/tracing.tg()
/home/foo//onet/tracing/logger_test.go:116 +0x44
created by go.dedis.ch/onet/v3/tracing.TestGor
/home/foo//onet/tracing/logger_test.go:108 +0x68
`, `goroutine 1 [runnable]:
sync.(*WaitGroup).Wait(0xc0001ce070)
/usr/local/Cellar/go/1.13.3/libexec/src/sync/waitgroup.go:103 +0x148
go.dedis.ch/onet/v3/tracing.tg()
/home/foo//onet/tracing/logger_test.go:136 +0x183
created by go.dedis.ch/onet/v3/tracing.TestGor
/home/foo//onet/tracing/logger_test.go:107 +0x50
`},
	{
		`goroutine 2 [running]:
runtime/debug.Stack(0x3, 0x3, 0xc00042dfb8)
	/usr/local/Cellar/go/1.13.3/libexec/src/runtime/debug/stack.go:24 +0xab
go.dedis.ch/onet/v3/log.Stack(...)
	/home/foo//onet/log/testutil.go:105
go.dedis.ch/onet/v3/tracing.subgo(0xc000388410)
	/home/foo//onet/tracing/logger_test.go:85 +0x87
created by go.dedis.ch/onet/v3/tracing.goroutines
	/home/foo//onet/tracing/logger_test.go:78 +0x209`,
		`goroutine 1 [running]:
runtime/debug.Stack(0x2, 0x3, 0xc0000c7f80)
	/usr/local/Cellar/go/1.13.3/libexec/src/runtime/debug/stack.go:24 +0xab
go.dedis.ch/onet/v3/log.Stack(...)
	/home/foo//onet/log/testutil.go:105
go.dedis.ch/onet/v3/tracing.goroutines(0x0)
	/home/foo//onet/tracing/logger_test.go:74 +0xc7
go.dedis.ch/onet/v3/tracing.TestGoroutines.func1(0xc0001e8620, 0x0)
	/home/foo//onet/tracing/logger_test.go:56 +0x39
created by go.dedis.ch/onet/v3/tracing.TestGoroutines
	/home/foo//onet/tracing/logger_test.go:55 +0x1eb`,
	}}

func TestTraceID(t *testing.T) {
	sc, tr := newSimulLogger()
	defer log.UnregisterLogger(tr.loggerID)
	if testing.Verbose() {
		tr.PrintSingleSpans = 10
		tr.TraceDebug = true
	}
	tr.AddEntryPoints("go.dedis.ch/onet/v3/tracing.setTraceID")
	tr.AddDoneMsgs("done trace")
	syncTrace[0] = make(chan bool, 2)
	syncTrace[1] = make(chan bool, 2)
	setTraceID(0, "get")
	setTraceID(1, "set")

	setTraceID(0, "set")
	setTraceID(1, "get")

	setTraceID(0, "done")
	setTraceID(1, "done")
	sc.Wg.Wait()
	sc.waitAndPrint()
	require.Equal(t, 2, len(sc.Traces))
}

// used to make sure that the 'done' comes _after_ the get and set method
var syncTrace [2]chan bool

func setTraceID(id int, cmd string) {
	log.TraceID([]byte{byte(id)})
	log.Lvl2("adding traceID")
	go traceCmd(id, cmd)
}

func traceCmd(id int, cmd string) {
	log.TraceID([]byte{byte(id)})
	log.Lvl2("cmd:", cmd)
	if cmd == "done" {
		<-syncTrace[id]
		<-syncTrace[id]
		log.Lvl2("done trace")
	} else {
		syncTrace[id] <- true
	}
}

type storeKVs struct {
	kvs map[string]string
}

func (s *storeKVs) AddField(key string, val interface{}) {
	s.kvs[key] = valToJSON(val)
}

func testSentTrace(t *testing.T, tr *traceWrapper, exp ...string) {
	testSent(t, tr.hcTrace.(*simulTrace).sent, exp...)
}

func testSent(t *testing.T, sent []map[string]string, exp ...string) {
	var sentStr []string
	for _, cl := range sent {
		var keys []string
		for k := range cl {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var s []string
		for _, k := range keys {
			s = append(s, fmt.Sprintf("%s:%s", k, strings.TrimSpace(cl[k])))
		}
		sentStr = append(sentStr, strings.Join(s, ";"))
	}
	sentNice := strings.Join(sentStr, "\n")
	require.Equal(t, len(exp), len(sent), sentNice)
	require.Equal(t, exp, sentStr, sentNice)
}
