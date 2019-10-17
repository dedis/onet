package onet

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"go.dedis.ch/onet/v4/ciphersuite"
	"go.dedis.ch/onet/v4/log"
	"go.dedis.ch/onet/v4/network"
	"golang.org/x/xerrors"
)

// LeakyTestCheck represents an enum to indicate how deep CloseAll needs to
// check the tests.
type LeakyTestCheck int

const (
	// CheckNone will make CloseAll not check anything.
	CheckNone LeakyTestCheck = iota + 1
	// CheckGoroutines will only check for leaking goroutines.
	CheckGoroutines
	// CheckAll will also check for leaking Overlay.Processors and
	// ProtocolInstances.
	CheckAll
)

// TestClose interface allows a service to clean up for the tests. It will only
// be called when a test calls `LocalTest.CloseAll()`.
type TestClose interface {
	// TestClose can clean up things needed in the service.
	TestClose()
}

// LocalTest represents all that is needed for a local test-run
type LocalTest struct {
	builder Builder
	// A map of ServerIdentity.Id to Servers
	Servers map[network.ServerIdentityID]*Server
	// A map of ServerIdentity.Id to Overlays
	Overlays map[network.ServerIdentityID]*Overlay
	// A map of ServerIdentity.Id to Services
	Services map[network.ServerIdentityID]map[string]Service
	// A map of Tree.Id to Trees
	Trees map[TreeID]*Tree
	// All single nodes
	Nodes []*TreeNodeInstance
	// How carefully to check for leaking resources at the end of the test.
	Check LeakyTestCheck
	// are we running tcp or local layer
	mode string
	// TLS certificate if we want TLS for websocket
	webSocketTLSCertificate []byte
	// TLS certificate key if we want TLS for websocket
	webSocketTLSCertificateKey []byte
	// True if the unit test wants that webSocketTLSCertificate and webSocketTLSCertificateKey
	// should be used as filenames.
	webSocketTLSReadFiles bool
	// the context for the local connections
	// it enables to have multiple local test running simultaneously
	ctx  *network.LocalManager
	path string
	// Once closed is set, do not allow further operations on it,
	// since now the temp directory is gone.
	closed bool
	T      *testing.T

	// keep the latestPort used so that we can add nodes later
	latestPort int
}

const (
	// TCP represents the TCP mode of networking for this local test
	TCP = "tcp"
	// Local represents the Local mode of networking for this local test
	Local = "local"
)

// NewLocalTest creates a new Local handler that can be used to test protocols
// locally
func NewLocalTest(builder Builder) *LocalTest {
	dir, err := ioutil.TempDir("", "onet")
	if err != nil {
		log.Fatal("could not create temp directory: ", err)
	}

	return &LocalTest{
		Servers:    make(map[network.ServerIdentityID]*Server),
		Overlays:   make(map[network.ServerIdentityID]*Overlay),
		Services:   make(map[network.ServerIdentityID]map[string]Service),
		Trees:      make(map[TreeID]*Tree),
		Nodes:      make([]*TreeNodeInstance, 0, 1),
		Check:      CheckAll,
		mode:       Local,
		ctx:        network.NewLocalManager(),
		builder:    builder,
		path:       dir,
		latestPort: 2000,
	}
}

// NewLocalTestT is like NewLocalTest but also stores the testing.T variable.
func NewLocalTestT(builder Builder, t *testing.T) *LocalTest {
	l := NewLocalTest(builder)
	l.T = t
	return l
}

// NewTCPTest returns a LocalTest but using a TCPRouter as the underlying
// communication layer.
func NewTCPTest(builder Builder) *LocalTest {
	t := NewLocalTest(builder)
	t.mode = TCP
	return t
}

// NewTCPTestWithTLS returns a LocalTest but using a TCPRouter as the
// underlying communication layer and containing information for TLS setup.
func NewTCPTestWithTLS(builder Builder, wsTLSCertificate []byte, wsTLSCertificateKey []byte) *LocalTest {
	t := NewLocalTest(builder)
	t.mode = TCP
	t.webSocketTLSCertificate = wsTLSCertificate
	t.webSocketTLSCertificateKey = wsTLSCertificateKey
	return t
}

