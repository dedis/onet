package onet

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dedis/onet/log"
	"github.com/dedis/onet/network"
	"github.com/dedis/protobuf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/websocket"
	"gopkg.in/satori/go.uuid.v1"
)

func init() {
	RegisterNewService(serviceWebSocket, newServiceWebSocket)
}

func generateSelfSignedCert() (string, string, error) {
	var (
		// Comma-separated hostnames and IPs to generate a certificate for
		host = "127.0.0.1"
		// Creation date formatted as Jan 1 15:04:05 2006
		validFrom = time.Now().Format("Jan 1 15:04:05 2006")
		// Duration that certificate is valid for
		validFor = 365 * 24 * time.Hour
		// ECDSA curve to use to generate a key. Valid values are P224, P256 (recommended), P384, P521
		ecdsaCurve = "P256"
	)

	var priv *ecdsa.PrivateKey
	var err error
	switch ecdsaCurve {
	case "P224":
		priv, err = ecdsa.GenerateKey(elliptic.P224(), rand.Reader)
	case "P256":
		priv, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case "P384":
		priv, err = ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	case "P521":
		priv, err = ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	default:
		return "", "", fmt.Errorf("Unrecognized elliptic curve: %q", ecdsaCurve)
	}
	if err != nil {
		return "", "", err
	}

	var notBefore time.Time
	if len(validFrom) == 0 {
		notBefore = time.Now()
	} else {
		notBefore, err = time.Parse("Jan 2 15:04:05 2006", validFrom)
		if err != nil {
			return "", "", err
		}
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

	hosts := strings.Split(host, ",")
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
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
	key, err := ioutil.ReadFile(keyFilePath)
	if err != nil {
		return nil, nil, err
	}

	return cert, key, nil
}

func TestNewWebSocket(t *testing.T) {
	l := NewLocalTest(tSuite)
	defer l.CloseAll()

	c := newTCPServer(tSuite, 0, l.path)
	defer c.Close()
	require.Equal(t, len(c.serviceManager.services), len(c.websocket.services))
	require.NotEmpty(t, c.websocket.services[serviceWebSocket])
	url, err := getWebAddress(c.ServerIdentity, false)
	log.ErrFatal(err)
	ws, err := websocket.Dial(fmt.Sprintf("ws://%s/WebSocket/SimpleResponse", url),
		"", "http://something_else")
	log.ErrFatal(err)
	req := &SimpleResponse{}
	log.Lvlf1("Sending message Request: %x", uuid.UUID(network.MessageType(req)).Bytes())
	buf, err := protobuf.Encode(req)
	log.ErrFatal(err)
	log.ErrFatal(websocket.Message.Send(ws, buf))

	log.Lvl1("Waiting for reply")
	var rcv []byte
	log.ErrFatal(websocket.Message.Receive(ws, &rcv))
	log.Lvlf1("Received reply: %x", rcv)
	rcvMsg := &SimpleResponse{}
	log.ErrFatal(protobuf.Decode(rcv, rcvMsg))
	assert.Equal(t, 1, rcvMsg.Val)
}

func TestNewWebSocketTLS(t *testing.T) {
	cert, key, err := getSelfSignedCertificateAndKey()
	log.ErrFatal(err)
	CAPool := x509.NewCertPool()
	CAPool.AppendCertsFromPEM(cert)

	l := NewLocalTest(tSuite)
	defer l.CloseAll()

	c := newTCPServerWithWebSocketTLS(tSuite, 0, l.path, cert, key)
	defer c.Close()

	require.Equal(t, len(c.serviceManager.services), len(c.websocket.services))
	require.NotEmpty(t, c.websocket.services[serviceWebSocket])
	url, err := getWebAddress(c.ServerIdentity, false)
	log.ErrFatal(err)

	serverUrl := fmt.Sprintf("wss://%s/WebSocket/SimpleResponse", url)
	origin := "http://localhost"
	config, err := websocket.NewConfig(serverUrl, origin)
	log.ErrFatal(err)
	config.TlsConfig = &tls.Config{RootCAs: CAPool}
	ws, err := websocket.DialConfig(config)
	log.ErrFatal(err)

	req := &SimpleResponse{}
	log.Lvlf1("Sending message Request: %x", uuid.UUID(network.MessageType(req)).Bytes())
	buf, err := protobuf.Encode(req)
	log.ErrFatal(err)
	log.ErrFatal(websocket.Message.Send(ws, buf))

	log.Lvl1("Waiting for reply")
	var rcv []byte
	log.ErrFatal(websocket.Message.Receive(ws, &rcv))
	log.Lvlf1("Received reply: %x", rcv)
	rcvMsg := &SimpleResponse{}
	log.ErrFatal(protobuf.Decode(rcv, rcvMsg))
	assert.Equal(t, 1, rcvMsg.Val)
}

func TestGetWebHost(t *testing.T) {
	url, err := getWebAddress(&network.ServerIdentity{Address: "tcp://8.8.8.8"}, true)
	require.NotNil(t, err)
	url, err = getWebAddress(&network.ServerIdentity{Address: "tcp://8.8.8.8"}, false)
	require.NotNil(t, err)
	url, err = getWebAddress(&network.ServerIdentity{Address: "tcp://8.8.8.8:7770"}, true)
	log.ErrFatal(err)
	require.Equal(t, "0.0.0.0:7771", url)
	url, err = getWebAddress(&network.ServerIdentity{Address: "tcp://8.8.8.8:7770"}, false)
	log.ErrFatal(err)
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
	assert.Equal(t, uint64(0), client.Rx())
	assert.Equal(t, uint64(0), client.Tx())
	log.ErrFatal(client.SendProtobuf(servers[0].ServerIdentity, r, sr))
	assert.Equal(t, sr.Val, 10)
	assert.NotEqual(t, uint64(0), client.Rx())
	assert.NotEqual(t, uint64(0), client.Tx())
	assert.True(t, client.Tx() > client.Rx())
}

func TestClientTLS_Send(t *testing.T) {
	cert, key, err := getSelfSignedCertificateAndKey()
	log.ErrFatal(err)
	CAPool := x509.NewCertPool()
	CAPool.AppendCertsFromPEM(cert)

	local := NewTCPTestWithTLS(tSuite, cert, key)
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
	client.tls = true
	client.trustedCertificates = CAPool

	r := &SimpleRequest{
		ServerIdentities: el,
		Val:              10,
	}
	sr := &SimpleResponse{}
	assert.Equal(t, uint64(0), client.Rx())
	assert.Equal(t, uint64(0), client.Tx())
	log.ErrFatal(client.SendProtobuf(servers[0].ServerIdentity, r, sr))
	assert.Equal(t, sr.Val, 10)
	assert.NotEqual(t, uint64(0), client.Rx())
	assert.NotEqual(t, uint64(0), client.Tx())
	assert.True(t, client.Tx() > client.Rx())
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
			log.ErrFatal(client.SendProtobuf(servers[0].ServerIdentity, r, sr))
			assert.Equal(t, 10*i, sr.Val)
			log.Lvl1("Done with message", i)
			wg.Done()
		}(i)
	}
	wg.Wait()
}

