package onet

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"net/http"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/onet/v3/log"
	"go.dedis.ch/onet/v3/network"
	"go.dedis.ch/protobuf"
	"golang.org/x/xerrors"
)

func TestClient_Send(t *testing.T) {
	local := NewTCPTest(tSuite)
	defer local.CloseAll()

	// register service
	RegisterNewService(backForthServiceName, func(c *Context) (Service, error) {
		return &simpleService{
			ctx: c,
		}, nil
	})
	defer ServiceFactory.Unregister(backForthServiceName)

	// create servers
	servers, el, _ := local.GenTree(4, false)
	client := local.NewClient(backForthServiceName)

	r := &SimpleRequest{
		ServerIdentities: el,
		Val:              10,
	}
	sr := &SimpleResponse{}
	require.Equal(t, uint64(0), client.Rx())
	require.Equal(t, uint64(0), client.Tx())
	require.Nil(t, client.SendProtobuf(servers[0].ServerIdentity, r, sr))
	require.Equal(t, sr.Val, int64(10))
	require.NotEqual(t, uint64(0), client.Rx())
	require.NotEqual(t, uint64(0), client.Tx())
	require.True(t, client.Tx() > client.Rx())
}

func TestClientTLS_Send(t *testing.T) {
	cert, key, err := getSelfSignedCertificateAndKey()
	require.Nil(t, err)
	CAPool := x509.NewCertPool()
	CAPool.AppendCertsFromPEM(cert)

	local := NewTCPTest(tSuite)
	local.webSocketTLSCertificate = cert
	local.webSocketTLSCertificateKey = key
	defer local.CloseAll()

	// register service
	RegisterNewService(backForthServiceName, func(c *Context) (Service, error) {
		return &simpleService{
			ctx: c,
		}, nil
	})
	defer ServiceFactory.Unregister(backForthServiceName)

	// create servers
	servers, el, _ := local.GenTree(4, false)
	client := local.NewClient(backForthServiceName)
	client.TLSClientConfig = &tls.Config{RootCAs: CAPool}

	r := &SimpleRequest{
		ServerIdentities: el,
		Val:              10,
	}
	sr := &SimpleResponse{}
	require.Equal(t, uint64(0), client.Rx())
	require.Equal(t, uint64(0), client.Tx())
	require.Nil(t, client.SendProtobuf(servers[0].ServerIdentity, r, sr))
	require.Equal(t, sr.Val, int64(10))
	require.NotEqual(t, uint64(0), client.Rx())
	require.NotEqual(t, uint64(0), client.Tx())
	require.True(t, client.Tx() > client.Rx())
}

func TestClientTLS_certfile_Send(t *testing.T) {
	// like TestClientTLSfile_Send, but uses cert and key from a file
	// to solve issue 583.
	cert, key, err := getSelfSignedCertificateAndKey()
	require.Nil(t, err)
	CAPool := x509.NewCertPool()
	CAPool.AppendCertsFromPEM(cert)

	f1, err := ioutil.TempFile("", "cert")
	require.NoError(t, err)
	defer os.Remove(f1.Name())
	f1.Write(cert)
	f1.Close()

	f2, err := ioutil.TempFile("", "key")
	require.NoError(t, err)
	defer os.Remove(f2.Name())
	f2.Write(key)
	f2.Close()

	local := NewTCPTest(tSuite)
	local.webSocketTLSCertificate = []byte(f1.Name())
	local.webSocketTLSCertificateKey = []byte(f2.Name())
	local.webSocketTLSReadFiles = true
	defer local.CloseAll()

	// register service
	RegisterNewService(backForthServiceName, func(c *Context) (Service, error) {
		return &simpleService{
			ctx: c,
		}, nil
	})
	defer ServiceFactory.Unregister(backForthServiceName)

	// create servers
	servers, el, _ := local.GenTree(4, false)
	client := local.NewClient(backForthServiceName)
	client.TLSClientConfig = &tls.Config{RootCAs: CAPool}

	r := &SimpleRequest{
		ServerIdentities: el,
		Val:              10,
	}
	sr := &SimpleResponse{}
	require.Equal(t, uint64(0), client.Rx())
	require.Equal(t, uint64(0), client.Tx())
	require.Nil(t, client.SendProtobuf(servers[0].ServerIdentity, r, sr))
	require.Equal(t, sr.Val, int64(10))
	require.NotEqual(t, uint64(0), client.Rx())
	require.NotEqual(t, uint64(0), client.Tx())
	require.True(t, client.Tx() > client.Rx())
}