// StartProtocol takes a name and a tree and will create a
// new Node with the protocol 'name' running from the tree-root
func (l *LocalTest) StartProtocol(name string, t *Tree) (ProtocolInstance, error) {
	l.panicClosed()
	rootServerIdentityID := t.Root.ServerIdentity.ID
	for _, h := range l.Servers {
		if h.ServerIdentity.ID.Equal(rootServerIdentityID) {
			// XXX do we really need multiples overlays ? Can't we just use the
			// Node, since it is already dispatched as like a TreeNode ?
			pi, err := l.Overlays[h.ServerIdentity.ID].StartProtocol(name, t, NilServiceID)
			if err != nil {
				return nil, xerrors.Errorf("creating protocol: %v", err)
			}
			return pi, nil
		}
	}
	return nil, xerrors.New("Didn't find server for tree-root")
}

// CreateProtocol takes a name and a tree and will create a
// new Node with the protocol 'name' without running it
func (l *LocalTest) CreateProtocol(name string, t *Tree) (ProtocolInstance, error) {
	l.panicClosed()
	rootServerIdentityID := t.Root.ServerIdentity.ID
	for _, h := range l.Servers {
		if h.ServerIdentity.ID.Equal(rootServerIdentityID) {
			// XXX do we really need multiples overlays ? Can't we just use the
			// Node, since it is already dispatched as like a TreeNode ?
			pi, err := l.Overlays[h.ServerIdentity.ID].CreateProtocol(name, t, NilServiceID)
			if err != nil {
				return nil, xerrors.Errorf("creating protocol: %v", err)
			}
			return pi, nil
		}
	}
	return nil, xerrors.New("Didn't find server for tree-root")
}

// GenServers returns n Servers with a localRouter
func (l *LocalTest) GenServers(n int) []*Server {
	l.panicClosed()
	servers := l.genLocalHosts(n)
	for _, server := range servers {
		server.ServerIdentity.SetPrivate(server.secretKey.Pack())
		l.Servers[server.ServerIdentity.ID] = server
		l.Overlays[server.ServerIdentity.ID] = server.overlay
		l.Services[server.ServerIdentity.ID] = server.serviceManager.services
	}
	return servers

}

// GenTree will create a tree of n servers with a localRouter, and returns the
// list of servers and the associated roster / tree.
func (l *LocalTest) GenTree(n int, register bool) ([]*Server, *Roster, *Tree) {
	l.panicClosed()
	servers := l.GenServers(n)

	list := l.GenRosterFromHost(servers...)
	tree := list.GenerateBinaryTree()
	l.Trees[tree.ID] = tree
	if register {
		servers[0].overlay.RegisterTree(tree)
	}
	return servers, list, tree

}

// GenBigTree will create a tree of n servers.
// If register is true, the Roster and Tree will be registered with the overlay.
// 'nbrServers' is how many servers are created
// 'nbrTreeNodes' is how many TreeNodes are created
// nbrServers can be smaller than nbrTreeNodes, in which case a given server will
// be used more than once in the tree.
func (l *LocalTest) GenBigTree(nbrTreeNodes, nbrServers, bf int, register bool) ([]*Server, *Roster, *Tree) {
	l.panicClosed()
	servers := l.GenServers(nbrServers)

	list := l.GenRosterFromHost(servers...)
	tree := list.GenerateBigNaryTree(bf, nbrTreeNodes)
	l.Trees[tree.ID] = tree
	if register {
		servers[0].overlay.RegisterTree(tree)
	}
	return servers, list, tree
}

