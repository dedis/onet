package log

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/pprof"
	"strconv"
	"sync"
	"testing"
	"time"

	"golang.org/x/xerrors"
)

const (
	lvlWarning = iota - 20
	lvlError
	lvlFatal
	lvlPanic
	lvlInfo
	lvlPrint
)

// These formats can be used in place of the debugVisible
const (
	// FormatPython uses [x] and others to indicate what is shown
	FormatPython = -1
	// FormatNone is just pure print
	FormatNone = 0
)

// defaultMainTest indicates what debug-level should be used when `go test -v`
// is called.
const defaultMainTest = 2

// MainTestWait is the maximum time the MainTest-method waits before aborting
// and printing a stack-trace of all functions.
// This is deprecated and should not be used anymore.
var MainTestWait = 0 * time.Minute

// NamePadding - the padding of functions to make a nice debug-output - this is automatically updated
// whenever there are longer functions and kept at that new maximum. If you prefer
// to have a fixed output and don't remember oversized names, put a negative value
// in here.
var NamePadding = 40

// LinePadding of line-numbers for a nice debug-output - used in the same way as
// NamePadding.
// Deprecated: the caller is merged thus no line padding is necessary anymore.
var LinePadding = 3

// StaticMsg - if this variable is set, it will be outputted between the
// position and the message.
var StaticMsg = ""

// outputLines can be false to suppress outputting of lines in tests.
var outputLines = true

var debugMut sync.RWMutex

var regexpPaths, _ = regexp.Compile(".*/")

func init() {
	stdLogger, err := newStdLogger()
	if err != nil {
		panic(err)
	}
	stdKey := RegisterLogger(stdLogger)
	if stdKey != 0 {
		panic(xerrors.New("Cannot add a logger before the standard logger"))
	}
	ParseEnv()
}

func lvl(lvl, skip int, args ...interface{}) {
	debugMut.Lock()
	defer debugMut.Unlock()
	for _, l := range loggers {
		// Get the *LoggerInfo that contains how the formatting should be done.
		lInfo := l.GetLoggerInfo()

		if lvl > lInfo.DebugLvl {
			continue
		}

		// fmt.Sprint doesn't use spaces if all args are string.
		// Only fmt.Sprintln adds spaces between args if all of them are
		// strings. So the trailing "\n" needs to be removed.
		message := fmt.Sprintln(args...)
		message = message[:len(message)-1]
		if lInfo.RawMessage {
			l.Log(lvl, message)
			continue
		}

		pc, fn, line, _ := runtime.Caller(skip)
		funcName := filepath.Base(runtime.FuncForPC(pc).Name())
		name := fn
		if !lInfo.AbsoluteFilePath {
			name = filepath.Base(fn)
		}

		// For the testing-framework, we check the resulting string. So as not to
		// have the tests fail every time somebody moves the functions, we put
		// the line-# to 0
		if !outputLines {
			line = 0
			name = "fake_name.go"
		}

		caller := fmt.Sprintf("%s:%d (%s)", name, line, funcName)

		if lInfo.UseColors {
			// Only adjust the name and line padding if we also have color.
			if len(caller) > NamePadding && NamePadding > 0 {
				NamePadding = len(caller)
			}
		}
		namePadding := 0
		if lInfo.Padding {
			namePadding = NamePadding
		}
		if StaticMsg != "" {
			caller += "@" + StaticMsg
		}

		lvlAbs := lvl
		if lvl < 0 {
			lvlAbs *= -1
		}
		lvlStr := strconv.Itoa(lvlAbs)
		if lvl < 0 {
			lvlStr += "!"
		}
		switch lvl {
		case lvlPrint:
			lvlStr = "I"
		case lvlInfo:
			lvlStr = "I"
		case lvlWarning:
			lvlStr = "W"
		case lvlError:
			lvlStr = "E"
		case lvlFatal:
			lvlStr = "F"
		case lvlPanic:
			lvlStr = "P"
		}
		fmtstr := fmt.Sprintf(": %%-%ds - %%s", namePadding)
		str := fmt.Sprintf(fmtstr, caller, message)
		if lInfo.ShowTime {
			ti := time.Now()
			str = fmt.Sprintf("%s.%09d%s", ti.Format("06/02/01 15:04:05"), ti.Nanosecond(), str)
		}
		str = fmt.Sprintf("%-2s%s", lvlStr, str)

		l.Log(lvl, str)
	}
}