func TestClient_Parallel(t *testing.T) {
	nbrNodes := 4
	nbrParallel := 20
	local := NewTCPTest(tSuite)
	defer local.CloseAll()

	// register service
	RegisterNewService(backForthServiceName, func(c *Context) (Service, error) {
		return &simpleService{
			ctx: c,
		}, nil
	})
	defer ServiceFactory.Unregister(backForthServiceName)

	// create servers
	servers, el, _ := local.GenTree(nbrNodes, true)

	wg := sync.WaitGroup{}
	wg.Add(nbrParallel)
	for i := 0; i < nbrParallel; i++ {
		go func(i int) {
			defer wg.Done()
			log.Lvl1("Starting message", i)
			r := &SimpleRequest{
				ServerIdentities: el,
				Val:              int64(10 * i),
			}
			client := local.NewClient(backForthServiceName)
			sr := &SimpleResponse{}
			err := client.SendProtobuf(servers[0].ServerIdentity, r, sr)
			require.Nil(t, err)
			require.Equal(t, int64(10*i), sr.Val)
			log.Lvl1("Done with message", i)
		}(i)
	}
	wg.Wait()
}

func TestClientTLS_Parallel(t *testing.T) {
	cert, key, err := getSelfSignedCertificateAndKey()
	require.Nil(t, err)
	CAPool := x509.NewCertPool()
	CAPool.AppendCertsFromPEM(cert)

	nbrNodes := 4
	nbrParallel := 20
	local := NewTCPTest(tSuite)
	local.webSocketTLSCertificate = cert
	local.webSocketTLSCertificateKey = key
	defer local.CloseAll()

	// register service
	RegisterNewService(backForthServiceName, func(c *Context) (Service, error) {
		return &simpleService{
			ctx: c,
		}, nil
	})
	defer ServiceFactory.Unregister(backForthServiceName)

	// create servers
	servers, el, _ := local.GenTree(nbrNodes, true)

	wg := sync.WaitGroup{}
	wg.Add(nbrParallel)
	for i := 0; i < nbrParallel; i++ {
		go func(i int) {
			defer wg.Done()
			log.Lvl1("Starting message", i)
			r := &SimpleRequest{
				ServerIdentities: el,
				Val:              int64(10 * i),
			}
			client := local.NewClient(backForthServiceName)
			client.TLSClientConfig = &tls.Config{RootCAs: CAPool}
			sr := &SimpleResponse{}
			require.Nil(t, client.SendProtobuf(servers[0].ServerIdentity, r, sr))
			require.Equal(t, int64(10*i), sr.Val)
			log.Lvl1("Done with message", i)
		}(i)
	}
	wg.Wait()
}

func TestNewClientKeep(t *testing.T) {
	c := NewClientKeep(tSuite, serviceWebSocket)
	require.True(t, c.keep)
}

func TestMultiplePath(t *testing.T) {
	_, err := RegisterNewService(dummyService3Name, func(c *Context) (Service, error) {
		ds := &DummyService3{}
		return ds, nil
	})
	require.Nil(t, err)
	defer UnregisterService(dummyService3Name)

	local := NewTCPTest(tSuite)
	hs := local.GenServers(2)
	server := hs[0]
	defer local.CloseAll()
	client := NewClientKeep(tSuite, dummyService3Name)
	msg, err := protobuf.Encode(&DummyMsg{})
	require.Nil(t, err)
	path1, path2 := "path1", "path2"
	resp, err := client.Send(server.ServerIdentity, path1, msg)
	require.Nil(t, err)
	require.Equal(t, path1, string(resp))
	resp, err = client.Send(server.ServerIdentity, path2, msg)
	require.Nil(t, err)
	require.Equal(t, path2, string(resp))
}

