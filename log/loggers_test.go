package log

import (
    "io/ioutil"
    "os"
    "testing"

    "github.com/stretchr/testify/require"
)

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