func TestClientTLS_Parallel(t *testing.T) {
	cert, key, err := getSelfSignedCertificateAndKey()
	log.ErrFatal(err)
	CAPool := x509.NewCertPool()
	CAPool.AppendCertsFromPEM(cert)

	nbrNodes := 4
	nbrParallel := 20
	local := NewTCPTestWithTLS(tSuite, cert, key)
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
			client.tls = true
			client.trustedCertificates = CAPool
			sr := &SimpleResponse{}
			log.ErrFatal(client.SendProtobuf(servers[0].ServerIdentity, r, sr))
			assert.Equal(t, 10*i, sr.Val)
			log.Lvl1("Done with message", i)
			wg.Done()
		}(i)
	}
	wg.Wait()
}

func TestNewClientKeep(t *testing.T) {
	c := NewClientKeep(tSuite, serviceWebSocket)
	assert.True(t, c.keep)
}

func TestMultiplePath(t *testing.T) {
	_, err := RegisterNewService(dummyService3Name, func(c *Context) (Service, error) {
		ds := &DummyService3{}
		return ds, nil
	})
	log.ErrFatal(err)
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
	log.ErrFatal(err)
	CAPool := x509.NewCertPool()
	CAPool.AppendCertsFromPEM(cert)

	_, err = RegisterNewService(dummyService3Name, func(c *Context) (Service, error) {
		ds := &DummyService3{}
		return ds, nil
	})
	log.ErrFatal(err)
	defer UnregisterService(dummyService3Name)

	local := NewTCPTestWithTLS(tSuite, cert, key)
	hs := local.GenServers(2)
	server := hs[0]
	defer local.CloseAll()
	client := NewClientKeep(tSuite, dummyService3Name)
	client.tls = true
	client.trustedCertificates = CAPool
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

	log.OutputToBuf()
	_, err := client.Send(server.ServerIdentity, "test", nil)
	log.OutputToOs()
	require.NotEqual(t, "websocket: bad handshake", err.Error())
	assert.NotEqual(t, "", log.GetStdOut())
}

func TestWebSocketTLS_Error(t *testing.T) {
	cert, key, err := getSelfSignedCertificateAndKey()
	log.ErrFatal(err)
	CAPool := x509.NewCertPool()
	CAPool.AppendCertsFromPEM(cert)

	client := NewClientKeep(tSuite, dummyService3Name)
	client.tls = true
	client.trustedCertificates = CAPool
	local := NewTCPTestWithTLS(tSuite, cert, key)
	hs := local.GenServers(2)
	server := hs[0]
	defer local.CloseAll()

	log.OutputToBuf()
	_, err = client.Send(server.ServerIdentity, "test", nil)
	log.OutputToOs()
	require.NotEqual(t, "websocket: bad handshake", err.Error())
	assert.NotEqual(t, "", log.GetStdOut())
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