func TestMultiplePathTLS(t *testing.T) {
	cert, key, err := getSelfSignedCertificateAndKey()
	require.Nil(t, err)
	CAPool := x509.NewCertPool()
	CAPool.AppendCertsFromPEM(cert)

	_, err = RegisterNewService(dummyService3Name, func(c *Context) (Service, error) {
		ds := &DummyService3{}
		return ds, nil
	})
	require.Nil(t, err)
	defer UnregisterService(dummyService3Name)

	local := NewTCPTest(tSuite)
	local.webSocketTLSCertificate = cert
	local.webSocketTLSCertificateKey = key
	hs := local.GenServers(2)
	server := hs[0]
	defer local.CloseAll()
	client := NewClientKeep(tSuite, dummyService3Name)
	client.TLSClientConfig = &tls.Config{RootCAs: CAPool}
	msg, err := protobuf.Encode(&DummyMsg{})
	require.Nil(t, err)
	path1, path2 := "path1", "path2"
	resp, err := client.Send(server.ServerIdentity, path1, msg)
	require.Nil(t, err)
	require.Equal(t, path1, string(resp))
	resp, err = client.Send(server.ServerIdentity, path2, msg)
	require.Nil(t, err)
	require.Equal(t, path2, string(resp))
}

func TestWebSocket_Error(t *testing.T) {
	client := NewClientKeep(tSuite, dummyService3Name)
	local := NewTCPTest(tSuite)
	hs := local.GenServers(2)
	server := hs[0]
	defer local.CloseAll()

	lvl := log.DebugVisible()
	log.SetDebugVisible(0)
	log.OutputToBuf()
	_, err := client.Send(server.ServerIdentity, "test", nil)
	log.OutputToOs()
	log.SetDebugVisible(lvl)
	require.NotEqual(t, websocket.ErrBadHandshake, err)
	require.NotEqual(t, "", log.GetStdErr())
}

func TestWebSocketTLS_Error(t *testing.T) {
	cert, key, err := getSelfSignedCertificateAndKey()
	require.Nil(t, err)
	CAPool := x509.NewCertPool()
	CAPool.AppendCertsFromPEM(cert)

	client := NewClientKeep(tSuite, dummyService3Name)
	client.TLSClientConfig = &tls.Config{RootCAs: CAPool}
	local := NewTCPTest(tSuite)
	local.webSocketTLSCertificate = cert
	local.webSocketTLSCertificateKey = key
	hs := local.GenServers(2)
	server := hs[0]
	defer local.CloseAll()

	lvl := log.DebugVisible()
	log.SetDebugVisible(0)
	log.OutputToBuf()
	_, err = client.Send(server.ServerIdentity, "test", nil)
	log.OutputToOs()
	log.SetDebugVisible(lvl)
	require.NotEqual(t, websocket.ErrBadHandshake, err)
	require.NotEqual(t, "", log.GetStdErr())
}

// TestWebSocket_Streaming_normal reads all messages from the service
func TestWebSocket_Streaming_normal(t *testing.T) {
	local := NewTCPTest(tSuite)
	defer local.CloseAll()

	serName := "streamingService"
	_, err := RegisterNewService(serName, newStreamingService)
	require.NoError(t, err)
	defer UnregisterService(serName)

	servers, el, _ := local.GenTree(4, false)
	client := local.NewClientKeep(serName)

	n := 5
	r := &SimpleRequest{
		ServerIdentities: el,
		Val:              int64(n),
	}

	log.Lvl1("Happy-path testing")
	conn, err := client.Stream(servers[0].ServerIdentity, r)
	require.NoError(t, err)

	for i := 0; i < n; i++ {
		sr := &SimpleResponse{}
		require.NoError(t, conn.ReadMessage(sr))
		require.Equal(t, sr.Val, int64(n))
	}

	// Using the same client (connection) to repeat the same request should
	// fail because the connection should be closed by the service when
	// there are no more messages.
	log.Lvl1("Fail on re-use")
	sr := &SimpleResponse{}
	require.Error(t, conn.ReadMessage(sr))
	require.NoError(t, client.Close())
}

