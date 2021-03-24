package onet

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"reflect"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/group/edwards25519"
	"go.dedis.ch/kyber/v3/pairing/bn256"
	"go.dedis.ch/kyber/v3/sign/bls"
	"go.dedis.ch/kyber/v3/util/key"
	"go.dedis.ch/kyber/v3/util/random"
	"go.dedis.ch/onet/v3/log"
	"go.dedis.ch/onet/v3/network"
	"go.dedis.ch/protobuf"
	"golang.org/x/xerrors"
)

const testServiceName = "testService"

func init() {
	RegisterNewService(testServiceName, newTestService)
	ServiceFactory.ServiceID(testServiceName)
	network.RegisterMessages(&testMsg{}, &testPanicMsg{})
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
	require.Error(t, p.RegisterHandlers(procMsg3, procMsgWrong1))
}

func TestProcessor_RegisterStreamingMessage(t *testing.T) {
	h1 := NewLocalServer(tSuite, 2000)
	defer h1.Close()
	p := NewServiceProcessor(&Context{server: h1})

	// correct registration
	f1 := func(m *testMsg) (chan network.Message, chan bool, error) {
		return make(chan network.Message), make(chan bool), nil
	}
	f2 := func(m *testMsg) (chan *testMsg, chan bool, error) {
		return make(chan *testMsg), make(chan bool), nil
	}
	require.Nil(t, p.RegisterStreamingHandlers(f1, f2))

	// wrong registrations
	require.Error(t, p.RegisterStreamingHandler(
		func(m int) (chan network.Message, chan bool, error) {
			return nil, nil, nil
		}), "input must be a pointer to struct")
	require.Error(t, p.RegisterStreamingHandler(
		func(m testMsg) (chan network.Message, chan bool, error) {
			return nil, nil, nil
		}), "input must be a pointer to struct")
	require.Error(t, p.RegisterStreamingHandler(
		func(m *testMsg) (chan int, chan bool, error) {
			return nil, nil, nil
		}), "first return must be a pointer or interface")
	require.Error(t, p.RegisterStreamingHandler(
		func(m *testMsg) (chan testMsg, chan bool, error) {
			return nil, nil, nil
		}), "first return must be a pointer or interface")
	require.Error(t, p.RegisterStreamingHandler(
		func(m *testMsg) (chan testMsg, error) {
			return nil, nil
		}), "must have three return values")
	require.Error(t, p.RegisterStreamingHandler(
		func(m *testMsg) (chan testMsg, chan int, error) {
			return nil, nil, nil
		}), "second return must be a boolean channel")
	require.Error(t, p.RegisterStreamingHandler(
		func(m *testMsg) (chan testMsg, []int, error) {
			return nil, nil, nil
		}), "second return must be a boolean channel")
	require.Error(t, p.RegisterStreamingHandler(
		func(m *testMsg) (chan testMsg, chan int, []byte) {
			return nil, nil, nil
		}), "must return an error")
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
	require.NotEqual(t, "", log.GetStdErr())
}

func TestServiceProcessor_ProcessClientRequest_Streaming_Simple(t *testing.T) {
	h1 := NewLocalServer(tSuite, 2000)
	defer h1.Close()

	p := NewServiceProcessor(&Context{server: h1})
	h := func(m *testMsg) (chan network.Message, chan bool, error) {
		outChan := make(chan network.Message)
		closeChan := make(chan bool)
		go func() {
			for i := 0; i < int(m.I); i++ {
				outChan <- m
			}
			<-closeChan
			close(outChan)
		}()
		return outChan, closeChan, nil
	}
	require.Nil(t, p.RegisterStreamingHandler(h))

	n := 5
	buf, err := protobuf.Encode(&testMsg{int64(n)})
	require.NoError(t, err)
	rep, _, err := p.ProcessClientRequest(nil, "testMsg", buf)
	// Using ProcessClientRequest with a streaming request should yield an error
	require.Nil(t, rep)
	require.Error(t, err)

	inputChan := make(chan []byte, 1)
	inputChan <- buf
	outChan, err := p.ProcessClientStreamRequest(nil, "testMsg", inputChan)

	require.NoError(t, err)

	for i := 0; i < n; i++ {
		buf := <-outChan
		val := &testMsg{}
		require.Nil(t, protobuf.Decode(buf, val))
		require.Equal(t, val.I, int64(n))
	}
	close(inputChan)
}

