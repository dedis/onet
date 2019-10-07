package log

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

type countMsgs struct {
	count int
	lInfo *LoggerInfo
}

func (c *countMsgs) Log(lvl int, msg string) {
	c.count++
}

func (c *countMsgs) Close() {}

func (c *countMsgs) GetLoggerInfo() *LoggerInfo {
	return c.lInfo
}

func TestRegisterLogger(t *testing.T) {
	lInfo := &LoggerInfo{
		DebugLvl:  3,
		UseColors: false,
		ShowTime:  false,
		Padding:   false,
	}
	c := &countMsgs{
		count: 0,
		lInfo: lInfo,
	}
	key := RegisterLogger(c)
	defer UnregisterLogger(key)
	Lvl1("testing")
	Lvl3("testing")
	Lvl5("testing")
	if c.count != 2 {
		t.Fatal("wrong count")
	}
}

func TestUnregisterLogger(t *testing.T) {
	lInfo := &LoggerInfo{
		DebugLvl:  3,
		UseColors: false,
		ShowTime:  false,
		Padding:   false,
	}
	c := &countMsgs{
		count: 0,
		lInfo: lInfo,
	}
	key := RegisterLogger(c)
	Lvl1("testing")
	UnregisterLogger(key)
	Lvl1("testing")
	if c.count != 1 {
		t.Fatal("wrong count")
	}
}

func TestFileLogger(t *testing.T) {
	tempFile, err := ioutil.TempFile("", "test_file_logger.txt")
	require.Nil(t, err)
	path := tempFile.Name()
	lInfo := &LoggerInfo{
		DebugLvl:  2,
		ShowTime:  false,
		UseColors: false,
		Padding:   false,
	}
	fileLogger, err := NewFileLogger(lInfo, path)
	require.Nil(t, err)
	key := RegisterLogger(fileLogger)
	defer func() {
		UnregisterLogger(key)
		err := os.Remove(path)
		require.Nil(t, err)
	}()

	Lvl1("testing1")
	Lvl2("testing2")
	out, err := ioutil.ReadFile(path)
	require.Nil(t, err)
	require.Equal(t, "1 : fake_name.go:0 - testing1\n"+
		"2 : fake_name.go:0 - testing2\n", string(out))
}
