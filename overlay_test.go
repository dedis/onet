package onet

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/onet/v3/log"
	"go.dedis.ch/onet/v3/network"
)

// A checkableError is a type that implements error and also lets
// you find out, by reading on a channel, how many times it has been
// formatted using Error().
type checkableError struct {
	ch  chan struct{}
	msg string
}

func (ce *checkableError) Error() string {
	ce.ch <- struct{}{}
	return ce.msg
}

var dispFailErr = &checkableError{
	ch:  make(chan struct{}, 10),
	msg: "Dispatch failed",
}

type ProtocolOverlay struct {
	*TreeNodeInstance
	done         bool
	failDispatch bool
	failChan     chan bool
}

func (po *ProtocolOverlay) Start() error {
	// no need to do anything
	return nil
}

func (po *ProtocolOverlay) Dispatch() error {
	if po.failDispatch {
		return dispFailErr
	}
	return nil
}

func (po *ProtocolOverlay) Release() {
	// call the Done function
	po.Done()
}

func TestOverlayDispatchFailure(t *testing.T) {
	// setup
	failChan := make(chan bool, 1)
	fn := func(n *TreeNodeInstance) (ProtocolInstance, error) {
		ps := ProtocolOverlay{
			TreeNodeInstance: n,
			failDispatch:     true,
			failChan:         failChan,
		}
		return &ps, nil
	}
	GlobalProtocolRegister("ProtocolOverlay", fn)
	local := NewLocalTest(tSuite)
	defer local.CloseAll()

	// Redirect output so we can check for the failure
	log.OutputToBuf()
	defer log.OutputToOs()

	h, _, tree := local.GenTree(1, true)
	h1 := h[0]
	pi, err := h1.CreateProtocol("ProtocolOverlay", tree)
	if err != nil {
		t.Fatal("error starting new node", err)
	}

	// wait for the error message to get formatted by overlay.go
	<-dispFailErr.ch

	// This test was apparently always a bit fragile, and commit 5931349
	// seems to have made it worse. Adding this tiny sleep makes
	// 2000 iterations pass where before I could see errors about 1 in 20 times.
	time.Sleep(5 * time.Millisecond)

	// when using `go test -v`, the error string goes into the stderr buffer
	// but with `go test`, it goes into the stdout buffer, so we check both
	assert.Contains(t, log.GetStdOut()+log.GetStdErr(), "Dispatch failed")
	pi.(*ProtocolOverlay).Done()
}

func TestOverlayDone(t *testing.T) {
	log.OutputToBuf()
	defer log.OutputToOs()

	// setup
	fn := func(n *TreeNodeInstance) (ProtocolInstance, error) {
		ps := ProtocolOverlay{
			TreeNodeInstance: n,
		}
		return &ps, nil
	}
	GlobalProtocolRegister("ProtocolOverlay", fn)
	local := NewLocalTest(tSuite)
	defer local.CloseAll()
	h, _, tree := local.GenTree(1, true)
	h1 := h[0]
	p, err := h1.CreateProtocol("ProtocolOverlay", tree)
	if err != nil {
		t.Fatal("error starting new node", err)
	}
	po := p.(*ProtocolOverlay)
	// release the resources
	var count int
	po.OnDoneCallback(func() bool {
		count++
		if count >= 2 {
			return true
		}
		return false
	})
	po.Release()
	overlay := h1.overlay
	if _, ok := overlay.TokenToNode(po.Token()); !ok {
		t.Fatal("Node should exists after first call Done()")
	}
	po.Release()
	if _, ok := overlay.TokenToNode(po.Token()); ok {
		t.Fatal("Node should NOT exists after call Done()")
	}
}

type protocolCatastrophic struct {
	*TreeNodeInstance

	ChannelMsg chan WrapDummyMsg

	done chan bool
}

func (po *protocolCatastrophic) Start() error {
	panic("start panic")
}

func (po *protocolCatastrophic) Dispatch() error {
	if !po.IsRoot() {
		<-po.ChannelMsg

		po.SendToParent(&DummyMsg{})

		po.Done()
		panic("dispatch panic")
	}

	err := po.SendToChildren(&DummyMsg{})
	if err != nil {
		return err
	}

	<-po.ChannelMsg
	<-po.ChannelMsg
	po.done <- true

	po.Done()
	panic("root dispatch panic")
}

