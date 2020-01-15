package onet

import (
	"bytes"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/onet/v3/log"
	"go.dedis.ch/onet/v3/network"
	"go.dedis.ch/protobuf"
	"golang.org/x/xerrors"
)

const dummyServiceName = "dummyService"
const dummyService2Name = "dummyService2"
const ismServiceName = "ismService"
const backForthServiceName = "backForth"
const dummyProtocolName = "DummyProtocol2"

func init() {
	network.RegisterMessage(SimpleMessageForth{})
	network.RegisterMessage(SimpleMessageBack{})
	network.RegisterMessage(SimpleRequest{})
	dummyMsgType = network.RegisterMessage(DummyMsg{})
	RegisterNewService(ismServiceName, newServiceMessages)
	RegisterNewService(dummyService2Name, newDummyService2)
	GlobalProtocolRegister(dummyProtocolName, newDummyProtocol2)
}

func TestServiceRegistration(t *testing.T) {
	var name = "dummy"
	RegisterNewService(name, func(c *Context) (Service, error) {
		return &DummyService{}, nil
	})

	names := ServiceFactory.RegisteredServiceNames()
	var found bool
	for _, n := range names {
		if n == name {
			found = true
		}
	}
	if !found {
		t.Fatal("Name not found !?")
	}
	ServiceFactory.Unregister(name)
	names = ServiceFactory.RegisteredServiceNames()
	for _, n := range names {
		if n == name {
			t.Fatal("Dummy should not be found!")
		}
	}

	var nameWithSuite = "dummyWithSuite"
	sid, err := RegisterNewServiceWithSuite(nameWithSuite, tSuite, func(c *Context) (Service, error) {
		return &DummyService{}, nil
	})
	require.NoError(t, err)

	suite := ServiceFactory.Suite(nameWithSuite)
	require.NotNil(t, suite)
	suite = ServiceFactory.SuiteByID(sid)
	require.NotNil(t, suite)

	suite = ServiceFactory.Suite(name)
	require.Nil(t, suite)

	UnregisterService(nameWithSuite)
}

func TestServiceNew(t *testing.T) {
	ds := &DummyService{
		link: make(chan bool, 1),
	}
	RegisterNewService(dummyServiceName, func(c *Context) (Service, error) {
		ds.c = c
		ds.link <- true
		return ds, nil
	})
	defer UnregisterService(dummyServiceName)
	var local *LocalTest
	servers := make(chan bool)
	go func() {
		local = NewLocalTest(tSuite)
		local.GenServers(1)
		servers <- true
	}()

	<-servers
	waitOrFatal(ds.link, t)
	local.CloseAll()
}

// TestService_StartWithKP checks that services with a registered suite
// won't start if the key pair is not provided in the conode toml file.
func TestService_StartWithKP(t *testing.T) {
	dummyWithSuite := "dummyWithSuite"
	RegisterNewServiceWithSuite(dummyWithSuite, tSuite, func(c *Context) (Service, error) {
		return &DummyService{}, nil
	})

	si := &network.ServerIdentity{}
	ctx := &Context{
		server: &Server{
			Router: &network.Router{ServerIdentity: si},
		},
	}
	_, err := ServiceFactory.start(dummyWithSuite, ctx)
	require.Contains(t, err.Error(), "requires a key pair")

	ServiceFactory.generateKeyPairs(si)
	_, err = ServiceFactory.start(dummyWithSuite, ctx)
	require.NoError(t, err)

	UnregisterService(dummyWithSuite)
}

func TestServiceProcessRequest(t *testing.T) {
	link := make(chan bool, 1)
	_, err := RegisterNewService(dummyServiceName, func(c *Context) (Service, error) {
		ds := &DummyService{
			link: link,
			c:    c,
		}
		return ds, nil
	})
	log.ErrFatal(err)
	defer UnregisterService(dummyServiceName)

	local := NewTCPTest(tSuite)
	hs := local.GenServers(2)
	server := hs[0]
	log.Lvl1("Host created and listening")
	defer local.CloseAll()
	// Send a request to the service
	client := NewClient(tSuite, dummyServiceName)
	log.Lvl1("Sending request to service...")
	_, err = client.Send(server.ServerIdentity, "nil", []byte("a"))
	log.Lvl2("Got reply")
	require.Error(t, err)
	// wait for the link
	if <-link {
		log.Fatal("was expecting false !")
	}
}

