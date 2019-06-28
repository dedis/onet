package onet

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io/ioutil"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/onet/v3/log"
	"go.dedis.ch/onet/v3/network"
	"go.dedis.ch/protobuf"
	uuid "gopkg.in/satori/go.uuid.v1"
)

func init() {
	RegisterNewService(serviceWebSocket, newServiceWebSocket)
}

// Adapted from 'https://golang.org/src/crypto/tls/generate_cert.go'
func generateSelfSignedCert() (string, string, error) {
	// Hostname or IP to generate a certificate for
	host := "127.0.0.1"
	// Creation date formatted as Jan 2 15:04:05 2006
	validFrom := time.Now().UTC().Format("Jan 2 15:04:05 2006")
	// Duration that certificate is valid for
	validFor := 365 * 24 * time.Hour

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}

	notBefore, err := time.Parse("Jan 2 15:04:05 2006", validFrom)
	if err != nil {
		return "", "", err
	}

	notAfter := notBefore.Add(validFor)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return "", "", err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"DEDIS EPFL"},
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	if ip := net.ParseIP(host); ip != nil {
		template.IPAddresses = append(template.IPAddresses, ip)
	} else {
		template.DNSNames = append(template.DNSNames, host)
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return "", "", err
	}

	certOut, err := ioutil.TempFile("", "cert.pem")
	if err != nil {
		return "", "", err
	}
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	certOut.Close()

	keyOut, err := ioutil.TempFile("", "key.pem")
	if err != nil {
		return "", "", err
	}

	b, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return "", "", err
	}
	pemBlockForKey := &pem.Block{Type: "EC PRIVATE KEY", Bytes: b}

	pem.Encode(keyOut, pemBlockForKey)
	keyOut.Close()

	return certOut.Name(), keyOut.Name(), nil
}

func getSelfSignedCertificateAndKey() ([]byte, []byte, error) {
	certFilePath, keyFilePath, err := generateSelfSignedCert()
	if err != nil {
		return nil, nil, err
	}

	cert, err := ioutil.ReadFile(certFilePath)
	if err != nil {
		return nil, nil, err
	}
	err = os.Remove(certFilePath)
	if err != nil {
		return nil, nil, err
	}

	key, err := ioutil.ReadFile(keyFilePath)
	if err != nil {
		return nil, nil, err
	}
	err = os.Remove(keyFilePath)
	if err != nil {
		return nil, nil, err
	}

	return cert, key, nil
}

func TestNewWebSocket(t *testing.T) {
	l := NewLocalTest(tSuite)
	defer l.CloseAll()

	c := newTCPServer(tSuite, 0, l.path)
	c.StartInBackground()

	defer c.Close()
	require.Equal(t, len(c.serviceManager.services), len(c.WebSocket.services))
	require.NotEmpty(t, c.WebSocket.services[serviceWebSocket])
	cl := NewClientKeep(tSuite, "WebSocket")
	req := &SimpleResponse{}
	log.Lvlf1("Sending message Request: %x", uuid.UUID(network.MessageType(req)).Bytes())
	buf, err := protobuf.Encode(req)
	require.Nil(t, err)
	rcv, err := cl.Send(c.ServerIdentity, "SimpleResponse", buf)
	require.Nil(t, err)

	log.Lvlf1("Received reply: %x", rcv)
	rcvMsg := &SimpleResponse{}
	require.Nil(t, protobuf.Decode(rcv, rcvMsg))
	require.Equal(t, int64(1), rcvMsg.Val)
}

func TestNewWebSocketTLS(t *testing.T) {
	cert, key, err := getSelfSignedCertificateAndKey()
	require.Nil(t, err)
	CAPool := x509.NewCertPool()
	CAPool.AppendCertsFromPEM(cert)

	l := NewLocalTest(tSuite)
	defer l.CloseAll()

	c := newTCPServer(tSuite, 0, l.path)
	defer c.Close()

	certToAdd, err := tls.X509KeyPair(cert, key)
	if err != nil {
		require.Nil(t, err)
	}
	c.WebSocket.Lock()
	c.WebSocket.TLSConfig = &tls.Config{Certificates: []tls.Certificate{certToAdd}}
	c.WebSocket.Unlock()
	c.StartInBackground()

	require.Equal(t, len(c.serviceManager.services), len(c.WebSocket.services))
	require.NotEmpty(t, c.WebSocket.services[serviceWebSocket])

	// Test the traditional host:port+1 way of specifying the websocket server.
	cl := NewClientKeep(tSuite, "WebSocket")
	defer cl.Close()
	cl.TLSClientConfig = &tls.Config{RootCAs: CAPool}
	req := &SimpleResponse{}
	log.Lvlf1("Sending message Request: %x", uuid.UUID(network.MessageType(req)).Bytes())
	buf, err := protobuf.Encode(req)
	require.Nil(t, err)
	rcv, err := cl.Send(c.ServerIdentity, "SimpleResponse", buf)
	log.Lvlf1("Received reply: %x", rcv)
	rcvMsg := &SimpleResponse{}
	require.Nil(t, protobuf.Decode(rcv, rcvMsg))
	require.Equal(t, int64(1), rcvMsg.Val)

	// Set c.ServerIdentity.URL, in order to test the other way of triggering wss:// connection.
	hp, err := getWSHostPort(c.ServerIdentity, false)
	require.NoError(t, err)
	u := &url.URL{Scheme: "https", Host: hp}
	c.ServerIdentity.URL = u.String()

	log.Lvlf1("Sending message Request: %x", uuid.UUID(network.MessageType(req)).Bytes())
	rcv, err = cl.Send(c.ServerIdentity, "SimpleResponse", buf)
	log.Lvlf1("Received reply: %x", rcv)
	require.Nil(t, protobuf.Decode(rcv, rcvMsg))
	require.Equal(t, int64(1), rcvMsg.Val)
}

