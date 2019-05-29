package monitor

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSingleMeasure(t *testing.T) {
	mon, stats := setupMonitor(t)
	mon.InsertBucket(0, []string{"0:1"}, NewStats(nil))

	RecordSingleMeasure("a", 1)
	RecordSingleMeasureWithHost("a", 5, 0)

	time.Sleep(100 * time.Millisecond)

	stats.Collect()
	require.NotNil(t, stats.Value("a"))
	require.Equal(t, 3.0, stats.Value("a").Avg())

	b := mon.buckets.Get(0)
	require.NotNil(t, b.Value(("a")))
	require.Equal(t, 5.0, b.Value("a").Avg())

	EndAndCleanup()
	time.Sleep(100 * time.Millisecond)
}

func TestTimeMeasure(t *testing.T) {
	mon, stats := setupMonitor(t)
	mon.InsertBucket(0, []string{"0:1"}, NewStats(nil))

	NewTimeMeasure("a").Record()
	tm := NewTimeMeasureWithHost("a", 0)

	time.Sleep(50 * time.Millisecond)
	tm.Record()

	time.Sleep(100 * time.Millisecond)

	stats.Collect()
	v := stats.Value("a_wall")
	require.NotNil(t, v)
	// the value is in seconds
	require.True(t, v.Min() > 0 && v.Min() < 0.05, fmt.Sprintf("%v < 0.05", v.Min()))

	b := mon.buckets.Get(0)
	require.NotNil(t, b.Value("a_wall"))
	// the value is in seconds
	require.True(t, b.Value("a_wall").Min() > 0.05, fmt.Sprintf("%v > 0.05", v.Min()))

	EndAndCleanup()
	time.Sleep(100 * time.Millisecond)
}

type DummyCounterIO struct {
	rvalue    uint64
	wvalue    uint64
	msgrvalue uint64
	msgwvalue uint64
}

func (dm *DummyCounterIO) Rx() uint64 {
	dm.rvalue += 10
	return dm.rvalue
}

func (dm *DummyCounterIO) Tx() uint64 {
	dm.wvalue += 10
	return dm.wvalue
}

func (dm *DummyCounterIO) MsgRx() uint64 {
	dm.msgrvalue++
	return dm.msgrvalue
}

func (dm *DummyCounterIO) MsgTx() uint64 {
	dm.msgwvalue++
	return dm.msgwvalue
}

func TestCounterIOMeasureRecord(t *testing.T) {
	mon, _ := setupMonitor(t)
	mon.InsertBucket(0, []string{"0:1"}, NewStats(nil))
	dm := &DummyCounterIO{0, 0, 0, 0}
	// create the counter measure
	cm := NewCounterIOMeasureWithHost("dummy", dm, 0)
	if cm.baseRx != dm.rvalue || cm.baseTx != dm.wvalue {
		t.Logf("baseRx = %d vs rvalue = %d || baseTx = %d vs wvalue = %d", cm.baseRx, dm.rvalue, cm.baseTx, dm.wvalue)
		t.Fatal("Tx() / Rx() not working ?")
	}
	//bread, bwritten := cm.baseRx, cm.baseTx
	cm.Record()
	// check the values again
	if cm.baseRx != dm.rvalue || cm.baseTx != dm.wvalue {
		t.Fatal("Record() not working for CounterIOMeasure")
	}

	// Important otherwise data don't get written down to the monitor yet.
	time.Sleep(100 * time.Millisecond)
	str := new(bytes.Buffer)
	stat := mon.stats
	stat.Collect()
	stat.WriteHeader(str)
	stat.WriteValues(str)
	wr, re := stat.Value("dummy_tx"), stat.Value("dummy_rx")
	if wr == nil || wr.Avg() != 10 {
		t.Logf("stats => %v", stat.values)
		if wr != nil {
			t.Logf("wr.Avg() = %f", wr.Avg())
		}
		t.Fatal("Stats doesn't have the right value (write)")
	}
	if re == nil || re.Avg() != 10 {
		t.Fatal("Stats doesn't have the right value (read)")
	}
	mwr, mre := stat.Value("dummy_msg_tx"), stat.Value("dummy_msg_rx")
	if mwr == nil || mwr.Avg() != 1 {
		t.Fatal("Stats doesn't have the right value (msg written)")
	}
	if mre == nil || mre.Avg() != 1 {
		t.Fatal("Stats doesn't have the right value (msg read)")
	}

	// check the bucket is filled
	b := mon.buckets.Get(0)
	require.Equal(t, 10.0, b.Value("dummy_tx").Avg())

	EndAndCleanup()
	time.Sleep(100 * time.Millisecond)
}

// Test that reset sets the values to the base ones
func TestCounterIOMeasureReset(t *testing.T) {
	dm := &DummyCounterIO{0, 0, 0, 0}
	cm := NewCounterIOMeasureWithHost("dummy", dm, 0)

	// increase the actual
	dm.Tx()
	dm.Rx()
	dm.MsgRx()
	dm.MsgTx()

	// several resets should still get the base values
	cm.Reset()
	cm.Reset()

	if cm.baseRx != dm.rvalue || cm.baseTx != dm.wvalue {
		t.Logf("baseRx = %d vs rvalue = %d || baseTx = %d vs wvalue = %d",
			cm.baseRx, dm.rvalue, cm.baseTx, dm.wvalue)
		t.Fatal("Tx() / Rx() not working ?")
	}

	if cm.baseMsgRx != dm.msgrvalue || cm.baseMsgTx != dm.msgwvalue {
		t.Logf("baseMsgRx = %d vs msgrvalue = %d || baseMsgTx = %d vs msgwvalue = %d",
			cm.baseMsgRx, dm.msgrvalue, cm.baseMsgTx, dm.msgwvalue)
		t.Fatal("MsgTx() / MsgRx() not working ?")
	}
}
