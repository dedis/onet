package onet

import (
	"crypto/tls"
	"io/ioutil"
	"net"
	"strconv"

	"go.dedis.ch/onet/v4/ciphersuite"
	"go.dedis.ch/onet/v4/log"
	"go.dedis.ch/onet/v4/network"
	"golang.org/x/xerrors"
)

type Builder interface {
	SetService(name string, suite ciphersuite.CipherSuite, fn NewServiceFunc)
	SetSuite(suite ciphersuite.CipherSuite)
	SetPort(port int)
	SetDbPath(path string)
	SetIdentity(si *network.ServerIdentity)
	SetSSLCertificate(cert []byte, key []byte, isFile bool)
	Identity() *network.ServerIdentity
	Build() *Server
	Clone() Builder
}

type serviceRecord struct {
	fn    NewServiceFunc
	suite ciphersuite.CipherSuite
}

type DefaultBuilder struct {
	services       map[string]serviceRecord
	cipherRegistry *ciphersuite.Registry
	port           int
	suite          ciphersuite.CipherSuite
	dbPath         string
	si             *network.ServerIdentity
	cert           []byte
	certKey        []byte
	certAsFile     bool
}

func NewDefaultBuilder() *DefaultBuilder {
	return &DefaultBuilder{
		services:       make(map[string]serviceRecord),
		cipherRegistry: ciphersuite.NewRegistry(),
	}
}

func (b *DefaultBuilder) SetService(name string, suite ciphersuite.CipherSuite, fn NewServiceFunc) {
	b.services[name] = serviceRecord{
		fn:    fn,
		suite: suite,
	}
	if suite != nil {
		b.cipherRegistry.RegisterCipherSuite(suite)
	}
}

func (b *DefaultBuilder) SetSuite(suite ciphersuite.CipherSuite) {
	b.suite = suite
	b.cipherRegistry.RegisterCipherSuite(suite)
}

func (b *DefaultBuilder) SetPort(port int) {
	b.port = port
}

func (b *DefaultBuilder) SetDbPath(path string) {
	b.dbPath = path
}

func (b *DefaultBuilder) SetIdentity(si *network.ServerIdentity) {
	b.si = si
}

func (b *DefaultBuilder) SetSSLCertificate(cert []byte, key []byte, isFile bool) {
	b.cert = cert
	b.certKey = key
	b.certAsFile = isFile
}

func (b DefaultBuilder) Clone() Builder {
	return &b
}

func (b *DefaultBuilder) Identity() *network.ServerIdentity {
	return b.newIdentity()
}

func (b *DefaultBuilder) Build() *Server {
	if b.si != nil {
		return b.buildTCP()
	}

	si := b.newIdentity()
	addr := network.NewTCPAddress(si.Address.NetworkAddress())
	id2 := network.NewServerIdentity(si.PublicKey, addr)

	var tcpHost *network.TCPHost
	var addrWS string
	// For the websocket we need a port at the address one higher than the
	// TCPHost. Let TCPHost chose a port, then check if the port+1 is also
	// available. Else redo the search.
	for {
		var err error
		tcpHost, err = network.NewTCPHost(b.cipherRegistry, id2)
		if err != nil {
			panic(xerrors.Errorf("tcp host: %v", err))
		}
		si.Address = tcpHost.Address()
		port, err := strconv.Atoi(si.Address.Port())
		if err != nil {
			panic(xerrors.Errorf("invalid port: %v", err))
		}
		addrWS = net.JoinHostPort(si.Address.Host(), strconv.Itoa(port+1))
		if b.port != 0 {
			break
		}
		if l, err := net.Listen("tcp", addrWS); err == nil {
			l.Close()
			break
		}
		log.Lvl2("Found closed port:", addrWS)
	}

	if len(b.cert) > 0 {
		si.URL = "https://" + addrWS
	} else {
		si.URL = "http://" + addrWS
	}

	si.Address = network.NewAddress(si.Address.ConnType(), "127.0.0.1:"+si.Address.Port())

	router := network.NewRouter(si, tcpHost)
	router.UnauthOk = true
	srv := newServer(b.cipherRegistry, b.dbPath, router, si.GetPrivate())
	for name, record := range b.services {
		srv.serviceManager.register(record.suite, name, record.fn)
	}

	if len(b.cert) > 0 {
		b.fillSSLCertificate(srv)
	}

	return srv
}

func (b *DefaultBuilder) buildTCP() *Server {
	r, err := network.NewTCPRouterWithListenAddr(b.cipherRegistry, b.si, "")
	if err != nil {
		panic(err)
	}

	srv := newServer(b.cipherRegistry, "", r, b.si.GetPrivate())

	if len(b.cert) > 0 {
		b.fillSSLCertificate(srv)
	}

	return srv
}

func (b *DefaultBuilder) newIdentity() *network.ServerIdentity {
	address := network.NewLocalAddress("127.0.0.1:" + strconv.Itoa(b.port))
	pk, sk := b.suite.KeyPair()
	id := network.NewServerIdentity(pk.Pack(), address)
	id.SetPrivate(sk.Pack())
	b.generateKeyPairs(id)

	return id
}

func (b *DefaultBuilder) generateKeyPairs(si *network.ServerIdentity) {
	services := []network.ServiceIdentity{}
	for name, record := range b.services {
		if record.suite != nil {
			pk, sk := record.suite.KeyPair()
			sid := network.NewServiceIdentity(name, pk.Pack(), sk.Pack())

			services = append(services, sid)
		}
	}
	si.ServiceIdentities = services
}

func (b *DefaultBuilder) fillSSLCertificate(server *Server) {
	server.WebSocket.Lock()
	if b.certAsFile {
		cr, err := NewCertificateReloader(string(b.cert), string(b.certKey))
		if err != nil {
			log.Error("cannot configure TLS reloader", err)
			panic(err)
		}
		server.WebSocket.TLSConfig = &tls.Config{
			GetCertificate: cr.GetCertificateFunc(),
		}
	} else {
		cert, err := tls.X509KeyPair(b.cert, b.certKey)
		if err != nil {
			panic(err)
		}
		server.WebSocket.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
	}
	server.WebSocket.Unlock()
}

type LocalBuilder struct {
	*DefaultBuilder
	netman *network.LocalManager
}

func NewLocalBuilder(b *DefaultBuilder) *LocalBuilder {
	return &LocalBuilder{
		DefaultBuilder: b.Clone().(*DefaultBuilder),
	}
}

func (b *LocalBuilder) SetLocalManager(lm *network.LocalManager) {
	b.netman = lm
}

// Build returns a new server using a LocalRouter (channels) to communicate.
func (b *LocalBuilder) Build() *Server {
	dir, err := ioutil.TempDir("", "example")
	if err != nil {
		log.Fatal(err)
	}

	if b.dbPath == "" {
		b.dbPath = dir
	}

	si := b.newIdentity()
	var r *network.Router
	if b.netman != nil {
		r, err = network.NewLocalRouterWithManager(b.netman, si)
	} else {
		r, err = network.NewLocalRouter(si)
	}
	if err != nil {
		panic(err)
	}

	srv := newServer(b.cipherRegistry, dir, r, si.GetPrivate())
	for name, record := range b.services {
		srv.serviceManager.register(record.suite, name, record.fn)
	}

	return srv
}

func (b LocalBuilder) Clone() Builder {
	return &b
}
