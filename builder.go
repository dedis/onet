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

// Builder provides the utility functions to create servers.
type Builder interface {
	// SetService registers the service constructor by its name. If a cipher
	// suite is provided, a key pair will be generated with it. Otherwise the
	// service will have the default pair assigned.
	SetService(string, ciphersuite.CipherSuite, NewServiceFunc)

	// SetSuite must be called at least once as it defines the default suite
	// to use by the server.
	SetSuite(ciphersuite.CipherSuite)

	// SetHost changes the host that the server will listen to.
	SetHost(string)

	// SetPort changes the port that the server will listen to.
	SetPort(int)

	// SetDbPath changes the path of the database file.
	SetDbPath(string)

	// SetIdentity sets the server identity that will be used to generate
	// the host and port the serer will listen to. If this method is called,
	// host and port will be ignored.
	SetIdentity(*network.ServerIdentity)

	// SetSSLCertificate sets the websocket SSL certificate.
	SetSSLCertificate([]byte, []byte, bool)

	// Identity returns the server identity of the builder.
	Identity() *network.ServerIdentity

	// Build returns the server.
	Build() *Server

	// Clone returns a clone of the builder.
	Clone() Builder
}

type serviceRecord struct {
	fn    NewServiceFunc
	suite ciphersuite.CipherSuite
}

// DefaultBuilder creates a server running over TCP.
type DefaultBuilder struct {
	services       map[string]serviceRecord
	cipherRegistry *ciphersuite.Registry
	tls            bool
	host           string
	port           int
	suite          ciphersuite.CipherSuite
	dbPath         string
	si             *network.ServerIdentity
	cert           []byte
	certKey        []byte
	certAsFile     bool
}

// NewDefaultBuilder returns a default builder that will make a server
// running over TCP.
func NewDefaultBuilder() *DefaultBuilder {
	return &DefaultBuilder{
		host:           "127.0.0.1",
		port:           0,
		services:       make(map[string]serviceRecord),
		cipherRegistry: ciphersuite.NewRegistry(),
	}
}

// UseTLS enables the usage of TLS.
func (b *DefaultBuilder) UseTLS() {
	b.tls = true
}

// SetService assigns a service to a name.
func (b *DefaultBuilder) SetService(name string, suite ciphersuite.CipherSuite, fn NewServiceFunc) {
	// Register or get the reference.
	if suite != nil {
		suite = b.cipherRegistry.RegisterCipherSuite(suite)
	}

	b.services[name] = serviceRecord{
		fn:    fn,
		suite: suite,
	}
}

// SetSuite sets the default cipher suite of the server.
func (b *DefaultBuilder) SetSuite(suite ciphersuite.CipherSuite) {
	b.suite = suite
	b.cipherRegistry.RegisterCipherSuite(suite)
}

// SetHost sets the host of the server. Default to 127.0.0.1.
func (b *DefaultBuilder) SetHost(host string) {
	// TODO: host regex
	b.host = host
}

// SetPort sets the port of the server. When 0, it will look for a open one.
// Default to 0.
func (b *DefaultBuilder) SetPort(port int) {
	if port < 0 || port > 65535 {
		panic("port must be in range [0; 65535]")
	}

	b.port = port
}

// SetDbPath sets the path of the database file.
func (b *DefaultBuilder) SetDbPath(path string) {
	b.dbPath = path
}

// SetIdentity sets the server identity to use and thus overriding settings
// like the port number and the host.
func (b *DefaultBuilder) SetIdentity(si *network.ServerIdentity) {
	b.si = si
}

// SetSSLCertificate sets the certificate and its key.
func (b *DefaultBuilder) SetSSLCertificate(cert []byte, key []byte, isFile bool) {
	b.cert = cert
	b.certKey = key
	b.certAsFile = isFile
}

// Clone makes a clone of the builder.
func (b DefaultBuilder) Clone() Builder {
	return &b
}

// Identity returns the server identity of the builder.
func (b *DefaultBuilder) Identity() *network.ServerIdentity {
	str := net.JoinHostPort(b.host, strconv.Itoa(b.port))

	var addr network.Address
	if b.tls {
		addr = network.NewTLSAddress(str)
	} else {
		addr = network.NewTCPAddress(str)
	}
	return b.newIdentity(addr)
}