// Needs two functions to keep the caller-depth the same and find who calls us
// Lvlf1 -> Lvlf -> lvl
// or
// Lvl1 -> lvld -> lvl
func lvlf(l int, f string, args ...interface{}) {
	if !isVisible(l) {
		return
	}
	lvl(l, 3, fmt.Sprintf(f, args...))
}
func lvld(l int, args ...interface{}) {
	lvl(l, 3, args...)
}

// Lvl1 debug output is informational and always displayed
func Lvl1(args ...interface{}) {
	lvld(1, args...)
}

// Lvl2 is more verbose but doesn't spam the stdout in case
// there is a big simulation
func Lvl2(args ...interface{}) {
	lvld(2, args...)
}

// Lvl3 gives debug-output that can make it difficult to read
// for big simulations with more than 100 hosts
func Lvl3(args ...interface{}) {
	lvld(3, args...)
}

// Lvl4 is only good for test-runs with very limited output
func Lvl4(args ...interface{}) {
	lvld(4, args...)
}

// Lvl5 is for big data
func Lvl5(args ...interface{}) {
	lvld(5, args...)
}

// Lvlf1 is like Lvl1 but with a format-string
func Lvlf1(f string, args ...interface{}) {
	lvlf(1, f, args...)
}

// Lvlf2 is like Lvl2 but with a format-string
func Lvlf2(f string, args ...interface{}) {
	lvlf(2, f, args...)
}

// Lvlf3 is like Lvl3 but with a format-string
func Lvlf3(f string, args ...interface{}) {
	lvlf(3, f, args...)
}

// Lvlf4 is like Lvl4 but with a format-string
func Lvlf4(f string, args ...interface{}) {
	lvlf(4, f, args...)
}

// Lvlf5 is like Lvl5 but with a format-string
func Lvlf5(f string, args ...interface{}) {
	lvlf(5, f, args...)
}

// LLvl1 *always* prints
func LLvl1(args ...interface{}) { lvld(-1, args...) }

// LLvl2 *always* prints
func LLvl2(args ...interface{}) { lvld(-2, args...) }

// LLvl3 *always* prints
func LLvl3(args ...interface{}) { lvld(-3, args...) }

// LLvl4 *always* prints
func LLvl4(args ...interface{}) { lvld(-4, args...) }

// LLvl5 *always* prints
func LLvl5(args ...interface{}) { lvld(-5, args...) }

// LLvlf1 *always* prints
func LLvlf1(f string, args ...interface{}) { lvlf(-1, f, args...) }

// LLvlf2 *always* prints
func LLvlf2(f string, args ...interface{}) { lvlf(-2, f, args...) }

// LLvlf3 *always* prints
func LLvlf3(f string, args ...interface{}) { lvlf(-3, f, args...) }

// LLvlf4 *always* prints
func LLvlf4(f string, args ...interface{}) { lvlf(-4, f, args...) }

// LLvlf5 *always* prints
func LLvlf5(f string, args ...interface{}) { lvlf(-5, f, args...) }

// TestOutput sets the DebugVisible to 0 if 'show'
// is false, else it will set DebugVisible to 'level'
//
// Usage: TestOutput( test.Verbose(), 2 )
// DEPRECATED: write your own if/else
func TestOutput(show bool, level int) {
	debugMut.Lock()
	defer debugMut.Unlock()

	if show {
		loggers[0].GetLoggerInfo().DebugLvl = level
	} else {
		loggers[0].GetLoggerInfo().DebugLvl = 0
	}
}

