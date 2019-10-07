package log

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/xerrors"
)

func TestMain(m *testing.M) {
	OutputToBuf()
	MainTest(m)
}

func TestInfo(t *testing.T) {
	SetDebugVisible(FormatPython)
	Info("Python")
	assert.True(t, containsStdOut("[+] Python\n"))
	SetDebugVisible(FormatNone)
	Info("None")
	assert.True(t, containsStdOut("None\n"))
	Info("None", "Python")
	assert.True(t, containsStdOut("None Python\n"))
	SetDebugVisible(1)
}

func TestError(t *testing.T) {
	SetDebugVisible(1)
	Error(xerrors.New("error with stack"))
	assert.Contains(t, GetStdErr(), "/log/ui_test.go:")

	Error(xerrors.New("test"), "and another message")
	assert.NotContains(t, GetStdErr(), "/log/ui_test.go:")
}

func TestLvl(t *testing.T) {
	SetDebugVisible(1)
	Info("TestLvl")
	assert.Contains(t, GetStdOut(), "I :                             fake_name.go:0 - TestLvl\n")
	Print("TestLvl")
	assert.Contains(t, GetStdOut(), "I :                             fake_name.go:0 - TestLvl\n")
	Warn("TestLvl")
	assert.Contains(t, GetStdErr(), "W :                             fake_name.go:0 - TestLvl\n")
}

func TestPanic(t *testing.T) {
	assert.PanicsWithValue(t, "", func() {
		Panic()
	})
	assert.PanicsWithValue(t, "the number is 1", func() {
		Panic("the number is ", 1)
	})
}
