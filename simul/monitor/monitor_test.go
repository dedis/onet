package monitor

import (
	"bytes"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/dedis/onet/log"
)

func TestMain(m *testing.M) {
	log.MainTest(m)
}

func TestReadyNormal(t *testing.T) {
	m := make(map[string]string)
	m["servers"] = "1"
	stat := NewStats(m)
	fresh := stat.String()
	// First set up monitor listening
	mon := NewMonitor(stat)
	go mon.Listen()
	time.Sleep(100 * time.Millisecond)

	// Then measure
	err := ConnectSink("localhost:" + strconv.Itoa(DefaultSinkPort))
	if err != nil {
		t.Fatal(fmt.Sprintf("Error starting monitor: %s", err))
		return
	}

	meas := newSingleMeasure("round", 10)
	meas.Record()
	time.Sleep(200 * time.Millisecond)
	newSingleMeasure("round", 20)
	EndAndCleanup()
	time.Sleep(100 * time.Millisecond)
	updated := mon.stats.String()
	if updated == fresh {
		t.Fatal("Stats not updated ?")
	}
}

func TestKeyOrder(t *testing.T) {
	m := make(map[string]string)
	m["servers"] = "1"
	m["hosts"] = "1"
	m["bf"] = "2"
	// create stats
	stat := NewStats(m)
	m1 := newSingleMeasure("round", 10)
	m2 := newSingleMeasure("setup", 5)
	stat.Update(m1)
	stat.Update(m2)
	str := new(bytes.Buffer)
	stat.WriteHeader(str)
	stat.WriteValues(str)

	stat2 := NewStats(m)
	stat2.Update(m1)
	stat2.Update(m2)

	str2 := new(bytes.Buffer)
	stat2.WriteHeader(str2)
	stat2.WriteValues(str2)
	if !bytes.Equal(str.Bytes(), str2.Bytes()) {
		t.Fatal("KeyOrder / output not the same for same stats")
	}
}

// setupMonitor launches a basic monitor with a created Stats object
// When finished with the monitor, just call `End()`
func setupMonitor(t *testing.T) (*Monitor, *Stats) {
	m := make(map[string]string)
	m["servers"] = "1"
	stat := NewStats(m)
	// First set up monitor listening
	mon := NewMonitor(stat)
	go mon.Listen()
	time.Sleep(100 * time.Millisecond)

	// Then measure
	err := ConnectSink("localhost:" + strconv.Itoa(int(mon.SinkPort)))
	if err != nil {
		t.Fatal(fmt.Sprintf("Error starting monitor: %s", err))
	}
	return mon, stat
}

type testStringInput map[string]string // for testing string input  in setupMonitor

//type Pair struct { // this holds pairs of out put mon and stats below
//	m *Monitor

//	s *Stats
//}

// setupMonitorStringTest launches a basic monitor with a created Stats object
// When finished with the monitor, just call `End()`
// It mimics the setMonitor above but with interface as ouput. This enables testing multiple
// string as input
func setupMonitorStringTest(t *testing.T, testCount int) (*Monitor, *Stats) {

	var myMapSlice []testStringInput // for storing an array for maps. To be use for testing

	m1 := testStringInput{"server1": "crazyStrings"}
	m2 := testStringInput{"server2": ""}
	m3 := testStringInput{"server3": "123456789"}

	myMapSlice = append(myMapSlice, m1, m2, m3)

	if testCount > len(myMapSlice) {

		testCount = len(myMapSlice) - 1

	}

	m := myMapSlice[testCount]

	stat := NewStats(m)

	// First set up monitor listening
	mon := NewMonitor(stat)
	go mon.Listen()
	time.Sleep(100 * time.Millisecond)

	// Then measure

	k := strconv.Itoa(int(mon.SinkPort))

	err := ConnectSink("localhost:" + k)
	if err != nil {
		t.Fatal(fmt.Sprintf("Error starting monitor: %s", err))
	}

	return mon, stat

}
