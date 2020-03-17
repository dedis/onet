package log

import (
	"fmt"
	"os"
	"strconv"
)

func lvlUI(l int, args ...interface{}) {
	if DebugVisible() <= 0 {
		print(l, args...)
	}
	if isVisible(l) {
		lvl(l, 3, args...)
	}
}

// Info prints the arguments given with a 'info'-format
func Info(args ...interface{}) {
	lvlUI(lvlInfo, args...)
}

// Print directly sends the arguments to the stdout
func Print(args ...interface{}) {
	lvlUI(lvlPrint, args...)
}

// Warn prints out the warning message and quits
func Warn(args ...interface{}) {
	lvlUI(lvlWarning, args...)
}

// Error prints out the error message. If the
// argument is an error, it will print it using "%+v".
func Error(args ...interface{}) {
	last := len(args) - 1
	if last >= 0 {
		err, ok := args[last].(error)
		if ok {
			args[last] = fmt.Sprintf("%+v", err)
		}
	}

	lvlUI(lvlError, args...)
}

// Panic prints out the panic message and panics
func Panic(args ...interface{}) {
	lvlUI(lvlPanic, args...)
	panic(fmt.Sprint(args...))
}

// Fatal prints out the fatal message and quits with os.Exit(1)
func Fatal(args ...interface{}) {
	lvlUI(lvlFatal, args...)
	os.Exit(1)
}

// Infof takes a format-string and calls Info
func Infof(f string, args ...interface{}) {
	lvlUI(lvlInfo, fmt.Sprintf(f, args...))
}

// Printf is like Print but takes a formatting-argument first
func Printf(f string, args ...interface{}) {
	lvlUI(lvlPrint, fmt.Sprintf(f, args...))
}

// Warnf is like Warn but with a format-string
func Warnf(f string, args ...interface{}) {
	lvlUI(lvlWarning, fmt.Sprintf(f, args...))
}

// Errorf is like Error but with a format-string
func Errorf(f string, args ...interface{}) {
	lvlUI(lvlError, fmt.Sprintf(f, args...))
}

// Panicf is like Panic but with a format-string
func Panicf(f string, args ...interface{}) {
	lvlUI(lvlWarning, fmt.Sprintf(f, args...))
	panic(args)
}

// Fatalf is like Fatal but with a format-string
func Fatalf(f string, args ...interface{}) {
	lvlUI(lvlFatal, fmt.Sprintf(f, args...))
	os.Exit(-1)
}

// ErrFatal calls log.Fatal in the case err != nil
func ErrFatal(err error, args ...interface{}) {
	if err != nil {
		lvlUI(lvlFatal, fmt.Sprint(args...)+" "+err.Error())
		os.Exit(1)
	}
}

// ErrFatalf will call Fatalf when the error is non-nil
func ErrFatalf(err error, f string, args ...interface{}) {
	if err != nil {
		lvlUI(lvlFatal, fmt.Sprintf(f+" ", args...)+err.Error())
		os.Exit(1)
	}
}

// TraceID helps the tracing-simulation to know which trace it should attach
// to.
// The TraceID will be stored with the corresponding go-routine,
// and any new go-routines spawned from the method will be attached to the
// same TraceID.
func TraceID(id []byte) {
	for _, l := range loggers {
		if t, ok := l.(Tracer); ok {
			t.TraceID(id)
		}
	}
}

func print(lvl int, args ...interface{}) {
	debugMut.Lock()
	defer debugMut.Unlock()
	out := stdOut
	if lvl < lvlInfo {
		out = stdErr
	}
	switch loggers[0].GetLoggerInfo().DebugLvl {
	case FormatPython:
		prefix := []string{"[-]", "[!]", "[X]", "[Q]", "[+]", ""}
		ind := lvl - lvlWarning
		if ind < 0 || ind > 4 {
			panic("index out of range " + strconv.Itoa(ind))
		}
		fmt.Fprint(out, prefix[ind], " ")
	case FormatNone:
	}
	for i, a := range args {
		fmt.Fprint(out, a)
		if i != len(args)-1 {
			fmt.Fprint(out, " ")
		}
	}
	fmt.Fprint(out, "\n")
}
