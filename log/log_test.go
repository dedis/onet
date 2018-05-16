package log

import (
	"io/ioutil"
	"os"
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

func (c *countMsgs) Close() {}

func TestRegisterLogger(t *testing.T) {
	var c countMsgs
	lInfo := &LoggerInfo{
		debugLvl:  3,
		useColors: false,
		showTime:  false,
		Logger:    &c,
	}
	key := RegisterLogger(lInfo)
	defer UnregisterLogger(key)
	Lvl1("testing")
	Lvl3("testing")
	Lvl5("testing")
	if c != 2 {
		t.Fatal("wrong count")
	}
}

func TestUnregisterLogger(t *testing.T) {
	var c countMsgs
	lInfo := &LoggerInfo{
		debugLvl:  3,
		useColors: false,
		showTime:  false,
		Logger:    &c,
	}
	key := RegisterLogger(lInfo)
	Lvl1("testing")
	UnregisterLogger(key)
	Lvl1("testing")
	if c != 1 {
		t.Fatal("wrong count")
	}
}

func TestFileLogger(t *testing.T) {
	tempFile, err := ioutil.TempFile("", "test_file_logger.txt")
	require.Nil(t, err)
	path := tempFile.Name()
	lInfo := &LoggerInfo{
		debugLvl:  2,
		showTime:  false,
		useColors: false,
	}
	key, err := NewFileLogger(path, lInfo)
	require.Nil(t, err)
	defer func() {
		UnregisterLogger(key)
		err := os.Remove(path)
		require.Nil(t, err)
	}()

	Lvl1("testing1")
	Lvl2("testing2")
	out, err := ioutil.ReadFile(path)
	require.Nil(t, err)
	require.Equal(t, "1 : (                      log.TestFileLogger:   0) - testing1\n"+
		"2 : (                      log.TestFileLogger:   0) - testing2\n", string(out))
}
