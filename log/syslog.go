// +build freebsd linux darwin

package log

import "log/syslog"

type syslogLogger struct {
	lInfo  *LoggerInfo
	writer *syslog.Writer
}

func (sl *syslogLogger) Log(level int, msg string) {
	_, err := sl.writer.Write([]byte(msg))
	if err != nil {
		panic(err)
	}
}

func (sl *syslogLogger) Close() {
	sl.writer.Close()
}

func (sl *syslogLogger) GetLoggerInfo() *LoggerInfo {
	return sl.lInfo
}

// NewSyslogLogger creates a logger that writes into syslog with
// the given priority and tag, and is using the given LoggerInfo (without the
// Logger).
// It returns the logger.
func NewSyslogLogger(lInfo *LoggerInfo, priority syslog.Priority, tag string) (Logger, error) {
	writer, err := syslog.New(priority, tag)
	if err != nil {
		return nil, err
	}
	return &syslogLogger{
		lInfo:  lInfo,
		writer: writer,
	}, nil
}
