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
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/onet/v3/log"
	"go.dedis.ch/onet/v3/network"
	"go.dedis.ch/protobuf"
	"golang.org/x/xerrors"
)

func init() {
	RegisterNewService(serviceWebSocket, newServiceWebSocket)
}

// Adapted from 'https://golang.org/src/crypto/tls/generate_cert.go'
func generateSelfSignedCert() (string, string, error) {
	// Hostname or IP to generate a certificate for
	hosts := []string{
		"127.0.0.1",
		"::",
	}
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

	for _, host := range hosts {
		if ip := net.ParseIP(host); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, host)
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

	c := l.NewServer(tSuite, 2050)

	defer c.Close()
	require.Equal(t, len(c.serviceManager.services), len(c.WebSocket.services))
	require.NotEmpty(t, c.WebSocket.services[serviceWebSocket])
	cl := NewClientKeep(tSuite, "WebSocket")
	req := &SimpleResponse{}
	msgTypeID := network.MessageType(req)
	log.Lvlf1("Sending message Request: %x", msgTypeID[:])
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

	l := NewTCPTest(tSuite)
	l.webSocketTLSCertificate = cert
	l.webSocketTLSCertificateKey = key
	defer l.CloseAll()

	c := l.NewServer(tSuite, 2050)
	require.Equal(t, len(c.serviceManager.services), len(c.WebSocket.services))
	require.NotEmpty(t, c.WebSocket.services[serviceWebSocket])

	// Test the traditional host:port+1 way of specifying the websocket server.
	cl := NewClientKeep(tSuite, "WebSocket")
	defer cl.Close()
	cl.TLSClientConfig = &tls.Config{RootCAs: CAPool}
	req := &SimpleResponse{}
	msgTypeID := network.MessageType(req)
	log.Lvlf1("Sending message Request: %x", msgTypeID[:])
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

	log.Lvlf1("Sending message Request: %x", msgTypeID[:])
	rcv, err = cl.Send(c.ServerIdentity, "SimpleResponse", buf)
	log.Lvlf1("Received reply: %x", rcv)
	require.Nil(t, protobuf.Decode(rcv, rcvMsg))
	require.Equal(t, int64(1), rcvMsg.Val)
}

// Test the certificate reloader for websocket over TLS.
func TestCertificateReloader(t *testing.T) {
	certPath, keyPath, err := generateSelfSignedCert()
	require.NoError(t, err)
	defer func() {
		os.Remove(certPath)
		os.Remove(keyPath)
	}()

	reloader, err := NewCertificateReloader(certPath, keyPath)
	require.NoError(t, err)

	cert, err := reloader.GetCertificateFunc()(nil)
	require.NoError(t, err)
	require.NotNil(t, cert)

	reloader.certPath = ""
	reloader.keyPath = ""

	// It should work as the certificate is cached.
	cert, err = reloader.GetCertificateFunc()(nil)
	require.NoError(t, err)
	require.NotNil(t, cert)

	// Try with an expired certificate
	// thus expecting an error.
	cert.Leaf.NotAfter = time.Now().Add(30 * time.Minute)
	_, err = reloader.GetCertificateFunc()(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no such file or directory")

	// And finally it should reload the new cert
	reloader.certPath = certPath
	reloader.keyPath = keyPath
	cert, err = reloader.GetCertificateFunc()(nil)
	require.NoError(t, err)
	require.NotNil(t, cert)
}

func TestGetWebHost(t *testing.T) {
	url, err := getWSHostPort(&network.ServerIdentity{Address: "tcp://8.8.8.8"}, true)
	require.Error(t, err)

	url, err = getWSHostPort(&network.ServerIdentity{Address: "tcp://8.8.8.8"}, false)
	require.Error(t, err)

	url, err = getWSHostPort(&network.ServerIdentity{Address: "tcp://8.8.8.8:7770"}, true)
	require.NoError(t, err)
	require.Equal(t, "0.0.0.0:7771", url)

	url, err = getWSHostPort(&network.ServerIdentity{Address: "tcp://8.8.8.8:7770"}, false)
	require.NoError(t, err)
	require.Equal(t, "8.8.8.8:7771", url)

	url, err = getWSHostPort(&network.ServerIdentity{
		Address: "tcp://irrelevant:7770",
		URL:     "wrong url",
	}, false)
	require.Error(t, err)

	url, err = getWSHostPort(&network.ServerIdentity{
		Address: "tcp://irrelevant:7770",
		URL:     "http://8.8.8.8:8888",
	}, false)
	require.NoError(t, err)
	require.Equal(t, "8.8.8.8:8888", url)

	url, err = getWSHostPort(&network.ServerIdentity{
		Address: "tcp://irrelevant:7770",
		URL:     "http://8.8.8.8",
	}, false)
	require.NoError(t, err)
	require.Equal(t, "8.8.8.8:80", url)

	url, err = getWSHostPort(&network.ServerIdentity{
		Address: "tcp://irrelevant:7770",
		URL:     "https://8.8.8.8",
	}, false)
	require.NoError(t, err)
	require.Equal(t, "8.8.8.8:443", url)

	url, err = getWSHostPort(&network.ServerIdentity{
		Address: "tcp://irrelevant:7770",
		URL:     "invalid://8.8.8.8:8888",
	}, false)
	require.Error(t, err)
}

const serviceWebSocket = "WebSocket"

type ServiceWebSocket struct {
	*ServiceProcessor
	Errors int
}

func (i *ServiceWebSocket) SimpleResponse(msg *SimpleResponse) (network.Message, error) {
	return &SimpleResponse{msg.Val + 1}, nil
}

type ErrorRequest struct {
	Roster Roster
	Flags  int
}

func (i *ServiceWebSocket) ErrorRequest(msg *ErrorRequest) (network.Message, error) {
	i.Errors = 1
	index, _ := msg.Roster.Search(i.ServerIdentity().ID)
	if index < 0 {
		return nil, xerrors.New("not in roster")
	}
	if msg.Flags&(1<<uint(index)) > 0 {
		return nil, xerrors.New("found in flags: " + i.ServerIdentity().String())
	}
	i.Errors = 0
	return &SimpleResponse{}, nil
}

func newServiceWebSocket(c *Context) (Service, error) {
	s := &ServiceWebSocket{
		ServiceProcessor: NewServiceProcessor(c),
	}
	log.ErrFatal(s.RegisterHandlers(s.SimpleResponse, s.ErrorRequest))
	return s, nil
}