func TestGetWebHost(t *testing.T) {
	url, err := getWSHostPort(&network.ServerIdentity{Address: "tcp://8.8.8.8"}, true)
	require.NotNil(t, err)
	url, err = getWSHostPort(&network.ServerIdentity{Address: "tcp://8.8.8.8"}, false)
	require.NotNil(t, err)
	url, err = getWSHostPort(&network.ServerIdentity{Address: "tcp://8.8.8.8:7770"}, true)
	require.Nil(t, err)
	require.Equal(t, "0.0.0.0:7771", url)
	url, err = getWSHostPort(&network.ServerIdentity{Address: "tcp://8.8.8.8:7770"}, false)
	require.Nil(t, err)
	require.Equal(t, "8.8.8.8:7771", url)
}

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
	require.NotEqual(t, "websocket: bad handshake", err.Error())
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
	require.NotEqual(t, "websocket: bad handshake", err.Error())
	require.NotEqual(t, "", log.GetStdErr())
}

// TestWebSocket_Streaming performs 3 test cases.
// (1) happy-path, where client reads all messages from the service
// (2) unhappy-path, where client closes early
// (3) unhappy-path, where service closes early
func TestWebSocket_Streaming(t *testing.T) {
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

	// (1) happy-path testing
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
	sr := &SimpleResponse{}
	require.Error(t, conn.ReadMessage(sr))
	require.NoError(t, client.Close())

	// (2) This time, have the client terminate early, the service's
	// go-routine should also terminate.
	client = local.NewClientKeep(serName)
	services := local.GetServices(servers, serID)
	serviceRoot := services[0].(*StreamingService)
	serviceRoot.gotStopChan = make(chan bool, 1)

	conn, err = client.Stream(servers[0].ServerIdentity, r)
	require.NoError(t, err)
	require.NoError(t, client.Close())

	select {
	case <-serviceRoot.gotStopChan:
	case <-time.After(time.Second):
		require.Fail(t, "should have got an early finish signal")
	}

	// (3) Finally, have the service terminate early. The client should
	// stop receiving messages.
	stopAt := 1
	serviceRoot.stopAt = stopAt
	client = local.NewClientKeep(serName)

	conn, err = client.Stream(servers[0].ServerIdentity, r)
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

// TestWebSocket_Streaming_Parallel is essentially the same as
// TestWebSocket_Streaming, except we do it in parallel.
func TestWebSocket_Streaming_Parallel(t *testing.T) {
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

	// (1) We try to do streaming with 10 clients in parallel. Start with
	// the happy-path where clients read everything.
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

	// (2) Now try the unhappy-path where clients stop early.
	for i := range clients {
		clients[i] = local.NewClientKeep(serName)
	}
	serviceRoot.gotStopChan = make(chan bool, len(clients))
	wg = sync.WaitGroup{}
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

	// (3) The other unhappy-path where the service stops early.
	for i := range clients {
		clients[i] = local.NewClientKeep(serName)
	}
	stopAt := 1
	serviceRoot.stopAt = stopAt
	wg = sync.WaitGroup{}
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

const serviceWebSocket = "WebSocket"

type ServiceWebSocket struct {
	*ServiceProcessor
}

func (i *ServiceWebSocket) SimpleResponse(msg *SimpleResponse) (network.Message, error) {
	return &SimpleResponse{msg.Val + 1}, nil
}

func newServiceWebSocket(c *Context) (Service, error) {
	s := &ServiceWebSocket{
		ServiceProcessor: NewServiceProcessor(c),
	}
	log.ErrFatal(s.RegisterHandler(s.SimpleResponse))
	return s, nil
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