// Test if a request that makes the service create a new protocol works
func TestServiceRequestNewProtocol(t *testing.T) {
	ds := &DummyService{
		link: make(chan bool, 1),
	}
	RegisterNewService(dummyServiceName, func(c *Context) (Service, error) {
		ds.c = c
		return ds, nil
	})

	defer UnregisterService(dummyServiceName)
	local := NewTCPTest(tSuite)
	hs := local.GenServers(2)
	server := hs[0]
	client := local.NewClient(dummyServiceName)
	defer local.CloseAll()
	// create the entityList and tree
	el := NewRoster([]*network.ServerIdentity{server.ServerIdentity})
	tree := el.GenerateBinaryTree()
	// give it to the service
	ds.fakeTree = tree

	// Send a request to the service
	log.Lvl1("Sending request to service...")
	log.ErrFatal(client.SendProtobuf(server.ServerIdentity, &DummyMsg{10}, nil))
	// wait for the link from the
	waitOrFatalValue(ds.link, true, t)

	// Now resend the value so we instantiate using the same treenode
	log.Lvl1("Sending request again to service...")
	err := client.SendProtobuf(server.ServerIdentity, &DummyMsg{10}, nil)
	require.Error(t, err)
	// this should fail
	waitOrFatalValue(ds.link, false, t)
}

// test for calling the NewProtocol method on a remote Service
func TestServiceNewProtocol(t *testing.T) {
	ds1 := &DummyService{
		link: make(chan bool),
		Config: DummyConfig{
			Send: true,
		},
	}
	ds2 := &DummyService{
		link: make(chan bool),
	}
	var count int
	countMutex := sync.Mutex{}
	RegisterNewService(dummyServiceName, func(c *Context) (Service, error) {
		countMutex.Lock()
		defer countMutex.Unlock()
		log.Lvl2("Creating service", count)
		var localDs *DummyService
		switch count {
		case 2:
			// the client does not need a Service
			return &DummyService{link: make(chan bool)}, nil
		case 1: // children
			localDs = ds2
		case 0: // root
			localDs = ds1
		}
		localDs.c = c

		count++
		return localDs, nil
	})

	defer UnregisterService(dummyServiceName)
	local := NewTCPTest(tSuite)
	defer local.CloseAll()
	hs := local.GenServers(3)
	server1, server2 := hs[0], hs[1]
	client := local.NewClient(dummyServiceName)
	log.Lvl1("Host created and listening")

	// create the entityList and tree
	el := NewRoster([]*network.ServerIdentity{server1.ServerIdentity, server2.ServerIdentity})
	tree := el.GenerateBinaryTree()
	// give it to the service
	ds1.fakeTree = tree

	// Send a request to the service
	log.Lvl1("Sending request to service...")
	log.ErrFatal(client.SendProtobuf(server1.ServerIdentity, &DummyMsg{10}, nil))
	log.Lvl1("Waiting for end")
	// wait for the link from the protocol that Starts
	waitOrFatalValue(ds1.link, true, t)
	// now wait for the second link on the second HOST that the second service
	// should have started (ds2) in ProcessRequest
	waitOrFatalValue(ds2.link, true, t)
	log.Lvl1("Done")
}

func TestServiceProcessor(t *testing.T) {
	ds1 := &DummyService{
		link: make(chan bool),
	}
	ds2 := &DummyService{
		link: make(chan bool),
	}
	var count int
	RegisterNewService(dummyServiceName, func(c *Context) (Service, error) {
		var s *DummyService
		if count == 0 {
			s = ds1
		} else {
			s = ds2
		}
		s.c = c
		c.RegisterProcessor(s, dummyMsgType)
		return s, nil
	})
	local := NewLocalTest(tSuite)
	defer local.CloseAll()
	hs := local.GenServers(2)
	server1, server2 := hs[0], hs[1]

	defer UnregisterService(dummyServiceName)
	// create two servers
	log.Lvl1("Host created and listening")
	// create request
	log.Lvl1("Sending request to service...")
	sentLen, err := server2.Send(server1.ServerIdentity, &DummyMsg{10})
	require.Nil(t, err)
	require.NotNil(t, sentLen)

	// wait for the link from the Service on server 1
	waitOrFatalValue(ds1.link, true, t)
}