// TestWebSocket_Streaming_Parallel_normal
func TestWebSocket_Streaming_Parallel_normal(t *testing.T) {
	local := NewTCPTest(tSuite)
	defer local.CloseAll()

	serName := "streamingService"
	_, err := RegisterNewService(serName, newStreamingService)
	require.NoError(t, err)
	defer UnregisterService(serName)

	servers, el, _ := local.GenTree(4, false)
	n := 10

	// Do streaming with 10 clients in parallel. Happy-path where clients read
	// everything.
	clients := make([]*Client, 100)
	for i := range clients {
		clients[i] = local.NewClientKeep(serName)
	}
	var wg sync.WaitGroup
	for _, client := range clients {
		wg.Add(1)
		go func(c *Client) {
			defer wg.Done()
			r := &SimpleRequest{
				ServerIdentities: el,
				Val:              int64(n),
			}
			conn, err := c.Stream(servers[0].ServerIdentity, r)
			require.NoError(t, err)

			for i := 0; i < n; i++ {
				sr := &SimpleResponse{}
				require.NoError(t, conn.ReadMessage(sr))
				require.Equal(t, sr.Val, int64(n))
			}
		}(client)
	}
	wg.Wait()
	for i := range clients {
		require.NoError(t, clients[i].Close())
	}
}

// TestWebSocket_Streaming_bi_normal sends multiple messages from the clients
// and reads all the messages
func TestWebSocket_Streaming_bi_normal(t *testing.T) {
	local := NewTCPTest(tSuite)
	defer local.CloseAll()

	serviceStruct := struct {
		once      sync.Once
		outChan   chan *SimpleResponse
		closeChan chan bool
	}{
		outChan:   make(chan *SimpleResponse, 10),
		closeChan: make(chan bool),
	}

	h := func(m *SimpleRequest) (chan *SimpleResponse, chan bool, error) {
		go func() {
			for i := 0; i < int(m.Val); i++ {
				time.Sleep(100 * time.Millisecond)
				serviceStruct.outChan <- &SimpleResponse{int64(i)}
			}
			<-serviceStruct.closeChan
			serviceStruct.once.Do(func() {
				close(serviceStruct.outChan)
			})
		}()
		return serviceStruct.outChan, serviceStruct.closeChan, nil
	}

	newCustomStreamingService := func(c *Context) (Service, error) {
		s := &StreamingService{
			ServiceProcessor: NewServiceProcessor(c),
			stopAt:           -1,
		}
		if err := s.RegisterStreamingHandler(h); err != nil {
			panic(err.Error())
		}
		return s, nil
	}
	serName := "biStreamingService"
	_, err := RegisterNewService(serName, newCustomStreamingService)
	require.NoError(t, err)
	defer UnregisterService(serName)

	servers, el, _ := local.GenTree(4, false)
	client := local.NewClientKeep(serName)

	// A first request to the service
	n := 5
	r := &SimpleRequest{
		ServerIdentities: el,
		Val:              int64(n),
	}

	conn, err := client.Stream(servers[0].ServerIdentity, r)
	require.NoError(t, err)

	for i := 0; i < n; i++ {
		sr := &SimpleResponse{}
		require.NoError(t, conn.ReadMessage(sr))
		require.Equal(t, sr.Val, int64(i))
	}

	// Lets perform a second request
	n = 5
	r = &SimpleRequest{
		ServerIdentities: el,
		Val:              int64(n),
	}

	conn, err = client.Stream(servers[0].ServerIdentity, r)
	require.NoError(t, err)

	for i := 0; i < n; i++ {
		sr := &SimpleResponse{}
		require.NoError(t, conn.ReadMessage(sr))
		require.Equal(t, sr.Val, int64(i))
	}

	client.Close()
	time.Sleep(time.Second)
}