// GenRosterFromHost takes a number of servers as arguments and creates
// an Roster.
func (l *LocalTest) GenRosterFromHost(servers ...*Server) *Roster {
	l.panicClosed()
	var entities []*network.ServerIdentity
	for i := range servers {
		entities = append(entities, servers[i].ServerIdentity)
	}
	return NewRoster(entities)
}

func (l *LocalTest) panicClosed() {
	if l.closed {
		panic("attempt to use LocalTest after CloseAll")
	}
}

// WaitDone loops until all protocolInstances are done or
// the timeout is reached. If all protocolInstances are closed
// within the timeout, nil is returned.
func (l *LocalTest) WaitDone(t time.Duration) error {
	var lingering []string
	for i := 0; i < 10; i++ {
		lingering = []string{}
		for _, o := range l.Overlays {
			o.instancesLock.Lock()
			for si, pi := range o.protocolInstances {
				lingering = append(lingering, fmt.Sprintf("ProtocolInstance type %T on %s with id %s",
					pi, o.ServerIdentity(), si))
			}
			o.instancesLock.Unlock()
		}
		for _, s := range l.Servers {
			disp, ok := s.serviceManager.Dispatcher.(*network.RoutineDispatcher)
			if ok && disp.GetRoutines() > 0 {
				lingering = append(lingering, fmt.Sprintf("RoutineDispatcher has %v routines running on %s", disp.GetRoutines(), s.ServerIdentity))
			}
		}
		if len(lingering) == 0 {
			return nil
		}
		time.Sleep(t / 10)
	}
	return xerrors.New("still have things lingering: " + strings.Join(lingering, "\n"))
}

// CloseAll closes all the servers.
func (l *LocalTest) CloseAll() {
	log.Lvl3("Stopping all")
	if r := recover(); r != nil {
		// Make sure that a panic is correctly caught, as CloseAll is most often
		// called in a `defer` statement, and we don't want to show leaking
		// go-routines or hanging protocolInstances if a panic occurs.
		panic(r)
	}
	if l.T != nil && l.T.Failed() {
		return
	}
	InformAllServersStopped()

	// If the debug-level is 0, we copy all errors to a buffer that
	// will be discarded at the end.
	if log.DebugVisible() == 0 {
		log.OutputToBuf()
	}

	var wg sync.WaitGroup
	for _, srv := range l.Servers {
		wg.Add(1)
		go func(s *Server) {
			s.callTestClose()
			wg.Done()
		}(srv)
	}
	wg.Wait()

	if err := l.WaitDone(5 * time.Second); err != nil {
		switch l.Check {
		case CheckNone:
			// Ignore waitDone
		case CheckGoroutines:
			// Only print a warning
			if l.T != nil {
				l.T.Log("Warning:", err)
			} else {
				log.Warn("Warning:", err)
			}
		case CheckAll:
			// Fail if there are leaking processes or protocolInstances
			if l.T != nil {
				l.T.Fatal(err.Error())
			} else {
				log.Fatal(err.Error())
			}
		}
	}

	for _, node := range l.Nodes {
		log.Lvl3("Closing node", node)
		err := node.closeDispatch()
		if err != nil {
			log.Error("Error while closing dispatcher:", err)
		}
	}
	l.Nodes = make([]*TreeNodeInstance, 0)

	sd := sync.WaitGroup{}
	for _, srv := range l.Servers {
		sd.Add(1)
		go func(server *Server) {
			log.Lvl3("Closing server", server.ServerIdentity.Address)
			err := server.Close()
			if err != nil {
				log.Error("Closing server", server.ServerIdentity.Address,
					"gives error", err)
			}

			for server.Listening() {
				log.Lvl1("Sleeping while waiting to close...")
				time.Sleep(10 * time.Millisecond)
			}
			sd.Done()
		}(srv)
	}
	sd.Wait()
	l.Servers = map[network.ServerIdentityID]*Server{}
	l.ctx.Stop()

	err := os.RemoveAll(l.path)
	if err != nil {
		log.Error("Error while removing all db-files:", err)
	}
	l.closed = true

	if log.DebugVisible() == 0 {
		log.OutputToOs()
	}
	if l.Check != CheckNone {
		log.AfterTest(nil)
	}
}

