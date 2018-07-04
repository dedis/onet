package onet

import (
	"testing"
	"time"

	"github.com/dedis/kyber/suites"
	"github.com/dedis/onet/log"
	"github.com/dedis/onet/network"
	"github.com/stretchr/testify/require"
)

var tSuite = suites.MustFind("Ed25519")

const clientServiceName = "ClientService"

var clientServiceID ServiceID

func init() {
	var err error
	clientServiceID, err = RegisterNewService(clientServiceName, newClientService)
	log.ErrFatal(err)
}

func Test_panicClose(t *testing.T) {
	l := NewLocalTest(tSuite)
	l.CloseAll()
	require.Panics(t, func() { l.genLocalHosts(2) })
}

func Test_showPanic(t *testing.T) {
	l := NewLocalTest(tSuite)
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

func TestGenLocalHost(t *testing.T) {
	l := NewLocalTest(tSuite)
	hosts := l.genLocalHosts(2)
	defer l.CloseAll()

	log.Lvl4("Hosts are:", hosts[0].Address(), hosts[1].Address())
	if hosts[0].Address() == hosts[1].Address() {
		t.Fatal("Both addresses are equal")
	}
}

// This tests the client-connection in the case of a non-garbage-collected
// client that stays in the service.
func TestNewTCPTest(t *testing.T) {
	l := NewTCPTest(tSuite)
	_, el, _ := l.GenTree(3, true)
	defer l.CloseAll()

	c1 := NewClient(tSuite, clientServiceName)
	err := c1.SendProtobuf(el.List[0], &SimpleMessage{}, nil)
	log.ErrFatal(err)
}

func TestWaitDone(t *testing.T) {
	l := NewTCPTest(tSuite)
	servers, ro, _ := l.GenTree(1, true)
	defer l.CloseAll()

	services := l.GetServices(servers, clientServiceID)
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
	cl    *Client
	click chan bool
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

func newClientService(c *Context) (Service, error) {
	s := &clientService{
		ServiceProcessor: NewServiceProcessor(c),
		cl:               NewClient(c.server.Suite(), clientServiceName),
		click:            make(chan bool, 1),
	}
	log.ErrFatal(s.RegisterHandlers(s.SimpleMessage, s.SimpleMessage2))
	s.RegisterProcessorFunc(network.RegisterMessage(RawMessage{}), func(arg1 *network.Envelope) {
		s.click <- true
		time.Sleep(100 * time.Millisecond)
		s.click <- true
	})

	return s, nil
}
