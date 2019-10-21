package onet

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/onet/v4/ciphersuite"
	"go.dedis.ch/onet/v4/log"
	"go.dedis.ch/onet/v4/network"
)

const clientServiceName = "ClientService"

var clientServiceID ServiceID

var localTestBuilder = NewDefaultBuilder()

func init() {
	localTestBuilder.SetSuite(testSuite)
	localTestBuilder.SetService(clientServiceName, nil, newClientService)
}

func Test_panicClose(t *testing.T) {
	l := NewLocalTest(NewLocalBuilder(localTestBuilder))
	l.CloseAll()
	require.Panics(t, func() { l.genLocalHosts(2) })
}

func Test_showPanic(t *testing.T) {
	l := NewLocalTest(NewLocalBuilder(localTestBuilder))
	c := make(chan bool)
	go func() {
		<-c
	}()
	defer func() {
		require.NotNil(t, recover())
		c <- true
	}()
	defer l.CloseAll()
	panic("this should be caught")
}

func Test_showFail(t *testing.T) {
	t.Skip("I have no idea how I can have this test passing... It tests that CloseAll doesn't test goroutines when a test fails.")
	l := NewLocalTest(NewLocalBuilder(localTestBuilder))
	c := make(chan bool)
	go func() {
		<-c
	}()
	defer l.CloseAll()
	defer func() {
		if !t.Failed() {
			t.Fail()
		}
		c <- true
	}()
	l.T = t
	require.Nil(t, "not nil")
}

func TestGenLocalHost(t *testing.T) {
	l := NewLocalTest(NewLocalBuilder(localTestBuilder))
	hosts := l.genLocalHosts(2)
	defer l.CloseAll()

	log.Lvl4("Hosts are:", hosts[0].Address(), hosts[1].Address())
	if hosts[0].Address() == hosts[1].Address() {
		t.Fatal("Both addresses are equal")
	}
}

func TestGenLocalHostAfter(t *testing.T) {
	l := NewLocalTest(NewLocalBuilder(localTestBuilder))
	defer l.CloseAll()
	hosts := l.genLocalHosts(2)
	hosts2 := l.genLocalHosts(2)
	require.NotEqual(t, hosts2[0].Address(), hosts[0].Address())
}

// This tests the client-connection in the case of a non-garbage-collected
// client that stays in the service.
func TestNewTCPTest(t *testing.T) {
	l := NewLocalTest(localTestBuilder)
	_, el, _ := l.GenTree(3, true)
	defer l.CloseAll()

	c1 := NewClient(clientServiceName)
	err := c1.SendProtobuf(el.List[0], &SimpleMessage{}, nil)
	log.ErrFatal(err)
}

func TestNewTLSTest(t *testing.T) {
	builder := localTestBuilder.Clone()
	builder.(*DefaultBuilder).UseTLS()

	l := NewLocalTest(builder)
	_, ro, _ := l.GenTree(3, true)
	defer l.CloseAll()

	c1 := NewClient(clientServiceName)
	err := c1.SendProtobuf(ro.List[0], &SimpleMessage{}, nil)
	require.NoError(t, err)
}

func TestLocalTCPGenConnectableRoster(t *testing.T) {
	l := NewLocalTest(localTestBuilder)
	defer l.CloseAll()
	servers := l.GenServers(3)
	roster := *l.GenRosterFromHost(servers...)

	for _, serverIdent := range roster.List {
		got, err := http.Get(serverIdent.URL)
		require.NoError(t, err)
		got.Body.Close()
	}
}

// Tests whether TestClose is called in the service.
func TestTestClose(t *testing.T) {
	l := NewLocalTest(localTestBuilder)
	servers, _, _ := l.GenTree(1, true)
	services := l.GetServices(servers, clientServiceName)
	pingpong := make(chan bool, 1)
	go func() {
		pingpong <- true
		for _, s := range services {
			<-s.(*clientService).closed
		}
		pingpong <- true
	}()
	// Wait for the go-routine to be started
	<-pingpong
	l.CloseAll()
	// Wait for all services to be clsoed
	<-pingpong
}

func TestWaitDone(t *testing.T) {
	l := NewLocalTest(localTestBuilder)
	servers, ro, _ := l.GenTree(1, true)
	defer l.CloseAll()

	services := l.GetServices(servers, clientServiceName)
	service := services[0].(*clientService)
	require.Nil(t, service.SendRaw(ro.List[0], &RawMessage{}))
	<-service.click
	select {
	case <-service.click:
		log.Fatal("service is already done")
	default:
	}
	require.Nil(t, l.WaitDone(5*time.Second))
	select {
	case <-service.click:
	default:
		log.Fatal("service should be done by now")
	}
}

type clientService struct {
	*ServiceProcessor
	cl     *Client
	click  chan bool
	closed chan bool
}

type SimpleMessage2 struct{}

type RawMessage struct{}

func (c *clientService) SimpleMessage(msg *SimpleMessage) (network.Message, error) {
	log.Lvl3("Got request", msg)
	c.cl.SendProtobuf(c.ServerIdentity(), &SimpleMessage2{}, nil)
	return nil, nil
}

func (c *clientService) SimpleMessage2(msg *SimpleMessage2) (network.Message, error) {
	log.Lvl3("Got request", msg)
	return nil, nil
}

func (c *clientService) TestClose() {
	c.closed <- true
}

func newClientService(c *Context, suite ciphersuite.CipherSuite) (Service, error) {
	s := &clientService{
		ServiceProcessor: NewServiceProcessor(c),
		cl:               NewClient(clientServiceName),
		click:            make(chan bool, 1),
		closed:           make(chan bool, 1),
	}
	log.ErrFatal(s.RegisterHandlers(s.SimpleMessage, s.SimpleMessage2))
	s.RegisterProcessorFunc(network.RegisterMessage(RawMessage{}), func(arg1 *network.Envelope) error {
		s.click <- true
		time.Sleep(100 * time.Millisecond)
		s.click <- true
		return nil
	})

	return s, nil
}