func TestServiceProcessor_ProcessClientRequest_Streaming_Multiple(t *testing.T) {
	h1 := NewLocalServer(tSuite, 2000)

	// order matters: last-in first-out
	defer log.AfterTest(t) // check for leaking go routines
	defer h1.Close()

	p := NewServiceProcessor(&Context{server: h1})
	h := func(m *testMsg) (chan network.Message, chan bool, error) {
		outChan := make(chan network.Message)
		closeChan := make(chan bool)
		go func() {
			for i := 0; i < int(m.I); i++ {
				outChan <- m
			}
			<-closeChan
			close(outChan)
		}()
		return outChan, closeChan, nil
	}
	require.Nil(t, p.RegisterStreamingHandler(h))

	n := 5
	buf, err := protobuf.Encode(&testMsg{int64(n)})
	require.NoError(t, err)

	// Send 3 requests
	inputChan := make(chan []byte, 3)
	outChan, err := p.ProcessClientStreamRequest(nil, "testMsg", inputChan)
	require.NoError(t, err)

	inputChan <- buf
	inputChan <- buf
	inputChan <- buf

	for k := 0; k < 3; k++ {
		for i := 0; i < n; i++ {
			buf := <-outChan
			val := &testMsg{}
			require.Nil(t, protobuf.Decode(buf, val))
			require.Equal(t, val.I, int64(n))
		}
	}

	close(inputChan)
}

func TestServiceProcessor_ProcessClientStreamRequest(t *testing.T) {
	h1 := NewLocalServer(tSuite, 2000)
	defer h1.Close()

	p := NewServiceProcessor(&Context{server: h1})
	// The function need to use the same outChan and closeChan if a request is
	// coming from the same client that expects the same outChan to be used.
	serviceStruct := struct {
		once      sync.Once
		outChan   chan network.Message
		closeChan chan bool
	}{
		outChan:   make(chan network.Message, 10),
		closeChan: make(chan bool),
	}

	h := func(m *testMsg) (chan network.Message, chan bool, error) {
		go func() {
			for i := 0; i < int(m.I); i++ {
				serviceStruct.outChan <- m
			}
			<-serviceStruct.closeChan
			serviceStruct.once.Do(func() {
				close(serviceStruct.outChan)
			})
		}()
		return serviceStruct.outChan, serviceStruct.closeChan, nil
	}
	require.Nil(t, p.RegisterStreamingHandler(h))

	// Sending a first message from the client to the service.
	n := 5
	clientInputs := make(chan []byte, 10)
	buf, err := protobuf.Encode(&testMsg{int64(n)})
	require.NoError(t, err)
	clientInputs <- buf
	outVals, err := p.ProcessClientStreamRequest(nil, "testMsg", clientInputs)
	require.NoError(t, err)

	for i := 0; i < n; i++ {
		buf := <-outVals
		val := &testMsg{}
		require.Nil(t, protobuf.Decode(buf, val))
		require.Equal(t, val.I, int64(n))
	}

	// Sending an additional message from the client to the service using the
	// same channel.
	n = 2
	buf, err = protobuf.Encode(&testMsg{int64(n)})
	require.NoError(t, err)
	clientInputs <- buf

	for i := 0; i < n; i++ {
		buf := <-outVals
		val := &testMsg{}
		require.Nil(t, protobuf.Decode(buf, val))
		require.Equal(t, val.I, int64(n))
	}

	close(clientInputs)
}