// TestOverlayCatastrophicFailure checks if a panic during a protocol could
// cause the server to crash
func TestOverlayCatastrophicFailure(t *testing.T) {
	log.OutputToBuf()
	defer log.OutputToOs()

	fn := func(n *TreeNodeInstance) (ProtocolInstance, error) {
		ps := protocolCatastrophic{
			TreeNodeInstance: n,
			done:             make(chan bool),
		}

		err := ps.RegisterChannel(&ps.ChannelMsg)

		return &ps, err
	}
	GlobalProtocolRegister("ProtocolCatastrophic", fn)
	local := NewLocalTest(tSuite)
	defer local.CloseAll()

	h, _, tree := local.GenTree(3, true)
	h1 := h[0]
	pi, err := h1.StartProtocol("ProtocolCatastrophic", tree)
	assert.NoError(t, err)

	<-pi.(*protocolCatastrophic).done

	// can't have a synchronisation and a panic so we wait for the panic to be handled
	time.Sleep(1 * time.Second)

	stderr := log.GetStdErr()
	assert.Contains(t, stderr, "Start(): start panic")
	assert.Contains(t, stderr, "Panic in call to protocol")
	assert.Contains(t, stderr, "Dispatch(): root dispatch panic")
}

// Test when a peer receives a New Roster, it can create the trees that are
// waiting on this specific entitiy list, to be constructed.
func TestOverlayPendingTreeMarshal(t *testing.T) {
	local := NewLocalTest(tSuite)
	hosts, el, tree := local.GenTree(2, false)
	defer local.CloseAll()
	h1 := hosts[0]

	// Add the marshalled version of the tree
	local.addPendingTreeMarshal(h1, tree.MakeTreeMarshal())
	if _, ok := h1.GetTree(tree.ID); ok {
		t.Fatal("host 1 should not have the tree definition yet.")
	}
	// Now make it check
	local.checkPendingTreeMarshal(h1, el)
	if _, ok := h1.GetTree(tree.ID); !ok {
		t.Fatal("Host 1 should have the tree definition now.")
	}
}

// overlayProc is a Processor which handles the management packet of Overlay,
// i.e. Roster & Tree management.
// Each type of message will be sent trhough the appropriate channel
type overlayProc struct {
	sendRoster   chan *Roster
	responseTree chan *ResponseTree
	treeMarshal  chan *TreeMarshal
	requestTree  chan *RequestTree
}

func newOverlayProc() *overlayProc {
	return &overlayProc{
		sendRoster:   make(chan *Roster, 1),
		responseTree: make(chan *ResponseTree, 1),
		treeMarshal:  make(chan *TreeMarshal, 1),
		requestTree:  make(chan *RequestTree, 1),
	}
}

func (op *overlayProc) Process(env *network.Envelope) {
	switch env.MsgType {
	case SendRosterMsgID:
		op.sendRoster <- env.Msg.(*Roster)
	case ResponseTreeMsgID:
		op.responseTree <- env.Msg.(*ResponseTree)
	case SendTreeMsgID:
		op.treeMarshal <- env.Msg.(*TreeMarshal)
	case RequestTreeMsgID:
		op.requestTree <- env.Msg.(*RequestTree)
	}
}

func (op *overlayProc) Types() []network.MessageTypeID {
	return []network.MessageTypeID{TreeMarshalTypeID, SendRosterMsgID}
}

// Test propagation of roster - both known and unknown
// Deprecated: check the deprecation is still working
func TestOverlayRosterPropagation(t *testing.T) {
	local := NewLocalTest(tSuite)
	hosts, el, tree := local.GenTree(2, false)
	defer local.CloseAll()
	h1 := hosts[0]
	h2 := hosts[1]
	proc := newOverlayProc()
	h1.RegisterProcessor(proc, proc.Types()...)

	// Check that h2 sends back an empty list if it is unknown
	sentLen, err := h1.Send(h2.ServerIdentity, &RequestRoster{RosterID: el.ID})
	require.Nil(t, err, "Couldn't send message to h1")
	require.NotZero(t, sentLen)

	roster := <-proc.sendRoster
	if !roster.ID.IsNil() {
		t.Fatal("List should be empty")
	}

	// Now add the tree to h2 and try again
	h2.AddTree(tree)
	sentLen, err = h1.Send(h2.ServerIdentity, &RequestRoster{RosterID: el.ID})
	require.Nil(t, err, "Couldn't send message to h2")
	require.NotZero(t, sentLen)

	msg := <-proc.sendRoster
	if !msg.ID.Equal(el.ID) {
		t.Fatal("List should be equal to original list")
	}

	sentLen, err = h1.Send(h2.ServerIdentity, &RequestRoster{RosterID: el.ID})
	require.Nil(t, err, "Couldn't send message to h2")
	require.NotZero(t, sentLen)
}