func TestServiceBackForthProtocol(t *testing.T) {
	local := NewTCPTest(tSuite)
	defer local.CloseAll()

	// register service
	_, err := RegisterNewService(backForthServiceName, func(c *Context) (Service, error) {
		return &simpleService{
			ctx:      c,
			newProto: make(chan bool, 10),
		}, nil
	})
	log.ErrFatal(err)
	defer ServiceFactory.Unregister(backForthServiceName)

	// create servers
	servers, el, _ := local.GenTree(4, false)

	// create client
	client := local.NewClient(backForthServiceName)

	// create request
	r := &SimpleRequest{
		ServerIdentities: el,
		Val:              10,
	}
	sr := &SimpleResponse{}
	err = client.SendProtobuf(servers[0].ServerIdentity, r, sr)
	log.ErrFatal(err)
	require.Equal(t, sr.Val, int64(10))
}

func TestPanicNewProto(t *testing.T) {
	local := NewTCPTest(tSuite)
	defer local.CloseAll()

	name := "panicSvc"
	panicID, err := RegisterNewService(name, func(c *Context) (Service, error) {
		return &simpleService{
			ctx:      c,
			panic:    true,
			newProto: make(chan bool, 1),
		}, nil
	})
	log.ErrFatal(err)
	defer ServiceFactory.Unregister(name)

	// create servers
	servers, el, _ := local.GenTree(2, false)
	services := local.GetServices(servers, panicID)

	// create client
	client := local.NewClient(name)

	// create request
	r := &SimpleRequest{
		ServerIdentities: el,
		Val:              10,
	}
	sr := &SimpleResponse{}
	err = client.SendProtobuf(servers[0].ServerIdentity, r, sr)
	require.Nil(t, err)
	client.Close()
	<-services[1].(*simpleService).newProto
}

func TestServiceManager_Service(t *testing.T) {
	local := NewLocalTest(tSuite)
	defer local.CloseAll()
	servers, _, _ := local.GenTree(2, true)

	services := servers[0].serviceManager.availableServices()
	require.NotEqual(t, 0, len(services), "no services available")

	service := servers[0].serviceManager.service("testService")
	require.NotNil(t, service, "Didn't find service testService")
}

func TestServiceMessages(t *testing.T) {
	local := NewLocalTest(tSuite)
	defer local.CloseAll()
	servers, _, _ := local.GenTree(2, true)

	service := servers[0].serviceManager.service(ismServiceName)
	require.NotNil(t, service, "Didn't find service ISMService")
	ism := service.(*ServiceMessages)
	ism.SendRaw(servers[0].ServerIdentity, &SimpleResponse{})
	require.True(t, <-ism.GotResponse, "Didn't get response")
}

func TestServiceProtocolInstantiation(t *testing.T) {
	local := NewLocalTest(tSuite)
	defer local.CloseAll()
	servers, _, tree := local.GenTree(2, true)

	s1 := servers[0].serviceManager.service(dummyService2Name)
	s2 := servers[1].serviceManager.service(dummyService2Name)

	ds1 := s1.(*dummyService2)
	ds2 := s2.(*dummyService2)

	link := make(chan bool)
	ds1.link = link
	ds2.link = link

	go ds1.launchProtoStart(tree, false, true)
	waitOrFatal(link, t)
	waitOrFatal(link, t)
	waitOrFatal(link, t)
}

func TestServiceGenericConfig(t *testing.T) {
	local := NewLocalTest(tSuite)
	defer local.CloseAll()
	servers, _, tree := local.GenTree(2, true)

	s1 := servers[0].serviceManager.service(dummyService2Name)
	s2 := servers[1].serviceManager.service(dummyService2Name)

	ds1 := s1.(*dummyService2)
	ds2 := s2.(*dummyService2)

	link := make(chan bool)
	ds1.link = link
	ds2.link = link

	// First launch without any config
	go ds1.launchProto(tree, false)
	// wait for the service's protocol creation
	waitOrFatalValue(link, true, t)
	// wait for the service 2 say there is no config
	waitOrFatalValue(link, false, t)
	// then laucnh with config
	go ds1.launchProto(tree, true)
	// wait for the service's protocol creation
	waitOrFatalValue(link, true, t)
	// wait for the service 2 say there is no config
	waitOrFatalValue(link, true, t)
}