// SetDebugVisible set the global debug output level in a go-rountine-safe way
func SetDebugVisible(lvl int) {
	debugMut.Lock()
	defer debugMut.Unlock()
	loggers[0].GetLoggerInfo().DebugLvl = lvl
	loggers[0].GetLoggerInfo().RawMessage = lvl <= 0
}

// DebugVisible returns the actual visible debug-level
func DebugVisible() int {
	debugMut.RLock()
	defer debugMut.RUnlock()
	return loggers[0].GetLoggerInfo().DebugLvl
}

// isVisible returns true if at least one logger wants to use the message
func isVisible(lvl int) bool {
	debugMut.RLock()
	defer debugMut.RUnlock()
	for _, logger := range loggers {
		if lvl <= logger.GetLoggerInfo().DebugLvl {
			return true
		}
	}
	return false
}

// SetShowTime allows for turning on the flag that adds the current
// time to the debug-output
func SetShowTime(show bool) {
	debugMut.Lock()
	defer debugMut.Unlock()
	loggers[0].GetLoggerInfo().ShowTime = show
}

// ShowTime returns the current setting for showing the time in the debug
// output
func ShowTime() bool {
	debugMut.Lock()
	defer debugMut.Unlock()
	return loggers[0].GetLoggerInfo().ShowTime
}

// SetAbsoluteFilePath allows to print the absolute file path
// for logging calls.
func SetAbsoluteFilePath(absolute bool) {
	debugMut.Lock()
	defer debugMut.Unlock()
	loggers[0].GetLoggerInfo().AbsoluteFilePath = absolute
}

// AbsoluteFilePath returns the current setting for showing the absolute
// path inside a log.
func AbsoluteFilePath() bool {
	debugMut.Lock()
	defer debugMut.Unlock()
	return loggers[0].GetLoggerInfo().AbsoluteFilePath
}

// SetUseColors can turn off or turn on the use of colors in the debug-output
func SetUseColors(useColors bool) {
	debugMut.Lock()
	defer debugMut.Unlock()
	loggers[0].GetLoggerInfo().UseColors = useColors
}

// UseColors returns the actual setting of the color-usage in log
func UseColors() bool {
	debugMut.Lock()
	defer debugMut.Unlock()
	return loggers[0].GetLoggerInfo().UseColors
}

// SetPadding can turn off or turn on the use of padding in log
func SetPadding(padding bool) {
	debugMut.Lock()
	defer debugMut.Unlock()
	loggers[0].GetLoggerInfo().Padding = padding
}

// Padding returns the actual setting of the padding in log
func Padding() bool {
	debugMut.Lock()
	defer debugMut.Unlock()
	return loggers[0].GetLoggerInfo().Padding
}

// MainTest can be called from TestMain. It will parse the flags and
// set the DebugVisible to defaultMainTest, then run the tests and check for
// remaining go-routines.
// For the debug-level, the following heuristic is used to set the debug-level:
//   - the first ls argument given, else
//   - from the environment variable, else
//   - defaultMainTest (2)
// Also be aware that go only outputs the stdout of the tests if the verbose
// flag is given.
func MainTest(m *testing.M, ls ...int) {
	flag.Parse()
	if len(ls) > 0 {
		SetDebugVisible(ls[0])
	} else if os.Getenv("DEBUG_LVL") == "" {
		SetDebugVisible(defaultMainTest)
	}

	done := make(chan int)
	go func() {
		code := m.Run()
		done <- code
	}()
	select {
	case code := <-done:
		if code == 0 {
			AfterTest(nil)
		}
		os.Exit(code)
	case <-time.After(interpretWait()):
		Error("Didn't finish in time")
		pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
		os.Exit(1)
	}
}