// Build returns the server.
func (b *DefaultBuilder) Build() *Server {
	b.verifyInput()

	if b.si != nil {
		return b.buildTCP()
	}

	si := b.Identity()
	port := b.port

	tcpHost, err := network.NewTCPHost(b.cipherRegistry, si)
	if err != nil {
		panic(xerrors.Errorf("tcp host: %v", err))
	}

	for port == 0 {
		// For the websocket we need a port at the address one higher than the
		// TCPHost. Let TCPHost chose a port, then check if the port+1 is also
		// available. Else redo the search.
		port, err = strconv.Atoi(tcpHost.Address().Port())
		if err != nil {
			panic(xerrors.Errorf("invalid port: %v", err))
		}

		addrWS := net.JoinHostPort(si.Address.Host(), strconv.Itoa(port+1))
		if l, err := net.Listen("tcp", addrWS); err == nil {
			l.Close()
		} else {
			// Try again..
			port = 0
			tcpHost.Stop()
			tcpHost, err = network.NewTCPHost(b.cipherRegistry, si)
			if err != nil {
				panic(xerrors.Errorf("tcp host: %v", err))
			}
		}
	}

	if len(b.cert) > 0 {
		si.URL = "https://"
	} else {
		si.URL = "http://"
	}

	si.URL += net.JoinHostPort(b.host, strconv.Itoa(port+1))
	straddr := net.JoinHostPort(b.host, strconv.Itoa(port))
	if b.tls {
		si.Address = network.NewTLSAddress(straddr)
	} else {
		si.Address = network.NewTCPAddress(straddr)
	}

	router := network.NewRouter(si, tcpHost)
	router.UnauthOk = true

	srv := newServer(b.cipherRegistry, b.dbPath, router, si.GetPrivate())
	b.registerServices(srv)

	if len(b.cert) > 0 {
		b.fillSSLCertificate(srv)
	}

	return srv
}

func (b *DefaultBuilder) verifyInput() {
	if b.suite == nil {
		panic("A default suite must be set")
	}
}

func (b *DefaultBuilder) buildTCP() *Server {
	r, err := network.NewTCPRouter(b.cipherRegistry, b.si)
	if err != nil {
		panic(err)
	}

	srv := newServer(b.cipherRegistry, "", r, b.si.GetPrivate())
	b.registerServices(srv)

	if len(b.cert) > 0 {
		b.fillSSLCertificate(srv)
	}

	return srv
}

func (b *DefaultBuilder) registerServices(srv *Server) {
	for name, record := range b.services {
		suite := record.suite
		if suite == nil {
			// If the service doesn't have a suite registered, we use the
			// default one.
			suite = b.suite
		} else if !srv.ServerIdentity.HasServiceKeyPair(name) {
			panic("Service " + name + " requires a key pair. " +
				"Use the interactive setup to generate a new file that will include this service.")
		}

		err := srv.serviceManager.register(suite, name, record.fn)
		if err != nil {
			log.Error(err)
			panic("Couldn't register service")
		}
	}
}

func (b *DefaultBuilder) newIdentity(addr network.Address) *network.ServerIdentity {
	pk, sk := b.suite.KeyPair()
	id := network.NewServerIdentity(pk.Pack(), addr)
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

// LocalBuilder is builder to make a server running with a local manager
// instead of using the network.
type LocalBuilder struct {
	*DefaultBuilder
	netman *network.LocalManager
}

// NewLocalBuilder returns a local builder.
func NewLocalBuilder(b *DefaultBuilder) *LocalBuilder {
	lb := &LocalBuilder{
		DefaultBuilder: b.Clone().(*DefaultBuilder),
	}

	if lb.port == 0 {
		// LocalManager does not generate ports like for TCP so it needs
		// to be above 0.
		lb.port = 2000
	}

	return lb
}

// SetLocalManager sets the local manager to use for the server.
func (b *LocalBuilder) SetLocalManager(lm *network.LocalManager) {
	b.netman = lm
}

// SetPort sets the port for the server.
func (b *LocalBuilder) SetPort(port int) {
	if port == 0 {
		panic("local server must have a defined port")
	}

	b.port = port
}

// Identity returns the server identity of the builder.
func (b *LocalBuilder) Identity() *network.ServerIdentity {
	addr := network.NewLocalAddress("127.0.0.1:" + strconv.Itoa(b.port))
	return b.newIdentity(addr)
}

// Build returns a new server using a LocalRouter (channels) to communicate.
func (b *LocalBuilder) Build() *Server {
	b.verifyInput()

	if b.dbPath == "" {
		dir, err := ioutil.TempDir("", "example")
		if err != nil {
			log.Error(err)
			panic("Couldn't create the temporary folder for the database.")
		}

		b.dbPath = dir
	}

	si := b.Identity()
	var err error
	var r *network.Router
	if b.netman != nil {
		r, err = network.NewLocalRouterWithManager(b.netman, si)
	} else {
		r, err = network.NewLocalRouter(si)
	}
	if err != nil {
		panic(err)
	}

	srv := newServer(b.cipherRegistry, b.dbPath, r, si.GetPrivate())
	b.registerServices(srv)

	return srv
}

// Clone returns a clone of the builder.
func (b LocalBuilder) Clone() Builder {
	return &b
}
