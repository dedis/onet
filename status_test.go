package onet

import (
	"strings"
	"testing"

	"strconv"

	"github.com/stretchr/testify/assert"
	"go.dedis.ch/onet/v3/ciphersuite"
)

func TestSRStruct(t *testing.T) {
	srs := newStatusReporterStruct()
	assert.NotNil(t, srs)
	dtr := &dummyTestReporter{5}
	srs.RegisterStatusReporter("Dummy", dtr)
	assert.Equal(t, srs.ReportStatus()["Dummy"].Field["Connections"], "5")
	dtr.Status = 10
	assert.Equal(t, srs.ReportStatus()["Dummy"].Field["Connections"], "10")
}

func TestStatusHost(t *testing.T) {
	builder := NewDefaultBuilder()
	builder.SetSuite(testSuite)
	builder.SetService("abc", nil, func(c *Context, suite ciphersuite.CipherSuite) (Service, error) {
		return nil, nil
	})

	l := NewLocalTest(builder)
	defer l.CloseAll()

	c := l.NewServer(2050)
	defer c.Close()

	stats := c.GetStatus()
	count := len(c.serviceManager.services)
	services := strings.Split(stats.Field["Available_Services"], ",")
	assert.Equal(t, len(services), count)
}

type dummyTestReporter struct {
	Status int
}

func (d *dummyTestReporter) GetStatus() *Status {
	return &Status{map[string]string{"Connections": strconv.Itoa(d.Status)}}
}