// ParseEnv looks at the following environment-variables:
//   DEBUG_LVL - for the actual debug-lvl - default is 1
//   DEBUG_TIME - whether to show the timestamp - default is false
//   DEBUG_COLOR - whether to color the output - default is false
//   DEBUG_PADDING - whether to pad the output nicely - default is true
func ParseEnv() {
	dv := os.Getenv("DEBUG_LVL")
	if dv != "" {
		dvInt, err := strconv.Atoi(dv)
		Lvl3("Setting level to", dv, dvInt, err)
		SetDebugVisible(dvInt)
		if err != nil {
			Error("Couldn't convert", dv, "to debug-level")
		}
	}

	dt := os.Getenv("DEBUG_TIME")
	if dt != "" {
		dtInt, err := strconv.ParseBool(dt)
		Lvl3("Setting showTime to", dt, dtInt, err)
		SetShowTime(dtInt)
		if err != nil {
			Error("Couldn't convert", dt, "to boolean")
		}
	}

	dfp := os.Getenv("DEBUG_FILEPATH")
	if dfp != "" {
		dfpInt, err := strconv.ParseBool(dfp)
		Lvl3("Setting absoluteFilePath to", dfp, dfpInt, err)
		SetAbsoluteFilePath(dfpInt)
		if err != nil {
			Error("Couldn't convert", dfp, "to boolean")
		}
	}

	dc := os.Getenv("DEBUG_COLOR")
	if dc != "" {
		ucInt, err := strconv.ParseBool(dc)
		Lvl3("Setting useColor to", dc, ucInt, err)
		SetUseColors(ucInt)
		if err != nil {
			Error("Couldn't convert", dc, "to boolean")
		}
	}

	dp := os.Getenv("DEBUG_PADDING")
	if dp != "" {
		dpBool, err := strconv.ParseBool(dp)
		Lvl3("Setting padding to", dp, dpBool, err)
		SetPadding(dpBool)
		if err != nil {
			Error("Couldn't convert", dp, "to boolean")
		}
	}
}

// RegisterFlags adds the flags and the variables for the debug-control
// (standard logger) using the standard flag-package.
func RegisterFlags() {
	ParseEnv()
	defaultDebugLvl := DebugVisible()
	defaultShowTime := ShowTime()
	defaultUseColors := UseColors()
	defaultPadding := Padding()
	debugMut.Lock()
	defer debugMut.Unlock()
	flag.IntVar(&loggers[0].GetLoggerInfo().DebugLvl, "debug", defaultDebugLvl, "Change debug level (0-5)")
	flag.BoolVar(&loggers[0].GetLoggerInfo().ShowTime, "debug-time", defaultShowTime, "Shows the time of each message")
	flag.BoolVar(&loggers[0].GetLoggerInfo().UseColors, "debug-color", defaultUseColors, "Colors each message")
	flag.BoolVar(&loggers[0].GetLoggerInfo().Padding, "debug-padding", defaultPadding, "Pads each message nicely")
	flag.BoolVar(&loggers[0].GetLoggerInfo().AbsoluteFilePath, "debug-filepath", DefaultStdAbsoluteFilePath, "extends the filename with the absolute path")
}

var timeoutFlagMutex sync.Mutex

// interpretWait will use the test.timeout flag and MainTestWait to get
// the time to wait. From highest preference to lowest preference:
//  1. "-timeout 1m"
//  2. MainTestWait = 2*time.Minute
//  3. 10*time.Minute
// interpretWait will throw a warning if MainTestWait has been set.
func interpretWait() time.Duration {
	timeoutFlagMutex.Lock()
	defer timeoutFlagMutex.Unlock()
	toFlag := flag.Lookup("test.timeout")
	if toFlag == nil {
		Fatal("MainTest should not be called outside of tests")
	}
	dur := MainTestWait
	var err error
	if dur != 0*time.Second {
		Warn("Usage of MainTestWait is deprecated!")
	} else {
		dur = 10 * time.Minute
	}
	if toStr := toFlag.Value.String(); toStr != "0s" {
		dur, err = time.ParseDuration(toStr)
		ErrFatal(err, "couldn't parse passed timeout value")
	}
	return dur
}
