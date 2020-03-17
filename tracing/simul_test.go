package tracing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSimulHC(t *testing.T) {
	shc := simulCollector{}
	_, hct := shc.newTrace(context.TODO(), "")
	_, hct2 := shc.newTrace(context.TODO(), "")
	go func() {
		_, sp := hct2.GetRootSpan().CreateChild(hct2.(*simulTrace).ctx)
		sp.AddField("sp2", 2)
		hct2.Send()
	}()
	go func() {
		_, sp := hct.GetRootSpan().CreateChild(hct.(*simulTrace).ctx)
		sp.AddField("sp1", 1)
		hct.Send()
	}()
	shc.Wg.Wait()
	require.Equal(t, 2, len(shc.Traces))
	found := 0
	for _, c := range shc.Traces {
		if len(c) == 2 {
			if c[0]["sp1"] == "1" {
				found++
			}
			if c[0]["sp2"] == "2" {
				found++
			}
		}
	}
	require.Equal(t, 2, found)
}

func TestNewTrace(t *testing.T) {
	ctx, hcTr := newSimulTrace(context.TODO(), "")
	tr, _ := newTraceWrapper(ctx, hcTr, simpleSe("root"))
	tr.hcTrace.AddField("one", 2)
	tr.send()
	testSentTrace(t, tr, `method:"root";one:2`)
}

func TestNewTraceSpan(t *testing.T) {
	ctx, hcTr := newSimulTrace(context.TODO(), "")
	tr, child := newTraceWrapper(ctx, hcTr, simpleSe("root", "one"))
	child.log(1, "logging")
	tr.hcTrace.AddField("two", 2)
	tr.hcTrace.(*simulTrace).printTrace()
	tr.send()
	testSentTrace(t, tr,
		`log.Lvl:1;log.Msg:"logging";method:"one";two:2`,
		`method:"one";two:2`,
		`method:"root";two:2`)
}

func TestHCS(t *testing.T) {
	_, tr := newSimulTrace(context.TODO(), "")
	hcs := tr.root
	hcs.AddField("two", 2)
	_, sp := hcs.CreateChild(context.TODO())
	sp.AddField("one", 1)
	hcs.Send()
	testSent(t, hcs.trace.sent, "one:1", "two:2")
}

func TestHCSChild(t *testing.T) {
	_, tr := newSimulTrace(context.TODO(), "")
	hcs := tr.root
	hcs.AddField("two", 2)
	_, sp := hcs.CreateChild(context.TODO())
	sp.AddField("one", 1)
	_, spc := sp.CreateChild(context.TODO())
	spc.AddField("three", 3)
	hcs.Send()
	testSent(t, hcs.trace.sent, "three:3", "one:1", "two:2")
}
