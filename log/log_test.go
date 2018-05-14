package log

import (
	//"io/ioutil"
	"log/syslog"
	//"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContains(t *testing.T) {
	// Flush buffers from previous tests.
	GetStdOut()
	GetStdErr()
	old := DebugVisible()
	SetDebugVisible(1)

	assert.False(t, containsStdOut("info"))
	assert.False(t, containsStdErr("error"))
	Info("Some information")
	assert.True(t, containsStdOut("info"))
	Error("Some error")
	assert.True(t, containsStdErr("error"))

	SetDebugVisible(old)
}

// containsStdErr will look for str in StdErr and flush the output-buffer.
// If you need to look at multiple strings, use GetStdErr.
func containsStdErr(str string) bool {
	return strings.Contains(GetStdErr(), str)
}

// containsStdOut will look for str in StdOut and flush the output-buffer.
// If you need to look at multiple strings, use GetStdOut.
func containsStdOut(str string) bool {
	return strings.Contains(GetStdOut(), str)
}

type countMsgs int

func (c *countMsgs) Log(lvl int, msg string) {
	*c++
}

func TestListener(t *testing.T) {
	var c countMsgs
	RegisterListener(&c)
	SetDebugVisible(2)
	Lvl1("testing")
	Lvl5("testing")
	if c != 2 {
		t.Fatal("wrong count")
	}
}

func TestFileLogger(t *testing.T) {
	tempFile, err := ioutil.TempFile("", "test_file_logger.txt")
	require.Nil(t, err)
	path := "/Users/valentin/go/src/github.com/dedis/onet/log/test_val.log"
	require.Nil(t, NewFileLogger(path))
	/*
	defer func() {
		err := os.Remove(path)
		require.Nil(t, err)
	}()
	*/

	SetDebugVisible(2)
	Lvl1("testing1")
	SetShowTime(true)
	Lvl5("testing2")
	out, err := ioutil.ReadFile(path)
	require.Nil(t, err)
	require.Equal(t, "testing1\ntesting2\n", string(out))
}

func TestSyslogLogger(t *testing.T) {
	writer, err := NewSyslogLogger(syslog.LOG_NOTICE, "")
	require.Nil(t, err)
	writer.Close()
	/*
	defer func() {
		err := writer.Close()
		require.Nil(t, err)
	}()
	*/

	SetDebugVisible(2)
	Lvl1("testing1")
	Lvl5("testing2")
}
