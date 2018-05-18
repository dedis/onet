package log

import (
	"errors"
	"fmt"
	"log/syslog"
	"os"

	"github.com/daviddengcn/go-colortext"
)

// LoggerInfo is a structure that should be used when creating a logger.
// It contains parameters about how to log (with time, colors, ...) and
// embeds the Logger interface, which should define how the logger should log.
type LoggerInfo struct {
	// These are information-debugging levels that can be turned on or off.
	// Every logging greater than 'debugLvl' will be discarded . So you can
	// Log at different levels and easily turn on or off the amount of logging
	// generated by adjusting the 'debugLvl' variable.
	debugLvl int
	// If 'showTime' is true, it will print the time for each line displayed
	// by the logger.
	showTime bool
	// If 'useColors' is true, logs will be colored (defaults to monochrome
	// output). It also controls padding, since colorful output is higly
	// correlated with humans who like their log lines padded.
	useColors bool
	// If 'padding' is true, it will nicely pad the line that is written.
	padding bool
	// When using LoggerInfo, we need to be able to act as a Logger.
	Logger
}

// Logger is the interface that specifies how loggers
// will receive and display messages.
type Logger interface {
	Log(level int, msg string, l *LoggerInfo)
	Close()
}

const (
	// The default debug level for the standard logger
	DefaultStdDebugLvl  = 1
	// The default value for 'showTime' for the standard logger
	DefaultStdShowTime  = false
	// The default value for 'useColors' for the standard logger
	DefaultStdUseColors = false
	// The default value for 'padding' for the standard logger
	DefaultStdPadding   = true
)

var (
	// concurrent access is protected by debugMut
	loggers        = make(map[int]*LoggerInfo)
	loggersCounter int
)

// RegisterLogger will register a callback that will receive a copy of every
// message, fully formatted. It returns the key assigned to the logger (used
// to unregister the logger).
func RegisterLogger(l *LoggerInfo) int {
	debugMut.Lock()
	defer debugMut.Unlock()
	key := loggersCounter
	loggers[key] = l
	loggersCounter++
	return key
}

// UnregisterLogger takes the key it was assigned and returned by
// 'RegisterLogger', closes the corresponding Logger and removes it from the
// loggers.
func UnregisterLogger(key int) {
	debugMut.Lock()
	defer debugMut.Unlock()
	if l, ok := loggers[key]; ok {
		l.Close()
		delete(loggers, key)
	}
}

type fileLogger struct {
	file *os.File
}

func (fl *fileLogger) Log(level int, msg string, l *LoggerInfo) {
	if _, err := fl.file.WriteString(msg); err != nil {
		panic(err)
	}
}

func (fl *fileLogger) Close() {
	fl.file.Close()
}

// NewFileLogger creates and registers a logger that writes into the file with
// the given path and is using the given LoggerInfo (without the Logger).
// It returns the key assigned to the logger.
func NewFileLogger(path string, lInfo *LoggerInfo) (int, error) {
	// Override file if it already exists.
	file, err := os.Create(path)
	if err != nil {
		return -1, err
	}
	lInfo.Logger = &fileLogger{file: file}
	return RegisterLogger(lInfo), nil
}

type syslogLogger struct {
	writer *syslog.Writer
}

func (sl *syslogLogger) Log(level int, msg string, l *LoggerInfo) {
	_, err := sl.writer.Write([]byte(msg))
	if err != nil {
		panic(err)
	}
}

func (sl *syslogLogger) Close() {
	sl.writer.Close()
}

// NewSyslogLogger creates and registers a logger that writes into syslog with
// the given priority and tag, and is using the given LoggerInfo (without the
// Logger).
// It returns the key assigned to the logger.
func NewSyslogLogger(priority syslog.Priority, tag string, lInfo *LoggerInfo) (int, error) {
	writer, err := syslog.New(priority, tag)
	if err != nil {
		return -1, err
	}
	lInfo.Logger = &syslogLogger{writer: writer}
	return RegisterLogger(lInfo), nil
}

type stdLogger struct{}

func (sl *stdLogger) Log(lvl int, msg string, l *LoggerInfo) {
	bright := lvl < 0
	lvlAbs := lvl
	if bright {
		lvlAbs *= -1
	}

	switch lvl {
	case lvlPrint:
		fg(l, ct.White, true)
	case lvlInfo:
		fg(l, ct.White, true)
	case lvlWarning:
		fg(l, ct.Green, true)
	case lvlError:
		fg(l, ct.Red, false)
	case lvlFatal:
		fg(l, ct.Red, true)
	case lvlPanic:
		fg(l, ct.Red, true)
	default:
		if lvl != 0 {
			if lvlAbs <= 5 {
				colors := []ct.Color{ct.Yellow, ct.Cyan, ct.Green, ct.Blue, ct.Cyan}
				fg(l, colors[lvlAbs-1], bright)
			}
		}
	}

	if lvl < lvlInfo {
		fmt.Fprint(stdErr, msg)
	} else {
		fmt.Fprint(stdOut, msg)
	}

	if l.useColors {
		ct.ResetColor()
	}
}

func (sl *stdLogger) Close() {}

func newStdLogger() {
	lInfo := &LoggerInfo{
		debugLvl:  DefaultStdDebugLvl,
		useColors: DefaultStdUseColors,
		showTime:  DefaultStdShowTime,
		padding:   DefaultStdPadding,
		Logger:    &stdLogger{},
	}
	stdKey := RegisterLogger(lInfo)
	if stdKey != 0 {
		panic(errors.New("Cannot add a logger before the standard logger"))
	}
}