// TestWebSocket_Streaming_early_client makes the client close early.
func TestWebSocket_Streaming_early_client(t *testing.T) {
	local := NewTCPTest(tSuite)
	defer local.CloseAll()

	serName := "streamingService"
	serID, err := RegisterNewService(serName, newStreamingService)
	require.NoError(t, err)
	defer UnregisterService(serName)

	servers, el, _ := local.GenTree(4, false)
	client := local.NewClientKeep(serName)

	n := 5
	r := &SimpleRequest{
		ServerIdentities: el,
		Val:              int64(n),
	}

	// go-routine should also terminate.
	client = local.NewClientKeep(serName)
	services := local.GetServices(servers, serID)
	serviceRoot := services[0].(*StreamingService)
	serviceRoot.gotStopChan = make(chan bool, 1)

	_, err = client.Stream(servers[0].ServerIdentity, r)
	require.NoError(t, err)
	require.NoError(t, client.Close())

	select {
	case <-serviceRoot.gotStopChan:
	case <-time.After(time.Second):
		require.Fail(t, "should have got an early finish signal")
	}

}

// TestWebSocket_Streaming_Parallel_early_client
func TestWebSocket_Streaming_Parallel_early_client2(t *testing.T) {
	local := NewTCPTest(tSuite)
	defer local.CloseAll()

	serName := "streamingService"
	serID, err := RegisterNewService(serName, newStreamingService)
	require.NoError(t, err)
	defer UnregisterService(serName)

	servers, el, _ := local.GenTree(4, false)
	services := local.GetServices(servers, serID)
	serviceRoot := services[0].(*StreamingService)
	n := 10

	// Unhappy-path where clients stop early.
	clients := make([]*Client, 100)
	for i := range clients {
		clients[i] = local.NewClientKeep(serName)
	}
	serviceRoot.gotStopChan = make(chan bool, len(clients))
	wg := sync.WaitGroup{}
	for _, client := range clients {
		wg.Add(1)
		go func(c *Client) {
			defer wg.Done()
			r := &SimpleRequest{
				ServerIdentities: el,
				Val:              int64(n),
			}
			conn, err := c.Stream(servers[0].ServerIdentity, r)
			require.NoError(t, err)

			// read one message instead of n then close
			sr := &SimpleResponse{}
			require.NoError(t, conn.ReadMessage(sr))
			require.Equal(t, sr.Val, int64(n))
			require.NoError(t, c.Close())
		}(client)
	}

	// we should get close messages
	for i := 0; i < len(clients); i++ {
		select {
		case <-serviceRoot.gotStopChan:
		case <-time.After(time.Second):
			require.Fail(t, "should have got an early finish signal")
		}
	}
	wg.Wait()
}

// TestWebSocket_Streaming_early_service closes the service early
func TestWebSocket_Streaming_early_service(t *testing.T) {
	local := NewTCPTest(tSuite)
	defer local.CloseAll()

	serName := "streamingService"
	serID, err := RegisterNewService(serName, newStreamingService)
	require.NoError(t, err)
	defer UnregisterService(serName)

	servers, el, _ := local.GenTree(4, false)
	client := local.NewClientKeep(serName)

	n := 5
	r := &SimpleRequest{
		ServerIdentities: el,
		Val:              int64(n),
	}

	// Have the service terminate early. The client should stop receiving
	// messages.
	log.Lvl1("Service terminate early")
	stopAt := 1
	client = local.NewClientKeep(serName)
	services := local.GetServices(servers, serID)
	serviceRoot := services[0].(*StreamingService)
	serviceRoot.stopAt = stopAt

	conn, err := client.Stream(servers[0].ServerIdentity, r)
	require.NoError(t, err)
	for i := 0; i < n; i++ {
		if i > stopAt {
			sr := &SimpleResponse{}
			require.Error(t, conn.ReadMessage(sr))
		} else {
			sr := &SimpleResponse{}
			require.NoError(t, conn.ReadMessage(sr))
			require.Equal(t, sr.Val, int64(n))
		}
	}
	require.NoError(t, client.Close())
}

