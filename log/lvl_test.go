package log

import (
	"flag"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

func init() {
	outputLines = false
	SetUseColors(false)
	clearEnv()
}

func TestTime(t *testing.T) {
	SetShowTime(false)
	SetDebugVisible(1)
	GetStdOut()
	Lvl1("No time")
	assert.True(t, containsStdOut("1 : "))
	SetShowTime(true)
	defer func() { SetShowTime(false) }()
	Lvl1("With time")
	str := GetStdOut()
	if strings.Contains(str, "1 : ") {
		t.Fatal("Didn't get correct string: ", str)
	}
	if strings.Contains(str, " +") {
		t.Fatal("Didn't get correct string: ", str)
	}
	if !strings.Contains(str, "With time") {
		t.Fatal("Didn't get correct string: ", str)
	}
}

func TestFlags(t *testing.T) {
	lvl := DebugVisible()
	time := ShowTime()
	color := UseColors()
	padding := Padding()
	SetDebugVisible(1)

	clearEnv()
	ParseEnv()
	if DebugVisible() != 1 {
		t.Fatal("Debugvisible should be 1")
	}
	if ShowTime() {
		t.Fatal("ShowTime should be false")
	}
	if UseColors() {
		t.Fatal("UseColors should be false")
	}
	if !Padding() {
		t.Fatal("Padding should be true")
	}

	os.Setenv("DEBUG_LVL", "3")
	os.Setenv("DEBUG_TIME", "true")
	os.Setenv("DEBUG_COLOR", "false")
	os.Setenv("DEBUG_PADDING", "false")
	ParseEnv()
	if DebugVisible() != 3 {
		t.Fatal("DebugVisible should be 3")
	}
	if !ShowTime() {
		t.Fatal("ShowTime should be true")
	}
	if UseColors() {
		t.Fatal("UseColors should be false")
	}
	if Padding() {
		t.Fatal("Padding should be false")
	}

	clearEnv()
	SetDebugVisible(lvl)
	SetShowTime(time)
	SetUseColors(color)
	SetPadding(padding)
}

func TestOutputFuncs(t *testing.T) {
	ErrFatal(checkOutput(func() {
		Lvl1("Testing stdout")
	}, true, false))
	ErrFatal(checkOutput(func() {
		LLvl1("Testing stdout")
	}, true, false))
	ErrFatal(checkOutput(func() {
		Print("Testing stdout")
	}, true, false))
	ErrFatal(checkOutput(func() {
		Warn("Testing stdout")
	}, false, true))
	ErrFatal(checkOutput(func() {
		Error("Testing errout")
	}, false, true))
}

func TestMainTestWait(t *testing.T) {
	toOld := flag.Lookup("test.timeout").Value.String()
	lvlOld := DebugVisible()
	defer func() {
		setFlag(toOld)
		SetDebugVisible(lvlOld)
	}()
	SetDebugVisible(1)
	setFlag("0s")
	require.Equal(t, time.Duration(10*time.Minute), interpretWait())
	setFlag("10s")
	require.Equal(t, time.Duration(10*time.Second), interpretWait())
	require.Equal(t, "", GetStdOut())

	MainTestWait = 20 * time.Second
	setFlag("0s")
	require.Equal(t, time.Duration(20*time.Second), interpretWait())
	require.NotEqual(t, "", GetStdErr())
	setFlag("10s")
	require.Equal(t, time.Duration(10*time.Second), interpretWait())
	require.NotEqual(t, "", GetStdErr())
}

func setFlag(t string) {
	timeoutFlagMutex.Lock()
	flag.Lookup("test.timeout").Value.Set(t)
	timeoutFlagMutex.Unlock()
}

func checkOutput(f func(), wantsStd, wantsErr bool) error {
	f()
	stdStr := GetStdOut()
	errStr := GetStdErr()
	if wantsStd {
		if len(stdStr) == 0 {
			return xerrors.New("Stdout was empty")
		}
	} else {
		if len(stdStr) > 0 {
			return xerrors.New("Stdout was full")
		}
	}
	if wantsErr {
		if len(errStr) == 0 {
			return xerrors.New("Stderr was empty")
		}
	} else {
		if len(errStr) > 0 {
			return xerrors.New("Stderr was full")
		}
	}
	return nil
}

func ExampleLvl2() {
	SetDebugVisible(2)
	OutputToOs()
	Lvl1("Level1")
	Lvl2("Level2")
	Lvl3("Level3")
	Lvl4("Level4")
	Lvl5("Level5")
	OutputToBuf()
	SetDebugVisible(1)

	// Output:
	// 1 : fake_name.go:0 (log.ExampleLvl2)         - Level1
	// 2 : fake_name.go:0 (log.ExampleLvl2)         - Level2
}

func ExampleLvl1() {
	OutputToOs()
	Lvl1("Multiple", "parameters")
	OutputToBuf()

	// Output:
	// 1 : fake_name.go:0 (log.ExampleLvl1)         - Multiple parameters
}

func ExampleLLvl1() {
	OutputToOs()
	Lvl1("Lvl output")
	LLvl1("LLvl output")
	Lvlf1("Lvlf output")
	LLvlf1("LLvlf output")
	OutputToBuf()

	// Output:
	// 1 : fake_name.go:0 (log.ExampleLLvl1)        - Lvl output
	// 1!: fake_name.go:0 (log.ExampleLLvl1)        - LLvl output
	// 1 : fake_name.go:0 (log.ExampleLLvl1)        - Lvlf output
	// 1!: fake_name.go:0 (log.ExampleLLvl1)        - LLvlf output
}

func thisIsAVeryLongFunctionNameThatWillOverflow() {
	OutputToOs()
	Lvl1("Overflow")
}

func ExampleLvlf1() {
	OutputToOs()
	Lvl1("Before")
	thisIsAVeryLongFunctionNameThatWillOverflow()
	Lvl1("After")
	OutputToBuf()

	// Output:
	// 1 : fake_name.go:0 (log.ExampleLvlf1)        - Before
	// 1 : fake_name.go:0 (log.thisIsAVeryLongFunctionNameThatWillOverflow) - Overflow
	// 1 : fake_name.go:0 (log.ExampleLvlf1)        - After
}

func ExampleLvl3() {
	NamePadding = -1
	OutputToOs()
	Lvl1("Before")
	thisIsAVeryLongFunctionNameThatWillOverflow()
	Lvl1("After")
	OutputToBuf()

	// Output:
	// 1 : fake_name.go:0 (log.ExampleLvl3) - Before
	// 1 : fake_name.go:0 (log.thisIsAVeryLongFunctionNameThatWillOverflow) - Overflow
	// 1 : fake_name.go:0 (log.ExampleLvl3) - After
}

func clearEnv() {
	os.Setenv("DEBUG_LVL", "")
	os.Setenv("DEBUG_TIME", "")
	os.Setenv("DEBUG_COLOR", "")
	os.Setenv("DEBUG_PADDING", "")
}