func TestServiceConfigRace(t *testing.T) {
	nbrNodes := 10
	log.SetDebugVisible(2)
	for i := 0; i < 1; i++ {
		local := NewLocalTest(tSuite)
		trees := make([]*Tree, nbrNodes)
		servers := local.GenServers(nbrNodes)
		roster := local.GenRosterFromHost(servers...)

		dummyServices := make([]*dummyService2, nbrNodes)
		msgs := make(chan bool)
		for node := 0; node < nbrNodes; node++ {
			trees[node] = roster.GenerateNaryTreeWithRoot(2,
				servers[node].ServerIdentity)
			s := servers[node].serviceManager.service(dummyService2Name)
			dummyServices[node] = s.(*dummyService2)
			dummyServices[node].link = msgs
		}

		// Launch n parallel protocols to check races in setting up the
		// connections
		wg := sync.WaitGroup{}
		wg.Add(nbrNodes)
		for node := 0; node < nbrNodes; node++ {
			go func(n int) {
				log.Print("launching", n)
				go dummyServices[n].launchProto(trees[n], true)
				// wait for the service's protocol creation
				for newProto := 0; newProto < nbrNodes; newProto++ {
					waitOrFatalValue(msgs, true, t)
					log.Print(n * 10 + newProto)
				}
				wg.Done()
			}(node)
		}
		wg.Wait()
		local.CloseAll()
	}
}

// BackForthProtocolForth & Back are messages that go down and up the tree.
// => BackForthProtocol protocol / message
type SimpleMessageForth struct {
	Val int64
}

type SimpleMessageBack struct {
	Val int64
}

type BackForthProtocol struct {
	*TreeNodeInstance
	Val       int64
	counter   int64
	forthChan chan struct {
		*TreeNode
		SimpleMessageForth
	}
	backChan chan struct {
		*TreeNode
		SimpleMessageBack
	}
	stop    chan struct{}
	handler func(val int)
}

func newBackForthProtocolRoot(tn *TreeNodeInstance, val int, handler func(int)) (ProtocolInstance, error) {
	s, err := newBackForthProtocol(tn)
	s.Val = int64(val)
	s.handler = handler
	return s, err
}

func newBackForthProtocol(tn *TreeNodeInstance) (*BackForthProtocol, error) {
	s := &BackForthProtocol{
		TreeNodeInstance: tn,
		stop:             make(chan struct{}),
	}
	err := s.RegisterChannel(&s.forthChan)
	if err != nil {
		return nil, err
	}
	err = s.RegisterChannel(&s.backChan)
	if err != nil {
		return nil, err
	}
	go s.dispatch()
	return s, nil
}

func (sp *BackForthProtocol) Start() error {
	// send down to children
	msg := &SimpleMessageForth{
		Val: sp.Val,
	}
	for _, ch := range sp.Children() {
		if err := sp.SendTo(ch, msg); err != nil {
			return err
		}
	}
	return nil
}

func (sp *BackForthProtocol) Shutdown() error {
	close(sp.stop)
	return nil
}

func (sp *BackForthProtocol) dispatch() error {
	for {
		select {
		// dispatch the first msg down
		case m := <-sp.forthChan:
			msg := &m.SimpleMessageForth
			for _, ch := range sp.Children() {
				sp.SendTo(ch, msg)
			}
			if sp.IsLeaf() {
				if err := sp.SendTo(sp.Parent(), &SimpleMessageBack{msg.Val}); err != nil {
					log.Error(err)
				}
				sp.Done()
				return nil
			}
		// pass the message up
		case m := <-sp.backChan:
			msg := m.SimpleMessageBack
			// call the handler  if we are the root
			sp.counter++
			if int(sp.counter) == len(sp.Children()) {
				if sp.IsRoot() {
					sp.handler(int(msg.Val))
				} else {
					sp.SendTo(sp.Parent(), &msg)
				}
				sp.Done()
				return nil
			}
		case <-sp.stop:
			sp.Done()
			return nil
		}
	}
}

// Client API request / response emulation
type SimpleRequest struct {
	ServerIdentities *Roster
	Val              int64
}

type SimpleResponse struct {
	Val int64
}

var SimpleResponseType = network.RegisterMessage(SimpleResponse{})

