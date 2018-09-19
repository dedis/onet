package onet

import (
	"errors"
	"testing"

	"reflect"

	"github.com/dedis/onet/log"
	"github.com/dedis/onet/network"
	"github.com/dedis/protobuf"
	"github.com/stretchr/testify/require"
)

const testServiceName = "testService"

func init() {
	RegisterNewService(testServiceName, newTestService)
	ServiceFactory.ServiceID(testServiceName)
	network.RegisterMessage(&testMsg{})
}

func TestProcessor_AddMessage(t *testing.T) {
	h1 := NewLocalServer(tSuite, 2000)
	defer h1.Close()
	p := NewServiceProcessor(&Context{server: h1})
	require.Nil(t, p.RegisterHandler(procMsg))
	if len(p.handlers) != 1 {
		require.Fail(t, "Should have registered one function")
	}
	mt := network.MessageType(&testMsg{})
	if mt.Equal(network.ErrorType) {
		require.Fail(t, "Didn't register message-type correctly")
	}
	var wrongFunctions = []interface{}{
		procMsgWrong1,
		procMsgWrong2,
		procMsgWrong3,
		procMsgWrong4,
		procMsgWrong5,
		procMsgWrong6,
		procMsgWrong7,
	}
	for _, f := range wrongFunctions {
		fsig := reflect.TypeOf(f).String()
		log.Lvl2("Checking function", fsig)
		require.Error(t, p.RegisterHandler(f),
			"Could register wrong function: "+fsig)
	}
}

func TestProcessor_RegisterMessages(t *testing.T) {
	h1 := NewLocalServer(tSuite, 2000)
	defer h1.Close()
	p := NewServiceProcessor(&Context{server: h1})
	require.Nil(t, p.RegisterHandlers(procMsg, procMsg2, procMsg3, procMsg4))
	require.Error(t, p.RegisterHandlers(procMsg3, procMsgWrong4))
}

func TestProcessor_RegisterStreamingMessage(t *testing.T) {
	h1 := NewLocalServer(tSuite, 2000)
	defer h1.Close()
	p := NewServiceProcessor(&Context{server: h1})
	f1 := func(m *testMsg) (chan int, error) {
		return make(chan int), nil
	}
	f2 := func(m *testMsg) (chan testMsg, error) {
		return make(chan testMsg), nil
	}
	require.Nil(t, p.RegisterStreamingHandlers(f1, f2))
}

func TestServiceProcessor_ProcessClientRequest(t *testing.T) {
	h1 := NewLocalServer(tSuite, 2000)
	defer h1.Close()
	p := NewServiceProcessor(&Context{server: h1})
	require.Nil(t, p.RegisterHandlers(procMsg, procMsg2))

	buf, err := protobuf.Encode(&testMsg{11})
	require.Nil(t, err)
	rep, repChan, err := p.ProcessClientRequest(nil, "testMsg", buf)
	require.Nil(t, repChan)
	require.NoError(t, err)
	val := &testMsg{}
	require.Nil(t, protobuf.Decode(rep, val))
	if val.I != 11 {
		require.Fail(t, "Value got lost - should be 11")
	}

	buf, err = protobuf.Encode(&testMsg{42})
	require.Nil(t, err)
	_, _, err = p.ProcessClientRequest(nil, "testMsg", buf)
	require.NotNil(t, err)

	buf, err = protobuf.Encode(&testMsg2{42})
	require.Nil(t, err)
	_, _, err = p.ProcessClientRequest(nil, "testMsg2", buf)
	require.NotNil(t, err)

	// Test non-existing endpoint
	buf, err = protobuf.Encode(&testMsg2{42})
	require.Nil(t, err)
	lvl := log.DebugVisible()
	log.SetDebugVisible(0)
	log.OutputToBuf()
	_, _, err = p.ProcessClientRequest(nil, "testMsgNotAvailable", buf)
	log.OutputToOs()
	log.SetDebugVisible(lvl)
	require.NotNil(t, err)
	require.NotEqual(t, "", log.GetStdOut())
}

func TestServiceProcessor_ProcessClientRequest_Streaming(t *testing.T) {
	h1 := NewLocalServer(tSuite, 2000)
	defer h1.Close()

	p := NewServiceProcessor(&Context{server: h1})
	f := func(m *testMsg) (chan network.Message, error) {
		c := make(chan network.Message)
		go func() {
			for i := 0; i < m.I; i++ {
				c <- m
			}
			close(c)
		}()
		return c, nil
	}
	require.Nil(t, p.RegisterStreamingHandler(f))

	n := 5
	buf, err := protobuf.Encode(&testMsg{n})
	require.NoError(t, err)
	rep, repChan, err := p.ProcessClientRequest(nil, "testMsg", buf)
	require.Nil(t, rep)
	require.NoError(t, err)

	for i := 0; i < n; i++ {
		buf := <-repChan
		val := &testMsg{}
		require.Nil(t, protobuf.Decode(buf, val))
		require.Equal(t, val.I, n)
	}
}

func TestProcessor_ProcessClientRequest(t *testing.T) {
	local := NewTCPTest(tSuite)

	// generate 5 hosts,
	h := local.GenServers(1)[0]
	defer local.CloseAll()

	client := local.NewClient(testServiceName)
	msg := &testMsg{}
	err := client.SendProtobuf(h.ServerIdentity, &testMsg{12}, msg)
	require.Nil(t, err)
	if msg == nil {
		require.Fail(t, "Msg should not be nil")
	}
	if msg.I != 12 {
		require.Fail(t, "Didn't send 12")
	}
}

type testMsg struct {
	I int
}

type testMsg2 testMsg
type testMsg3 testMsg
type testMsg4 testMsg
type testMsg5 testMsg

func procMsg(msg *testMsg) (network.Message, error) {
	// Return an error for testing
	if msg.I == 42 {
		return nil, errors.New("42 is NOT the answer")
	}
	return msg, nil
}

func procMsg2(msg *testMsg2) (network.Message, error) {
	// Return an error for testing
	if msg.I == 42 {
		return nil, errors.New("42 is NOT the answer")
	}
	return nil, nil
}
func procMsg3(msg *testMsg3) (network.Message, error) {
	return nil, nil
}
func procMsg4(msg *testMsg4) (*testMsg4, error) {
	return msg, nil
}

func procMsgWrong1() (network.Message, error) {
	return nil, nil
}

func procMsgWrong2(msg testMsg2) (network.Message, error) {
	return msg, nil
}

func procMsgWrong3(msg *testMsg3) error {
	return nil
}

func procMsgWrong4(msg *testMsg4) (error, network.Message) {
	return nil, msg
}

func procMsgWrong5(msg *testMsg) (*network.Message, error) {
	return nil, nil
}

func procMsgWrong6(msg *testMsg) (int, error) {
	return 10, nil
}
func procMsgWrong7(msg *testMsg) (testMsg, error) {
	return *msg, nil
}

type testService struct {
	*ServiceProcessor
	Msg interface{}
}

func newTestService(c *Context) (Service, error) {
	ts := &testService{
		ServiceProcessor: NewServiceProcessor(c),
	}
	if err := ts.RegisterHandler(ts.ProcessMsg); err != nil {
		panic(err.Error())
	}
	return ts, nil
}

func (ts *testService) NewProtocol(tn *TreeNodeInstance, conf *GenericConfig) (ProtocolInstance, error) {
	return nil, nil
}

func (ts *testService) ProcessMsg(msg *testMsg) (network.Message, error) {
	ts.Msg = msg
	return msg, nil
}