// Test propagation of tree - both known and unknown
func TestOverlayTreePropagation(t *testing.T) {
	local := NewLocalTest(tSuite)
	hosts, _, tree := local.GenTree(2, false)
	defer local.CloseAll()
	h1 := hosts[0]
	h2 := hosts[1]

	proc := newOverlayProc()
	h1.RegisterProcessor(proc, ResponseTreeMsgID)
	// h1 needs to expect the tree
	h1.Overlay().treeStorage.Register(tree.ID)

	// Check that h2 does nothing and doesn't crash
	sentLen, err := h1.Send(h2.ServerIdentity, &RequestTree{TreeID: tree.ID, Version: 1})
	require.Nil(t, err, "Couldn't send message to h2")
	require.NotZero(t, sentLen)

	// Now add the list to h2 and try again
	h2.AddTree(tree)
	sentLen, err = h1.Send(h2.ServerIdentity, &RequestTree{TreeID: tree.ID, Version: 1})
	require.Nil(t, err)
	require.NotZero(t, sentLen)

	msg := <-proc.responseTree
	assert.Equal(t, msg.TreeMarshal.TreeID, tree.ID)

	sentLen, err = h1.Send(h2.ServerIdentity, &RequestTree{TreeID: tree.ID, Version: 1})
	require.Nil(t, err)
	require.NotZero(t, sentLen)

	// check if we receive the tree then
	var tm *ResponseTree
	tm = <-proc.responseTree
	packet := network.Envelope{
		ServerIdentity: h2.ServerIdentity,
		Msg:            tm,
		MsgType:        SendTreeMsgID,
	}
	h1.overlay.Process(&packet)

	tree2, ok := h1.GetTree(tree.ID)
	if !ok {
		t.Fatal("List-id not found")
	}
	if !tree.Equal(tree2) {
		t.Fatal("Trees do not match")
	}
}

// Tests if a tree can be requested even after a failure
func TestOverlayTreeFailure(t *testing.T) {
	local := NewLocalTest(tSuite)
	hosts, _, tree := local.GenTree(3, false)
	defer local.CloseAll()

	h1 := hosts[0]
	h1.overlay.treeStorage.Register(tree.ID)
	h2 := hosts[1]
	h2.AddTree(tree)
	h3 := hosts[2]
	h3.Close()

	proc := newOverlayProc()
	h1.RegisterProcessor(proc, ResponseTreeMsgID)

	_, err := h1.Send(h3.ServerIdentity, &RequestTree{TreeID: tree.ID, Version: 1})
	require.NotNil(t, err)

	_, err = h1.Send(h2.ServerIdentity, &RequestTree{TreeID: tree.ID, Version: 1})
	require.Nil(t, err)

	// check if we have the tree
	treeM := <-proc.responseTree
	require.NotNil(t, treeM)
}

// Tests a tree propagation with an unknown and known roster
// Deprecated: check the deprecation is still working
func TestOverlayRosterTreePropagation(t *testing.T) {
	local := NewLocalTest(tSuite)
	hosts, ro, tree := local.GenTree(2, false)
	defer local.CloseAll()
	h1 := hosts[0]
	h1.Overlay().treeStorage.Register(tree.ID)
	h2 := hosts[1]

	// and the tree
	h2.AddTree(tree)
	// make the communcation happen
	sentLen, err := h1.Send(h2.ServerIdentity, &RequestTree{TreeID: tree.ID})
	require.Nil(t, err, "Could not send tree request to host2")
	require.NotZero(t, sentLen)

	proc := newOverlayProc()
	h1.RegisterProcessor(proc, SendTreeMsgID, SendRosterMsgID)

	// check if we have the tree
	treeM := <-proc.treeMarshal

	packet := network.Envelope{
		ServerIdentity: h2.ServerIdentity,
		Msg:            treeM,
		MsgType:        SendTreeMsgID,
	}
	// give it to overlay
	h1.overlay.Process(&packet)
	// the tree should not be there because we don't have the Roster associated
	// yet
	if _, ok := h1.GetTree(tree.ID); ok {
		t.Fatal("Tree should Not be there")
	}
	// check if we receive the Roster then
	roster := <-proc.sendRoster

	packet = network.Envelope{
		ServerIdentity: h2.ServerIdentity,
		Msg:            roster,
		MsgType:        SendRosterMsgID,
	}
	h1.overlay.Process(&packet)

	// check if we have the roster now  & the tree
	if _, ok := h1.Roster(ro.ID); !ok {
		t.Fatal("Roster should be here")
	}
	if _, ok := h1.GetTree(tree.ID); !ok {
		t.Fatal("Tree should be there")
	}

	// check it can get the tree without requesting the roster again
	tree2 := ro.GenerateNaryTreeWithRoot(2, ro.List[1])
	require.False(t, tree2.ID.Equal(tree.ID))

	packet = network.Envelope{
		ServerIdentity: h2.ServerIdentity,
		Msg:            tree2.MakeTreeMarshal(),
		MsgType:        SendTreeMsgID,
	}

	h1.overlay.instances[TokenID{}] = &TreeNodeInstance{overlay: h1.overlay, token: &Token{TreeID: tree.ID}}
	h1.overlay.treeStorage.Register(tree2.ID)
	h1.overlay.Process(&packet)
	if _, ok := h1.GetTree(tree2.ID); !ok {
		t.Fatal("Tree should be there")
	}
}