type simpleService struct {
	ctx      *Context
	panic    bool
	newProto chan bool
}

func (s *simpleService) ProcessClientRequest(req *http.Request, path string, buf []byte) ([]byte, *StreamingTunnel, error) {
	msg := &SimpleRequest{}
	err := protobuf.DecodeWithConstructors(buf, msg, network.DefaultConstructors(tSuite))
	if err != nil {
		return nil, nil, err
	}
	tree := msg.ServerIdentities.GenerateBinaryTree()
	tni := s.ctx.NewTreeNodeInstance(tree, tree.Root, backForthServiceName)
	ret := make(chan int)
	proto, err := newBackForthProtocolRoot(tni, int(msg.Val), func(n int) {
		ret <- n
	})
	if err != nil {
		return nil, nil, err
	}
	if err = s.ctx.RegisterProtocolInstance(proto); err != nil {
		return nil, nil, err
	}
	proto.Start()
	if s.panic {
		proto.(*BackForthProtocol).Done()
		close(ret)
	}
	resp, err := protobuf.Encode(&SimpleResponse{int64(<-ret)})
	return resp, nil, err
}

func (s *simpleService) NewProtocol(tni *TreeNodeInstance, conf *GenericConfig) (ProtocolInstance, error) {
	select {
	case s.newProto <- true:
	default:
	}
	if s.panic {
		panic("this is a panic in NewProtocol")
	}
	pi, err := newBackForthProtocol(tni)
	return pi, err
}

func (s *simpleService) Process(env *network.Envelope) {
	return
}

type DummyProtocol struct {
	*TreeNodeInstance
	link   chan bool
	config DummyConfig
}

type DummyConfig struct {
	A    int
	Send bool
}

type DummyMsg struct {
	A int64
}

var dummyMsgType network.MessageTypeID

func newDummyProtocol(tni *TreeNodeInstance, conf DummyConfig, link chan bool) *DummyProtocol {
	return &DummyProtocol{tni, link, conf}
}

func (dm *DummyProtocol) Start() error {
	dm.link <- true
	if dm.config.Send {
		// also send to the children if any
		if !dm.IsLeaf() {
			if err := dm.SendToChildren(&DummyMsg{}); err != nil {
				log.Error(err)
			}
		}
	}
	dm.Done()
	return nil
}

func (dm *DummyProtocol) ProcessProtocolMsg(msg *ProtocolMsg) {
	dm.link <- true
	dm.Done()
}

// legacy reasons
func (dm *DummyProtocol) Dispatch() error {
	return nil
}

type DummyService struct {
	c        *Context
	link     chan bool
	fakeTree *Tree
	firstTni *TreeNodeInstance
	Config   DummyConfig
}

func (ds *DummyService) ProcessClientRequest(req *http.Request, path string, buf []byte) ([]byte, *StreamingTunnel, error) {
	log.Lvl2("Got called with path", path, buf)
	msg := &DummyMsg{}
	err := protobuf.Decode(buf, msg)
	if err != nil {
		ds.link <- false
		return nil, nil, xerrors.New("wrong message")
	}
	if ds.firstTni == nil {
		ds.firstTni = ds.c.NewTreeNodeInstance(ds.fakeTree, ds.fakeTree.Root, dummyServiceName)
	}

	dp := newDummyProtocol(ds.firstTni, ds.Config, ds.link)

	if err := ds.c.RegisterProtocolInstance(dp); err != nil {
		ds.link <- false
		return nil, nil, err
	}
	log.Lvl2("Starting protocol")
	go func() {
		log.ErrFatal(dp.Start())
	}()
	return nil, nil, nil
}

func (ds *DummyService) NewProtocol(tn *TreeNodeInstance, conf *GenericConfig) (ProtocolInstance, error) {
	dp := newDummyProtocol(tn, DummyConfig{}, ds.link)
	return dp, nil
}

func (ds *DummyService) Process(env *network.Envelope) {
	if !env.MsgType.Equal(dummyMsgType) {
		ds.link <- false
		return
	}
	dms := env.Msg.(*DummyMsg)
	if dms.A != 10 {
		ds.link <- false
		return
	}
	ds.link <- true
}

type ServiceMessages struct {
	*ServiceProcessor
	GotResponse chan bool
}