// TestWebSocket_Streaming_Parallel_early_service
func TestWebSocket_Streaming_Parallel_ealry_service(t *testing.T) {
	local := NewTCPTest(tSuite)
	defer local.CloseAll()

	serName := "streamingService"
	serID, err := RegisterNewService(serName, newStreamingService)
	require.NoError(t, err)
	defer UnregisterService(serName)

	servers, el, _ := local.GenTree(4, false)
	services := local.GetServices(servers, serID)
	serviceRoot := services[0].(*StreamingService)
	n := 10

	// The other unhappy-path where the service stops early.
	clients := make([]*Client, 100)
	for i := range clients {
		clients[i] = local.NewClientKeep(serName)
	}
	stopAt := 1
	serviceRoot.stopAt = stopAt
	wg := sync.WaitGroup{}
	for _, client := range clients {
		wg.Add(1)
		go func(c *Client) {
			defer wg.Done()
			r := &SimpleRequest{
				ServerIdentities: el,
				Val:              int64(n),
			}
			conn, err := c.Stream(servers[0].ServerIdentity, r)
			require.NoError(t, err)

			for i := 0; i < n; i++ {
				if i > stopAt {
					sr := &SimpleResponse{}
					require.Error(t, conn.ReadMessage(sr))
				} else {
					sr := &SimpleResponse{}
					require.NoError(t, conn.ReadMessage(sr))
					require.Equal(t, sr.Val, int64(n))
				}
			}
		}(client)
	}
	wg.Wait()
	for i := range clients {
		require.NoError(t, clients[i].Close())
	}
}

// Tests the correct returning of values depending on the ParallelOptions structure
func TestParallelOptions_GetList(t *testing.T) {
	l := NewLocalTest(tSuite)
	defer l.CloseAll()

	var po *ParallelOptions
	_, roster, _ := l.GenTree(3, false)
	nodes := roster.List

	count, list := po.GetList(nodes)
	require.Equal(t, 2, count)
	require.Equal(t, 3, len(list))
	require.False(t, po.Quit())

	po = &ParallelOptions{}
	count, list = po.GetList(nodes)
	require.Equal(t, 2, count)
	require.Equal(t, 3, len(list))
	require.False(t, po.Quit())

	first := 0
	for i := 0; i < 32; i++ {
		_, list := po.GetList(nodes)
		if (<-list).Equal(nodes[0]) {
			first++
		}
	}
	require.NotEqual(t, 0, first)
	require.NotEqual(t, 32, first)
	po.DontShuffle = true
	first = 0
	for i := 0; i < 32; i++ {
		_, list := po.GetList(nodes)
		if (<-list).Equal(nodes[0]) {
			first++
		}
	}
	require.Equal(t, 32, first)

	po.IgnoreNodes = append(po.IgnoreNodes, nodes[0])
	count, list = po.GetList(nodes)
	require.Equal(t, 2, count)
	require.Equal(t, 2, len(list))

	po.IgnoreNodes = append(po.IgnoreNodes, nodes[1])
	count, list = po.GetList(nodes)
	require.Equal(t, 2, count)
	require.Equal(t, 1, len(list))

	po.IgnoreNodes = po.IgnoreNodes[0:1]
	po.QuitError = true
	require.True(t, po.Quit())

	po.AskNodes = 1
	count, list = po.GetList(nodes)
	require.Equal(t, 1, count)
	require.Equal(t, 1, len(list))

	po.StartNode = 1
	count, list = po.GetList(nodes)
	require.Equal(t, 1, count)
	require.Equal(t, 1, len(list))
}

