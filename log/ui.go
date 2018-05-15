package log

import (
	"fmt"
	"os"
	"strconv"
)


type ListenerInfo struct {
	lvl int
	useColors bool
	showTime bool
	Listener
}

// Listener is the interface that specifies how log listeners
// will receive messages.
type Listener interface {
	Log(level int, msg string)
	Close()
}

var (
	// concurrent access is protected by debugMut
	listeners map[int]*ListenerInfo = make(map[int]*ListenerInfo)
	listenersCounter int
)

// RegisterListener will register a callback that will receive
// a copy of every message, fully formatted but without a trailing
// newline.
func RegisterListener(l *ListenerInfo) int {
	debugMut.Lock()
	defer debugMut.Unlock()
	key := listenersCounter
	listeners[key] = l
	listenersCounter += 1
	return key
}

func UnregisterListener(key int) {
	debugMut.Lock()
	defer debugMut.Unlock()
	if l, ok := listeners[key]; ok {
		l.Close()
		delete(listeners, key)
	}
}

func lvlUI(l int, args ...interface{}) {
	if DebugVisible() > 0 {
		lvl(l, 3, args...)
	} else {
		print(l, args...)
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

// Error prints out the error message and quits
func Error(args ...interface{}) {
	lvlUI(lvlError, args...)
}

// Panic prints out the panic message and panics
func Panic(args ...interface{}) {
	lvlUI(lvlPanic, args...)
	panic(args)
}

// Fatal prints out the fatal message and quits
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
		lvlUI(lvlFatal, err.Error()+" "+fmt.Sprint(args...))
		os.Exit(1)
	}
}

// ErrFatalf will call Fatalf when the error is non-nil
func ErrFatalf(err error, f string, args ...interface{}) {
	if err != nil {
		lvlUI(lvlFatal, err.Error()+fmt.Sprintf(" "+f, args...))
		os.Exit(1)
	}
}

func print(lvl int, args ...interface{}) {
	debugMut.Lock()
	defer debugMut.Unlock()
	switch listeners[0].lvl {
	case FormatPython:
		prefix := []string{"[-]", "[!]", "[X]", "[Q]", "[+]", ""}
		ind := lvl - lvlWarning
		if ind < 0 || ind > 4 {
			panic("index out of range " + strconv.Itoa(ind))
		}
		fmt.Fprint(stdOut, prefix[ind], " ")
	case FormatNone:
	}
	for i, a := range args {
		fmt.Fprint(stdOut, a)
		if i != len(args)-1 {
			fmt.Fprint(stdOut, " ")
		}
	}
	fmt.Fprint(stdOut, "\n")
}
