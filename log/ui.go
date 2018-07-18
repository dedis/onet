package log

import (
	"fmt"
	"os"
	"strconv"
	"runtime"
	"time"
	"io"
	"sync"
)

const (
	Ldate  = 1 << iota
	Ltime
	Lmicroseconds
	Llongfile
	Lshortfile
	LUTC
	LstdFlags = Ldate | Ltime
)

type LoggerPanic struct {
	mu     sync.Mutex
	prefix string
	flag   int
	out    io.Writer
	buf    []byte
}

// New creates a new Logger
func New(out io.Writer, prefix string, flag int) *LoggerPanic {
        return &LoggerPanic{out: out, prefix: prefix, flag: flag}
}

var std = New(os.Stderr, "", LstdFlags)

// integer to fixed-width decimal ASCII. Give a negative width to avoid zero-padding.
func itoa(buf *[]byte, i int, wid int) {
        // Assemble decimal in reverse order.
        var b [20]byte
        bp := len(b) - 1
        for i >= 10 || wid > 1 {
                wid--
                q := i / 10
                b[bp] = byte('0' + i - q*10)
                bp--
                i = q
        }
        // i < 10
        b[bp] = byte('0' + i)
        *buf = append(*buf, b[bp:]...)
}

// formatHeader writes log header to buf.
func (l *LoggerPanic) formatHeader(buf *[]byte, t time.Time, file string, line int) {
	*buf = append(*buf, l.prefix...)
	if l.flag&(Ldate|Ltime|Lmicroseconds) != 0 {
		if l.flag&LUTC != 0 {
			t = t.UTC()
		}
		if l.flag&Ldate != 0 {
			year, month, day := t.Date()
			itoa(buf, year, 4)
			*buf = append(*buf, '/')
			itoa(buf, int(month), 2)
			*buf = append(*buf, '/')
			itoa(buf, day, 2)
			*buf = append(*buf, ' ')
		}
		if l.flag&(Ltime|Lmicroseconds) != 0 {
			hour, min, sec := t.Clock()
			itoa(buf, hour, 2)
			*buf = append(*buf, ':')
			itoa(buf, min, 2)
			*buf = append(*buf, ':')
			itoa(buf, sec, 2)
			if l.flag&Lmicroseconds != 0 {
				*buf = append(*buf, '.')
				itoa(buf, t.Nanosecond()/1e3, 6)
			}
			*buf = append(*buf, ' ')
		}
	}
	if l.flag&(Lshortfile|Llongfile) != 0 {
		if l.flag&Lshortfile != 0 {
			short := file
			for i := len(file) - 1; i > 0; i-- {
				if file[i] == '/' {
					short = file[i+1:]
					break
				}
			}
			file = short
		}
		*buf = append(*buf, file...)
		*buf = append(*buf, ':')
		itoa(buf, line, -1)
		*buf = append(*buf, ": "...)
	}
}

// Ouput writes the logging event.
func (l *LoggerPanic) Output(calldepth int, s string) error {
	now := time.Now() // get this early.
	var file string
	var line int
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.flag&(Lshortfile|Llongfile) != 0 {
		// Release lock while getting caller info - it's expensive.
		l.mu.Unlock()
		var ok bool
		_, file, line, ok = runtime.Caller(calldepth)
		if !ok {
			file = "???"
			line = 0
		}
		l.mu.Lock()
	}
	l.buf = l.buf[:0]
	l.formatHeader(&l.buf, now, file, line)
	l.buf = append(l.buf, s...)
	if len(s) == 0 || s[len(s)-1] != '\n' {
		l.buf = append(l.buf, '\n')
	}
	_, err := l.out.Write(l.buf)
	return err
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
	s := fmt.Sprint(args...)
	std.Output(2,s)
	panic(s)
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
	switch loggers[0].GetLoggerInfo().DebugLvl {
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