func TestClient_SendProtobufParallel(t *testing.T) {
	l := NewLocalTest(tSuite)
	defer l.CloseAll()

	servers, roster, _ := l.GenTree(3, false)
	cl := NewClient(tSuite, serviceWebSocket)
	tests := 10
	firstNodes := make([]*network.ServerIdentity, tests)
	for i := 0; i < tests; i++ {
		log.Lvl1("Sending", i)
		var err error
		firstNodes[i], err = cl.SendProtobufParallel(roster.List, &SimpleResponse{}, nil, nil)
		require.Nil(t, err)
	}

	for flags := 0; flags < 8; flags++ {
		log.Lvl1("Count errors over all services with error-flags", flags)
		_, err := cl.SendProtobufParallel(roster.List, &ErrorRequest{
			Roster: *roster,
			Flags:  flags,
		}, nil, nil)
		if flags == 7 {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}
		// Need to close here to make sure that all messages are being sent
		// before going on to the next stage.
		require.NoError(t, cl.Close())
	}
	var errs int
	for _, server := range servers {
		errs += server.Service(serviceWebSocket).(*ServiceWebSocket).Errors
	}
	require.Equal(t, 3, errs)

	sort.Slice(firstNodes, func(i, j int) bool {
		return bytes.Compare(firstNodes[i].ID[:], firstNodes[j].ID[:]) < 0
	})
	require.False(t, firstNodes[0].Equal(firstNodes[tests-1]))
}

func TestClient_SendProtobufParallelWithDecoder(t *testing.T) {
	l := NewLocalTest(tSuite)
	defer l.CloseAll()

	_, roster, _ := l.GenTree(3, false)
	cl := NewClient(tSuite, serviceWebSocket)

	decoderWithError := func(data []byte, ret interface{}) error {
		// As an example, the decoder should first decode the response, and it can then make
		// further verification like the latest block index.
		return xerrors.New("decoder error")
	}

	_, err := cl.SendProtobufParallelWithDecoder(roster.List, &SimpleResponse{}, &SimpleResponse{}, nil, decoderWithError)
	require.Error(t, err)
	require.Contains(t, err.Error(), "decoder error")

	decoderNoError := func(data []byte, ret interface{}) error {
		return nil
	}

	_, err = cl.SendProtobufParallelWithDecoder(roster.List, &SimpleResponse{}, &SimpleResponse{}, nil, decoderNoError)
	require.NoError(t, err)
}

const dummyService3Name = "dummyService3"

type DummyService3 struct {
}

func (ds *DummyService3) ProcessClientRequest(req *http.Request, path string, buf []byte) ([]byte, *StreamingTunnel, error) {
	log.Lvl2("Got called with path", path, buf)
	return []byte(path), nil, nil
}

func (ds *DummyService3) NewProtocol(tn *TreeNodeInstance, conf *GenericConfig) (ProtocolInstance, error) {
	return nil, nil
}

func (ds *DummyService3) Process(env *network.Envelope) {
}

type StreamingService struct {
	*ServiceProcessor
	stopAt      int
	gotStopChan chan bool
}

func newStreamingService(c *Context) (Service, error) {
	s := &StreamingService{
		ServiceProcessor: NewServiceProcessor(c),
		stopAt:           -1,
	}
	if err := s.RegisterStreamingHandler(s.StreamValues); err != nil {
		panic(err.Error())
	}
	return s, nil
}

func (ss *StreamingService) StreamValues(msg *SimpleRequest) (chan *SimpleResponse, chan bool, error) {
	streamingChan := make(chan *SimpleResponse)
	stopChan := make(chan bool)
	go func() {
	outer:
		for i := 0; i < int(msg.Val); i++ {
			// Add some delay between every message so that we can
			// actually catch the stop signal before everything is
			// sent out.
			time.Sleep(100 * time.Millisecond)
			select {
			case <-stopChan:
				ss.gotStopChan <- true
				break outer
			default:
				streamingChan <- &SimpleResponse{msg.Val}
			}
			if ss.stopAt == i {
				break outer
			}
		}
		close(streamingChan)
	}()
	return streamingChan, stopChan, nil
}