// If the caller closes early the client input chans then the
// ProcessClientStreamRequest should not try to decode an empty buffer
func TestServiceProcessor_ProcessClientStreamRequest_close_early(t *testing.T) {
	h1 := NewLocalServer(tSuite, 2000)
	defer h1.Close()

	p := NewServiceProcessor(&Context{server: h1})
	// The function need to use the same outChan and closeChan if a request is
	// coming from the same client that expects the same outChan to be used.
	serviceStruct := struct {
		once      sync.Once
		outChan   chan network.Message
		closeChan chan bool
	}{
		outChan:   make(chan network.Message, 10),
		closeChan: make(chan bool),
	}

	h := func(m *testMsg) (chan network.Message, chan bool, error) {
		go func() {
			for i := 0; i < int(m.I); i++ {
				serviceStruct.outChan <- m
			}
			<-serviceStruct.closeChan
			serviceStruct.once.Do(func() {
				close(serviceStruct.outChan)
			})
		}()
		return serviceStruct.outChan, serviceStruct.closeChan, nil
	}
	require.Nil(t, p.RegisterStreamingHandler(h))

	// Sending a first message from the client to the service.
	clientInputs := make(chan []byte, 10)
	// early close by the caller
	close(clientInputs)
	_, err := p.ProcessClientStreamRequest(nil, "testMsg", clientInputs)
	require.NoError(t, err)
}