// getTree returns the tree of the given TreeNode
func (l *LocalTest) getTree(tn *TreeNode) *Tree {
	l.panicClosed()
	var tree *Tree
	for _, t := range l.Trees {
		if tn.IsInTree(t) {
			tree = t
			break
		}
	}
	return tree
}

// NewTreeNodeInstance creates a new node on a TreeNode
func (l *LocalTest) NewTreeNodeInstance(tn *TreeNode, protName string) (*TreeNodeInstance, error) {
	l.panicClosed()
	o := l.Overlays[tn.ServerIdentity.ID]
	if o == nil {
		return nil, xerrors.New("Didn't find corresponding overlay")
	}
	tree := l.getTree(tn)
	if tree == nil {
		return nil, xerrors.New("Didn't find tree corresponding to TreeNode")
	}
	protID := ProtocolNameToID(protName)
	if !l.Servers[tn.ServerIdentity.ID].protocols.ProtocolExists(protID) {
		return nil, xerrors.New("Didn't find protocol: " + protName)
	}
	tok := &Token{
		TreeID:     tree.ID,
		TreeNodeID: tn.ID,
	}
	io := o.protoIO.getByName(protName)
	node := newTreeNodeInstance(o, tok, tn, io)
	l.Nodes = append(l.Nodes, node)
	return node, nil
}

// GetTreeNodeInstances returns all TreeNodeInstances that belong to a server
func (l *LocalTest) GetTreeNodeInstances(id network.ServerIdentityID) []*TreeNodeInstance {
	l.panicClosed()
	var nodes []*TreeNodeInstance
	for _, n := range l.Overlays[id].instances {
		nodes = append(nodes, n)
	}
	return nodes
}

// sendTreeNode injects a message directly in the Overlay-layer, bypassing
// Host and Network
func (l *LocalTest) sendTreeNode(proto string, from, to *TreeNodeInstance, msg network.Message) error {
	l.panicClosed()
	ft := from.Tree()
	tt := to.Tree()
	if ft == nil || tt == nil {
		return xerrors.New("cannot find tree")
	}
	if !ft.ID.Equal(tt.ID) {
		return xerrors.New("Can't send from one tree to another")
	}
	onetMsg := &ProtocolMsg{
		Msg:     msg,
		MsgType: network.MessageType(msg),
		From:    from.token,
		To:      to.token,
	}
	io := l.Overlays[to.ServerIdentity().ID].protoIO.getByName(proto)
	err := to.overlay.TransmitMsg(onetMsg, io)
	if err != nil {
		return xerrors.Errorf("transmitting message: %v", err)
	}
	return nil
}

// addPendingTreeMarshal takes a treeMarshal and adds it to the list of the
// known trees, also triggering dispatching of onet-messages waiting for that
// tree
func (l *LocalTest) addPendingTreeMarshal(c *Server, tm *TreeMarshal) {
	l.panicClosed()
	c.overlay.addPendingTreeMarshal(tm)
}

// checkPendingTreeMarshal looks whether there are any treeMarshals to be
// called
func (l *LocalTest) checkPendingTreeMarshal(c *Server, el *Roster) {
	l.panicClosed()
	c.overlay.checkPendingTreeMarshal(el)
}

// GetPrivate returns the private key of a server
func (l *LocalTest) GetPrivate(c *Server) ciphersuite.SecretKey {
	return c.secretKey
}

// GetServices returns a slice of all services asked for.
// The sid is the id of the service that will be collected.
func (l *LocalTest) GetServices(servers []*Server, name string) []Service {
	services := make([]Service, len(servers))
	for i, h := range servers {
		services[i] = l.Services[h.ServerIdentity.ID][name]
	}
	return services
}

