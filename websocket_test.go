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
	"os"
	"sync"
	"testing"
	"time"

	"github.com/dedis/protobuf"
	"github.com/stretchr/testify/require"
	"gopkg.in/dedis/onet.v2/log"
	"gopkg.in/dedis/onet.v2/network"
	"gopkg.in/satori/go.uuid.v1"
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
	require.Equal(t, 1, rcvMsg.Val)
}

func TestNewWebSocketTLS(t *testing.T) {
	cert, key, err := getSelfSignedCertificateAndKey()
	require.Nil(t, err)
	CAPool := x509.NewCertPool()
	CAPool.AppendCertsFromPEM(cert)

	l := NewLocalTest(tSuite)
	defer l.CloseAll()

	c := newTCPServer(tSuite, 0, l.path)

	certToAdd, err := tls.X509KeyPair(cert, key)
	if err != nil {
		require.Nil(t, err)
	}
	c.WebSocket.Lock()
	c.WebSocket.TLSConfig = &tls.Config{Certificates: []tls.Certificate{certToAdd}}
	c.WebSocket.Unlock()
	c.StartInBackground()
	defer c.Close()

	require.Equal(t, len(c.serviceManager.services), len(c.WebSocket.services))
	require.NotEmpty(t, c.WebSocket.services[serviceWebSocket])

	cl := NewClientKeep(tSuite, "WebSocket")
	cl.TLSClientConfig = &tls.Config{RootCAs: CAPool}
	req := &SimpleResponse{}
	log.Lvlf1("Sending message Request: %x", uuid.UUID(network.MessageType(req)).Bytes())
	buf, err := protobuf.Encode(req)
	require.Nil(t, err)
	rcv, err := cl.Send(c.ServerIdentity, "SimpleResponse", buf)
	log.Lvlf1("Received reply: %x", rcv)
	rcvMsg := &SimpleResponse{}
	require.Nil(t, protobuf.Decode(rcv, rcvMsg))
	require.Equal(t, 1, rcvMsg.Val)
}

func TestGetWebHost(t *testing.T) {
	url, err := getWebAddress(&network.ServerIdentity{Address: "tcp://8.8.8.8"}, true)
	require.NotNil(t, err)
	url, err = getWebAddress(&network.ServerIdentity{Address: "tcp://8.8.8.8"}, false)
	require.NotNil(t, err)
	url, err = getWebAddress(&network.ServerIdentity{Address: "tcp://8.8.8.8:7770"}, true)
	require.Nil(t, err)
	require.Equal(t, "0.0.0.0:7771", url)
	url, err = getWebAddress(&network.ServerIdentity{Address: "tcp://8.8.8.8:7770"}, false)
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
	require.Equal(t, sr.Val, 10)
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
	require.Equal(t, sr.Val, 10)
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
			log.Lvl1("Starting message", i)
			r := &SimpleRequest{
				ServerIdentities: el,
				Val:              10 * i,
			}
			client := local.NewClient(backForthServiceName)
			sr := &SimpleResponse{}
			err := client.SendProtobuf(servers[0].ServerIdentity, r, sr)
			require.Nil(t, err)
			require.Equal(t, 10*i, sr.Val)
			log.Lvl1("Done with message", i)
			wg.Done()
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
			log.Lvl1("Starting message", i)
			r := &SimpleRequest{
				ServerIdentities: el,
				Val:              10 * i,
			}
			client := local.NewClient(backForthServiceName)
			client.TLSClientConfig = &tls.Config{RootCAs: CAPool}
			sr := &SimpleResponse{}
			require.Nil(t, client.SendProtobuf(servers[0].ServerIdentity, r, sr))
			require.Equal(t, 10*i, sr.Val)
			log.Lvl1("Done with message", i)
			wg.Done()
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
	require.NotEqual(t, "", log.GetStdOut())
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
	require.NotEqual(t, "", log.GetStdOut())
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

func (ds *DummyService3) ProcessClientRequest(req *http.Request, path string, buf []byte) ([]byte, error) {
	log.Lvl2("Got called with path", path, buf)
	return []byte(path), nil
}

func (ds *DummyService3) NewProtocol(tn *TreeNodeInstance, conf *GenericConfig) (ProtocolInstance, error) {
	return nil, nil
}

func (ds *DummyService3) Process(env *network.Envelope) {
}