func TestProcessor_ProcessClientRequest(t *testing.T) {
	local := NewTCPTest(tSuite)

	// generate 1 host
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

// Test that the panic will be recovered and announced without crashing the server.
func TestProcessor_PanicClientRequest(t *testing.T) {
	local := NewTCPTest(tSuite)

	h := local.GenServers(1)[0]
	defer local.CloseAll()

	client := local.NewClient(testServiceName)
	err := client.SendProtobuf(h.ServerIdentity, &testPanicMsg{}, struct{}{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "deadbeef")
}

type testMsg struct {
	I int64
}

type testMsg2 testMsg
type testMsg3 testMsg
type testMsg4 testMsg
type testMsg5 testMsg
type testPanicMsg struct{}

func procMsg(msg *testMsg) (network.Message, error) {
	// Return an error for testing
	if msg.I == 42 {
		return nil, xerrors.New("42 is NOT the answer")
	}
	return msg, nil
}

func procMsg2(msg *testMsg2) (network.Message, error) {
	// Return an error for testing
	if msg.I == 42 {
		return nil, xerrors.New("42 is NOT the answer")
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

func procMsgWrong4(msg *testMsg) (*network.Message, error) {
	return nil, nil
}

func procMsgWrong5(msg *testMsg) (int, error) {
	return 10, nil
}

func procMsgWrong6(msg *testMsg) (testMsg, error) {
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
	if err := ts.RegisterHandlers(ts.ProcessMsg, ts.ProcessMsgPanic); err != nil {
		panic(err.Error())
	}

	if err := ts.RegisterRESTHandler(procRestMsgGET1, testServiceName, "GET", 3, 3); err != nil {
		panic(err.Error())
	}
	if err := ts.RegisterRESTHandler(procRestMsgGET2, testServiceName, "GET", 3, 3); err != nil {
		panic(err.Error())
	}
	if err := ts.RegisterRESTHandler(procRestMsgGET3, testServiceName, "GET", 3, 3); err != nil {
		panic(err.Error())
	}
	if err := ts.RegisterRESTHandler(procRestMsgPOSTString, testServiceName, "POST", 3, 3); err != nil {
		panic(err.Error())
	}
	if err := ts.RegisterRESTHandler(procRestMsgPOSTPoint, testServiceName, "POST", 3, 3); err != nil {
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

func (ts *testService) ProcessMsgPanic(msg *testPanicMsg) (network.Message, error) {
	panic("deadbeef")
}

func TestProcessor_REST_Registration(t *testing.T) {
	h1 := NewLocalServer(tSuite, 2000)
	defer h1.Close()
	p := NewServiceProcessor(&Context{server: h1})
	require.NoError(t, p.RegisterRESTHandler(procRestMsgGET1, "dummyService", "GET", 3, 3))
	require.NoError(t, p.RegisterRESTHandler(procRestMsgGET2, "dummyService", "GET", 3, 3))
	require.NoError(t, p.RegisterRESTHandler(procRestMsgGET3, "dummyService", "GET", 3, 3))

	require.NoError(t, p.RegisterRESTHandler(procRestMsgPOSTString, "dummyService", "POST", 3, 3))
	require.NoError(t, p.RegisterRESTHandler(procRestMsgPOSTPoint, "dummyService", "POST", 3, 3))
	require.NoError(t, p.RegisterRESTHandler(procMsg, "dummyService", "POST", 3, 3))
	require.NoError(t, p.RegisterRESTHandler(procMsg2, "dummyService", "POST", 3, 3))
	require.NoError(t, p.RegisterRESTHandler(procMsg3, "dummyService", "POST", 3, 3))
	require.NoError(t, p.RegisterRESTHandler(procMsg4, "dummyService", "POST", 3, 3))

	require.Error(t, p.RegisterRESTHandler(procRestMsgGET3, "dummyService", "GET", 3, 2))
	require.Error(t, p.RegisterRESTHandler(procRestMsgGET3, "dummyService", "GET", 1, 2))
	require.Error(t, p.RegisterRESTHandler(procRestMsgGETWrong1, "dummyService", "GET", 3, 3))
	require.Error(t, p.RegisterRESTHandler(procRestMsgGETWrong2, "dummyService", "GET", 3, 3))
	require.Error(t, p.RegisterRESTHandler(procRestMsgGETWrong3, "dummyService", "GET", 3, 3))
	require.Error(t, p.RegisterRESTHandler(procRestMsgGET1, "dummyService", "XXX", 3, 3))

	require.Error(t, p.RegisterRESTHandler(procMsgWrong1, "dummyService", "POST", 3, 3))
	require.Error(t, p.RegisterRESTHandler(procMsgWrong2, "dummyService", "POST", 3, 3))
	require.Error(t, p.RegisterRESTHandler(procMsgWrong3, "dummyService", "POST", 3, 3))
	require.Error(t, p.RegisterRESTHandler(procMsgWrong4, "dummyService", "POST", 3, 3))
	require.Error(t, p.RegisterRESTHandler(procMsgWrong5, "dummyService", "POST", 3, 3))
	require.Error(t, p.RegisterRESTHandler(procMsgWrong6, "dummyService", "POST", 3, 3))
}

type bnPoint struct {
	P kyber.Point
}

type bnPointWrapper struct {
	P []byte
}

func (p bnPoint) MarshalJSON() ([]byte, error) {
	buf, err := p.P.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return []byte(fmt.Sprintf(`{"P": "%s"}`, base64.StdEncoding.EncodeToString(buf))), nil
}

func (p *bnPoint) UnmarshalJSON(b []byte) error {
	wrapper := bnPointWrapper{}
	if err := json.Unmarshal(b, &wrapper); err != nil {
		return err
	}
	suite := bn256.NewSuite()
	g2 := suite.G2().Point()
	if err := g2.UnmarshalBinary(wrapper.P); err != nil {
		return err
	}
	p.P = g2
	return nil
}

type edPoint struct {
	P kyber.Point
}

type edPointWrapper struct {
	P []byte
}

func (p edPoint) MarshalJSON() ([]byte, error) {
	buf, err := p.P.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return []byte(fmt.Sprintf(`{"P": "%s"}`, base64.StdEncoding.EncodeToString(buf))), nil
}

func (p *edPoint) UnmarshalJSON(b []byte) error {
	wrapper := edPointWrapper{}
	if err := json.Unmarshal(b, &wrapper); err != nil {
		return err
	}
	suite := edwards25519.NewBlakeSHA256Ed25519()
	point := suite.Point()
	if err := point.UnmarshalBinary(wrapper.P); err != nil {
		return err
	}
	p.P = point
	return nil
}

type twoPoints struct {
	PointEd edPoint
	PointBn bnPoint
}

func TestMarshalJSON_bn(t *testing.T) {
	suite := bn256.NewSuite()
	_, pk := bls.NewKeyPair(suite, random.New())
	p := bnPoint{pk}
	buf, err := json.Marshal(&p)
	require.NoError(t, err)

	p2 := bnPoint{}
	err = json.Unmarshal(buf, &p2)
	require.NoError(t, err)

	require.True(t, p2.P.Equal(pk))
}

func TestMarshalJSON_ed(t *testing.T) {
	suite := edwards25519.NewBlakeSHA256Ed25519()
	pk := key.NewKeyPair(suite).Public

	p := edPoint{pk}
	buf, err := json.Marshal(&p)
	require.NoError(t, err)

	p2 := edPoint{}
	err = json.Unmarshal(buf, &p2)
	require.NoError(t, err)

	require.True(t, p2.P.Equal(pk))
}

func TestMarshalJSON_twoPoints(t *testing.T) {
	pkEd := key.NewKeyPair(edwards25519.NewBlakeSHA256Ed25519()).Public
	_, pkBn := bls.NewKeyPair(bn256.NewSuite(), random.New())

	pTwo := twoPoints{edPoint{pkEd}, bnPoint{pkBn}}
	buf, err := json.Marshal(&pTwo)
	require.NoError(t, err)

	pTwo2 := twoPoints{}
	err = json.Unmarshal(buf, &pTwo2)
	require.NoError(t, err)

	require.True(t, pTwo2.PointEd.P.Equal(pkEd))
	require.True(t, pTwo2.PointBn.P.Equal(pkBn))
}

func TestProcessor_REST_Handler(t *testing.T) {
	log.AddUserUninterestingGoroutine("created by net/http.(*Transport).dialConn")

	local := NewTCPTest(tSuite)

	// generate 1 host
	h := local.GenServers(1)[0]
	defer local.CloseAll()

	c := http.Client{}
	port, err := strconv.Atoi(h.ServerIdentity.Address.Port())
	require.NoError(t, err)
	addr := "http://" + h.ServerIdentity.Address.Host() + ":" + strconv.Itoa(port+1)

	// get with empty "body"
	resp, err := c.Get(addr + "/v3/testService/restMsgGET1")
	require.NoError(t, err)
	require.Equal(t, resp.StatusCode, http.StatusOK)
	msg := testMsg{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&msg))
	require.Equal(t, int64(42), msg.I)

	// get by an integer
	resp, err = c.Get(addr + "/v3/testService/restMsgGET2/99")
	require.NoError(t, err)
	require.Equal(t, resp.StatusCode, http.StatusOK)
	msg = testMsg{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&msg))
	require.Equal(t, int64(99), msg.I)

	// get by byte slice, e.g., skipchain ID
	resp, err = c.Get(addr + "/v3/testService/restMsgGET3/deadbeef")
	require.NoError(t, err)
	require.Equal(t, resp.StatusCode, http.StatusOK)
	msg = testMsg{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&msg))
	require.Equal(t, int64(0xde), msg.I)

	// wrong url
	// NOTE: the error code is 400 because the websocket upgrade failed
	// usually it should be http.StatusNotFound
	resp, err = c.Get(addr + "/v3/testService/doesnotexist")
	require.NoError(t, err)
	require.Equal(t, resp.StatusCode, http.StatusBadRequest)

	// wrong url (extra slash)
	resp, err = c.Get(addr + "/v3/testService/restMsgGET3/deadbeef/")
	require.NoError(t, err)
	require.Equal(t, resp.StatusCode, http.StatusNotFound)
	checkJSONMsg(t, resp.Body, "invalid path")

	// wrong encoding of integer
	resp, err = c.Get(addr + "/v3/testService/restMsgGET2/one")
	require.NoError(t, err)
	require.Equal(t, resp.StatusCode, http.StatusNotFound)
	checkJSONMsg(t, resp.Body, "invalid path")

	// wrong encoding of byte slice (must be hex)
	resp, err = c.Get(addr + "/v3/testService/restMsgGET3/zzzz")
	require.NoError(t, err)
	require.Equal(t, resp.StatusCode, http.StatusNotFound)
	checkJSONMsg(t, resp.Body, "invalid path")

	// good post request
	resp, err = c.Post(addr+"/v3/testService/restMsgPOSTString", "application/json", bytes.NewReader([]byte(`{"S": "42"}`)))
	require.NoError(t, err)
	require.Equal(t, resp.StatusCode, http.StatusOK)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&testMsg{}))

	// wrong content type
	resp, err = c.Post(addr+"/v3/testService/restMsgPOSTString", "application/text", bytes.NewReader([]byte(`{"S": "42"}`)))
	require.NoError(t, err)
	require.Equal(t, resp.StatusCode, http.StatusBadRequest)
	checkJSONMsg(t, resp.Body, "content type needs to be application/json")

	// wrong value in body
	resp, err = c.Post(addr+"/v3/testService/restMsgPOSTString", "application/json", bytes.NewReader([]byte(`{"S": "43"}`)))
	require.NoError(t, err)
	require.Equal(t, resp.StatusCode, http.StatusBadRequest)
	checkJSONMsg(t, resp.Body, "processing error")

	// wrong field name
	resp, err = c.Post(addr+"/v3/testService/restMsgPOSTString", "application/json", bytes.NewReader([]byte(`{"T": "42"}`)))
	require.NoError(t, err)
	require.Equal(t, resp.StatusCode, http.StatusBadRequest)
	checkJSONMsg(t, resp.Body, "processing error")

	// wrong method
	resp, err = c.Get(addr + "/v3/testService/restMsgPOSTString")
	require.NoError(t, err)
	require.Equal(t, resp.StatusCode, http.StatusMethodNotAllowed)
	checkJSONMsg(t, resp.Body, "unsupported method")

	// test sending points
	_, pk := bls.NewKeyPair(bn256.NewSuite(), random.New())
	buf, err := json.Marshal(&restMsgPOSTPoint{bnPoint{pk}})
	require.NoError(t, err)
	resp, err = c.Post(addr+"/v3/testService/restMsgPOSTPoint", "application/json", bytes.NewReader(buf))
	require.NoError(t, err)
	require.Equal(t, resp.StatusCode, http.StatusOK)
	respPoint := restMsgPOSTPoint{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&respPoint))
	require.True(t, respPoint.bnPoint.P.Equal(pk))
}