// MakeSRS creates and returns nbr Servers, the associated Roster and the
// Service object of the first server in the list having sid as a ServiceID.
func (l *LocalTest) MakeSRS(nbr int, name string) ([]*Server, *Roster, Service) {
	l.panicClosed()
	servers := l.GenServers(nbr)
	el := l.GenRosterFromHost(servers...)
	return servers, el, l.Services[servers[0].ServerIdentity.ID][name]
}

// NewClient returns *Client for which the types depend on the mode of the
// LocalContext.
func (l *LocalTest) NewClient(serviceName string) *Client {
	switch l.mode {
	case TCP:
		return NewClient(serviceName)
	default:
		log.Fatal("Can't make local client")
		return nil
	}
}

// NewClientKeep returns *Client for which the types depend on the mode of the
// LocalContext, the connection is not closed after sending requests.
func (l *LocalTest) NewClientKeep(serviceName string) *Client {
	switch l.mode {
	case TCP:
		return NewClientKeep(serviceName)
	default:
		log.Fatal("Can't make local client")
		return nil
	}
}

// genLocalHosts returns n servers created with a localRouter
func (l *LocalTest) genLocalHosts(n int) []*Server {
	l.panicClosed()
	servers := make([]*Server, n)
	for i := 0; i < n; i++ {
		port := l.latestPort
		l.latestPort += 10
		servers[i] = l.NewServer(port)
	}
	return servers
}

func (l LocalTest) wantsTLS() bool {
	return len(l.webSocketTLSCertificate) > 0 && len(l.webSocketTLSCertificateKey) > 0
}

// NewServer returns a new server which type is determined by the local mode:
// TCP or Local. If it's TCP, then an available port is used, otherwise, the
// port given in argument is used.
func (l *LocalTest) NewServer(port int) *Server {
	l.panicClosed()
	var server *Server
	switch l.mode {
	case TCP:
		server = l.newTCPServer()
		// Set TLS certificate if any configuration available
		if l.wantsTLS() {
			server.WebSocket.Lock()
			if l.webSocketTLSReadFiles {
				cr, err := NewCertificateReloader(
					string(l.webSocketTLSCertificate),
					string(l.webSocketTLSCertificateKey))
				if err != nil {
					log.Error("cannot configure TLS reloader", err)
					return nil
				}
				server.WebSocket.TLSConfig = &tls.Config{
					GetCertificate: cr.GetCertificateFunc(),
				}

			} else {
				cert, err := tls.X509KeyPair(l.webSocketTLSCertificate, l.webSocketTLSCertificateKey)
				if err != nil {
					panic(err)
				}
				server.WebSocket.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
			}
			server.WebSocket.Unlock()
		}
		server.StartInBackground()
	default:
		server = l.NewLocalServer(port)
	}
	return server
}

// NewTCPServer returns a new TCP Server attached to this LocalTest, configured
// for TLS if possible (if anything in LocalTest.webSocketTLSCertificate/Key).
func (l *LocalTest) newTCPServer() *Server {
	l.panicClosed()

	builder := l.builder.(*DefaultBuilder).Clone()
	builder.SetPort(0)
	builder.SetDbPath(l.path)
	builder.SetTLS(l.wantsTLS())
	server := builder.Build()
	l.Servers[server.ServerIdentity.ID] = server
	l.Overlays[server.ServerIdentity.ID] = server.overlay
	l.Services[server.ServerIdentity.ID] = server.serviceManager.services

	return server
}

// NewLocalServer returns a fresh Host using local connections within the context
// of this LocalTest
func (l *LocalTest) NewLocalServer(port int) *Server {
	l.panicClosed()
	builder := NewLocalBuilder(l.builder)
	builder.SetPort(port)
	builder.SetDbPath(l.path)
	builder.SetLocalManager(l.ctx)

	server := builder.Build()
	server.StartInBackground()
	l.Servers[server.ServerIdentity.ID] = server
	l.Overlays[server.ServerIdentity.ID] = server.overlay
	l.Services[server.ServerIdentity.ID] = server.serviceManager.services

	return server

}