// Tests that the tree is not registered when bad parameters are provided
func TestOverlayHandlersBadParameters(t *testing.T) {
	local := NewLocalTest(tSuite)
	hosts, ro, tree := local.GenTree(1, false)
	defer local.CloseAll()
	h := hosts[0]

	h.overlay.handleSendTree(h.ServerIdentity, &ResponseTree{}, nil)
	h.overlay.handleSendTree(h.ServerIdentity, &ResponseTree{TreeMarshal: tree.MakeTreeMarshal()}, nil)
	h.overlay.handleSendTree(h.ServerIdentity, &ResponseTree{TreeMarshal: tree.MakeTreeMarshal(), Roster: ro}, nil)

	h.overlay.handleSendTreeMarshal(h.ServerIdentity, &TreeMarshal{}, nil)
	h.overlay.handleSendTreeMarshal(h.ServerIdentity, tree.MakeTreeMarshal(), nil)
	require.Equal(t, 0, len(h.overlay.treeStorage.trees))

	h.overlay.handleSendRoster(h.ServerIdentity, &Roster{})
}

func TestTokenId(t *testing.T) {
	t1 := &Token{
		RosterID: RosterID(uuid.Must(uuid.NewUUID())),
		TreeID:   TreeID(uuid.Must(uuid.NewUUID())),
		ProtoID:  ProtocolID(uuid.Must(uuid.NewUUID())),
		RoundID:  RoundID(uuid.Must(uuid.NewUUID())),
	}
	id1 := t1.ID()
	t2 := &Token{
		RosterID: RosterID(uuid.Must(uuid.NewUUID())),
		TreeID:   TreeID(uuid.Must(uuid.NewUUID())),
		ProtoID:  ProtocolID(uuid.Must(uuid.NewUUID())),
		RoundID:  RoundID(uuid.Must(uuid.NewUUID())),
	}
	id2 := t2.ID()
	if id1.Equal(id2) {
		t.Fatal("Both token are the same")
	}
	if !id1.Equal(t1.ID()) {
		t.Fatal("Twice the Id of the same token should be equal")
	}
	t3 := t1.ChangeTreeNodeID(TreeNodeID(uuid.Must(uuid.NewUUID())))
	if t1.TreeNodeID.Equal(t3.TreeNodeID) {
		t.Fatal("OtherToken should modify copy")
	}
}

type testNilService struct{}

func (s testNilService) NewProtocol(tni *TreeNodeInstance, cfg *GenericConfig) (ProtocolInstance, error) {
	if cfg == nil {
		return nil, errors.New("config should not be nil")
	}

	c := make(chan bool, 1)
	return &DummyProtocol{TreeNodeInstance: tni, link: c}, nil
}

func (s testNilService) Process(*network.Envelope) {}

func (s testNilService) ProcessClientRequest(req *http.Request, handler string, msg []byte) (reply []byte, tunnel *StreamingTunnel, err error) {
	return nil, nil, nil
}

func TestOverlay_ConfigInMessage(t *testing.T) {
	local := NewLocalTest(tSuite)
	hosts, ro, _ := local.GenTree(1, false)
	defer local.CloseAll()
	h := hosts[0]

	tree := NewTree(ro, NewTreeNode(0, ro.List[0]))
	h.overlay.treeStorage.Set(tree)

	h.serviceManager.services[NilServiceID] = testNilService{}

	cfg := &GenericConfig{Data: []byte("deadbeef")}

	io := &defaultProtoIO{suite: tSuite}
	om := &OverlayMsg{
		Config: cfg,
		TreeNodeInfo: &TreeNodeInfo{
			To: &Token{
				TreeNodeID: tree.Root.ID,
				TreeID:     tree.ID,
			},
		},
	}

	env, err := io.Wrap(ro, om)
	require.NoError(t, err)
	require.NotNil(t, env.(*ProtocolMsg).Config)

	env.(*ProtocolMsg).ServerIdentity = &network.ServerIdentity{}

	err = h.overlay.TransmitMsg(env.(*ProtocolMsg), io)
	require.NoError(t, err)
}