func (i *ServiceMessages) SimpleResponse(env *network.Envelope) error {
	i.GotResponse <- true
	return nil
}

func newServiceMessages(c *Context) (Service, error) {
	s := &ServiceMessages{
		ServiceProcessor: NewServiceProcessor(c),
		GotResponse:      make(chan bool),
	}
	c.RegisterProcessorFunc(SimpleResponseType, s.SimpleResponse)
	return s, nil
}

type dummyService2 struct {
	*Context
	link chan bool
}

func newDummyService2(c *Context) (Service, error) {
	return &dummyService2{Context: c}, nil
}

func (ds *dummyService2) ProcessClientRequest(req *http.Request, path string, buf []byte) ([]byte, *StreamingTunnel, error) {
	panic("should not be called")
}

var serviceConfig = []byte{0x01, 0x02, 0x03, 0x04}

func (ds *dummyService2) NewProtocol(tn *TreeNodeInstance, conf *GenericConfig) (ProtocolInstance, error) {
	if conf != nil {
		log.Print(ds.ServerIdentity(), conf.Data)
	} else {
		log.Print(ds.ServerIdentity(), "has no configuration-data")
	}
	if tn.Parent() != nil{
		log.Print(ds.ServerIdentity(), "has parent", tn.Parent().Name())
	}
	ds.link <- conf != nil && bytes.Equal(conf.Data, serviceConfig)
	pi, err := newDummyProtocol2(tn)
	//pi.(*DummyProtocol2).finishEarly = true
	tn.SetConfig(conf)
	return pi, err
}

func (ds *dummyService2) Process(env *network.Envelope) {
	panic("should not be called")
}

func (ds *dummyService2) launchProto(t *Tree, config bool) {
	ds.launchProtoStart(t, config, false)
}

func (ds *dummyService2) launchProtoStart(t *Tree, config, startNew bool) {
	tni := ds.NewTreeNodeInstance(t, t.Root, dummyService2Name)
	pi, err := newDummyProtocol2(tni)
	pi.(*DummyProtocol2).startNewProtocol = startNew
	err2 := ds.RegisterProtocolInstance(pi)
	ds.link <- err == nil && err2 == nil

	if config {
		tni.SetConfig(&GenericConfig{serviceConfig})
	}
	go func() {
		log.ErrFatal(pi.Start())
	}()
}

type DummyProtocol2 struct {
	*TreeNodeInstance
	c                chan WrapDummyMsg
	startNewProtocol bool
	finishEarly      bool
}

type WrapDummyMsg struct {
	*TreeNode
	DummyMsg
}

func newDummyProtocol2(n *TreeNodeInstance) (ProtocolInstance, error) {
	d := &DummyProtocol2{TreeNodeInstance: n}
	d.c = make(chan WrapDummyMsg, 1)
	d.RegisterChannel(d.c)
	return d, nil
}

func (dp2 *DummyProtocol2) Start() error {
	if dp2.startNewProtocol {
		pi, err := dp2.CreateProtocol(dummyProtocolName, dp2.Tree())
		if err != nil {
			log.Error(err)
			return err
		}
		go pi.Start()
	}
	var children []string
	for _, child := range dp2.Children(){
		children = append(children, child.Name())
	}
	log.Print(dp2.ServerIdentity(), "sending to children", children)
	err := dp2.SendToChildren(&DummyMsg{20})
	dp2.Done()
	return err
}

func (dp2 *DummyProtocol2) Dispatch() error {
	if dp2.finishEarly {
		dp2.Done()
	}
	dm := <-dp2.c
	if len(dp2.Children()) > 0{
		log.Print(dp2.ServerIdentity(), "sending to children")
		if err := dp2.SendToChildren(&dm.DummyMsg); err != nil{
			return xerrors.Errorf("couldn't send to children: %+v", err)
		}
	}
	dp2.Done()
	return nil
}

func waitOrFatalValue(ch chan bool, v bool, t *testing.T) {
	select {
	case b := <-ch:
		if v != b {
			log.Fatal("Wrong value returned on channel")
		}
	case <-time.After(time.Second*10):
		log.Fatal("Waited too long")
	}

}

func waitOrFatal(ch chan bool, t *testing.T) {
	select {
	case _ = <-ch:
		return
	case <-time.After(time.Second):
		log.Fatal("Waited too long")
	}
}