func checkJSONMsg(t *testing.T, r io.Reader, contains string) {
	s, err := ioutil.ReadAll(r)
	require.NoError(t, err)
	type msg struct {
		Message string `json:"message"`
	}
	var m msg
	require.NoError(t, json.Unmarshal(s, &m))
	require.NotEmpty(t, m.Message)
	require.Contains(t, m.Message, contains)
}

func procRestMsgGET1(s *restMsgGET1) (*testMsg, error) {
	return &testMsg{42}, nil
}

func procRestMsgGET2(s *restMsgGET2) (*testMsg, error) {
	return &testMsg{int64(s.X)}, nil
}

func procRestMsgGET3(s *restMsgGET3) (*testMsg, error) {
	return &testMsg{int64(s.Xs[0])}, nil
}

func procRestMsgGETWrong1(s *restMsgGETWrong1) (*testMsg, error) {
	return &testMsg{}, nil
}

func procRestMsgGETWrong2(s *restMsgGETWrong2) (*testMsg, error) {
	return &testMsg{}, nil
}

func procRestMsgGETWrong3(s *restMsgGETWrong3) (*testMsg, error) {
	return &testMsg{}, nil
}

func procRestMsgPOSTString(s *restMsgPOSTString) (*testMsg, error) {
	if s.S != "42" {
		return nil, xerrors.New("not the right answer")
	}
	return &testMsg{}, nil
}

func procRestMsgPOSTPoint(s *restMsgPOSTPoint) (*restMsgPOSTPoint, error) {
	return &restMsgPOSTPoint{s.bnPoint}, nil
}

type restMsgGET1 struct {
}

type restMsgGET2 struct {
	X int
}

type restMsgGET3 struct {
	Xs []byte
}

type restMsgGETWrong1 struct {
	X float64
}

type restMsgGETWrong2 struct {
	X float64
	Y float64
}

type restMsgGETWrong3 struct {
	Xs []int
}

type restMsgPOSTString struct {
	S string
}

type restMsgPOSTPoint struct {
	bnPoint
}
