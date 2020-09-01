package log

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"testing"
	"time"
)

func interestingGoroutines() (gs []string) {
	buf := make([]byte, 2<<20)
	buf = buf[:runtime.Stack(buf, true)]
	for _, g := range strings.Split(string(buf), "\n\n") {
		sl := strings.SplitN(g, "\n", 2)
		if len(sl) != 2 {
			continue
		}
		stack := strings.TrimSpace(sl[1])
		if stack == "" ||
			strings.Contains(stack, "(*LocalTest).CloseAll") ||
			strings.Contains(stack, "created by testing.RunTests") ||
			strings.Contains(stack, "testing.RunTests(") ||
			strings.Contains(stack, "testing.(*T).Run(") ||
			strings.Contains(stack, "testing.Main(") ||
			strings.Contains(stack, "runtime.goexit") ||
			strings.Contains(stack, "interestingGoroutines") ||
			strings.Contains(stack, "created by runtime.gc") ||
			strings.Contains(stack, "runtime.MHeap_Scavenger") ||
			strings.Contains(stack, "sigqueue") ||
			strings.Contains(stack, "log.MainTest") ||
			matchesUserUninterestingGoroutine(stack) {
			continue
		}

		gs = append(gs, stack)
	}
	sort.Strings(gs)
	return
}

var userUninterestingGoroutines []string

// AddUserUninterestingGoroutine can be called when the environment of some
// specific tests leaks goroutines unknown to interestingGoroutines().
// This function is not safe for concurrent execution. The caller should add
// all of the desired exceptions before launching any goroutines.
func AddUserUninterestingGoroutine(newGr string) {
	userUninterestingGoroutines = append(userUninterestingGoroutines, newGr)
}

func matchesUserUninterestingGoroutine(stack string) bool {
	for _, gr := range userUninterestingGoroutines {
		if strings.Contains(stack, gr) {
			return true
		}
	}

	return false
}

// AfterTest can be called to wait for leaking goroutines to finish. If
// they do not finish after a reasonable time (600ms) the test will fail.
//
// Inspired by https://golang.org/src/net/http/main_test.go
// and https://github.com/coreos/etcd/blob/master/pkg/testutil/leak.go
func AfterTest(t *testing.T) {
	var stackCount map[string]int
	for i := 0; i < 10; i++ {
		n := 0
		stackCount = make(map[string]int)
		gs := interestingGoroutines()
		for _, g := range gs {
			stackCount[g]++
			n++
		}
		if n == 0 {
			break
		}
		// Wait for goroutines to schedule and die off:
		time.Sleep(100 * time.Millisecond)
	}
	if len(stackCount) > 0 {
		for stack, count := range stackCount {
			if t != nil {
				t.Logf("%d instances of:\n%s\n", count, stack)
			} else {
				Error(fmt.Sprintf("%d instances of:\n%s\n", count, stack))
			}
		}
		Print("Stack-trace of caller: ", Stack())
		if t != nil {
			t.Fatalf("Test leaks %d goroutines.", len(stackCount))
		} else {
			Fatal(fmt.Sprintf("Test leaks %d goroutines.", len(stackCount)))
		}
	}
}

// Stack converts []byte to string
func Stack() string {
	return string(debug.Stack())
}
