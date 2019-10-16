package onet

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/onet/v4/log"
	"go.dedis.ch/onet/v4/network"
)

func init() {
	GlobalProtocolRegister(spawnName, newSpawnProto)
	GlobalProtocolRegister(pingPongProtoName, newPingPongProto)
}

func TestTreeNodeInstance_KeyPairs(t *testing.T) {
	local := NewLocalTest(testSuite)
	defer local.CloseAll()

	_, _, tree := local.GenTree(5, true)
	tni, err := local.NewTreeNodeInstance(tree.Root, spawnName)

	require.NoError(t, err)
	require.True(t, tni.SecretKey().Pack().Equal(tni.Host().secretKey.Pack()))
	require.True(t, tni.PublicKey().Pack().Equal(tni.Host().ServerIdentity.PublicKey))
	require.True(t, tni.PublicKey().Pack().Equal(tni.NodePublic(tni.Host().ServerIdentity).Pack()))
	keys, err := tni.PublicKeys()
	require.NoError(t, err)
	require.Equal(t, 5, len(keys))
}

func TestTreeNodeCreateProtocol(t *testing.T) {
	local := NewLocalTest(testSuite)
	defer local.CloseAll()

	hosts, _, tree := local.GenTree(1, true)
	pi, err := hosts[0].overlay.CreateProtocol(spawnName, tree, NilServiceID)
	log.ErrFatal(err)
	p := pi.(*spawnProto)
	p.spawn = true
	go p.Start()

	// wait once for the protocol just created
	<-spawnCh
	// wait once more for the protocol created inside the first one
	<-spawnCh

	pi.(*spawnProto).Done()
}

func TestTreeNodeRxTx(t *testing.T) {
	local := NewTCPTest(testSuite)
	testTreeNodeRxTx(t, local)
	local.CloseAll()

	local = NewLocalTest(testSuite)
	testTreeNodeRxTx(t, local)
	local.CloseAll()
}

func testTreeNodeRxTx(t *testing.T, local *LocalTest) {
	_, _, tree := local.GenTree(2, true)
	pi, err := local.StartProtocol(pingPongProtoName, tree)
	require.Nil(t, err)
	protocol := pi.(*pingPongProto)

	<-protocol.done
	require.NotZero(t, protocol.TreeNodeInstance.Rx())
	require.NotZero(t, protocol.TreeNodeInstance.Tx())
}

func TestHandlerReturn(t *testing.T) {
	local := NewLocalTest(testSuite)
	defer local.CloseAll()

	hosts, _, tree := local.GenTree(1, true)
	pi, err := hosts[0].overlay.CreateProtocol(spawnName, tree, NilServiceID)
	log.ErrFatal(err)
	p := pi.(*spawnProto)
	assert.NotNil(t, p.RegisterHandler(p.HandlerError1))
	assert.Nil(t, p.RegisterHandler(p.HandlerError2))
	assert.NotNil(t, p.RegisterHandler(p.HandlerError3))
	p.Done()
}

type dummyMsg struct{}

type configProcessor struct {
	configCount int
	expected    int
	done        chan<- bool
	sync.Mutex
}

func (p *configProcessor) Process(env *network.Envelope) {
	p.Lock()
	if env.MsgType == ConfigMsgID {
		p.configCount++
		if p.configCount == p.expected {
			p.done <- true
		}
	}
	p.Unlock()
}

func TestConfigPropagation(t *testing.T) {
	local := NewLocalTest(testSuite)
	defer local.CloseAll()
	const treeSize = 3
	var serviceConfig = []byte{0x01, 0x02, 0x03, 0x04}
	hosts, _, tree := local.GenTree(treeSize, true)
	pi, err := hosts[0].overlay.CreateProtocol(spawnName, tree, NilServiceID)
	log.ErrFatal(err)

	done := make(chan bool)
	pr := &configProcessor{expected: treeSize - 1, done: done}

	for _, host := range hosts {
		host.RegisterProcessor(pr,
			ProtocolMsgID,
			RequestTreeMsgID,
			ConfigMsgID)
	}

	network.RegisterMessage(dummyMsg{})
	rootInstance, _ := local.NewTreeNodeInstance(tree.Root, spawnName)
	require.Zero(t, rootInstance.Tx())
	require.Zero(t, rootInstance.Rx())

	err = rootInstance.SetConfig(&GenericConfig{serviceConfig})
	assert.Nil(t, err)
	err = rootInstance.SetConfig(&GenericConfig{serviceConfig})
	assert.NotNil(t, err)

	err = rootInstance.SendToChildren(&dummyMsg{})
	assert.NotZero(t, rootInstance.Tx())
	log.ErrFatal(err)
	// wait until the processor has processed the expected number of config messages
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Didn't receive response in time")
	}
	pi.(*spawnProto).Done()
}

func TestTreeNodeInstance_RegisterChannel(t *testing.T) {
	local := NewLocalTest(testSuite)
	defer local.CloseAll()

	_, _, tree := local.GenTree(3, true)
	ri, err := local.NewTreeNodeInstance(tree.Root, spawnName)
	log.ErrFatal(err)

	var c chan spawnMsg
	log.ErrFatal(ri.RegisterChannel(&c))

	m := &ProtocolMsg{
		MsgType: network.RegisterMessage(&spawn{}),
		From: &Token{
			TreeNodeID: ri.treeNode.ID,
		},
		Msg: &spawn{I: 10},
	}
	msg := []*ProtocolMsg{}
	for i := 0; i < 101; i++ {
		msg = append(msg, m)
	}
	require.NotNil(t, ri.dispatchChannel(msg))
	log.ErrFatal(ri.RegisterChannelLength(&c, 200))
	log.ErrFatal(ri.dispatchChannel(msg))
}

// spawnCh is used to dispatch information from a spawnProto to the test
var spawnCh = make(chan bool)

const spawnName = "Spawn"

// spawnProto is a simple protocol which just spawn another protocol when
// started
type spawnProto struct {
	*TreeNodeInstance
	spawn bool
}

func newSpawnProto(tn *TreeNodeInstance) (ProtocolInstance, error) {
	sp := &spawnProto{
		TreeNodeInstance: tn,
	}
	return sp, nil
}

func (s *spawnProto) Start() error {
	defer s.Done()
	r := s.Roster()
	tree := r.GenerateBinaryTree()
	spawnCh <- true
	if !s.spawn {
		return nil
	}
	proto, err := s.CreateProtocol(spawnName, tree)
	log.ErrFatal(err)
	go proto.Start()
	return nil
}

type spawn struct {
	I int64
}

type spawnMsg struct {
	*TreeNode
	M spawn
}

// Invalid handler
func (s *spawnProto) HandlerError1(msg spawnMsg) {}

// Valid handler
func (s *spawnProto) HandlerError2(msg spawnMsg) error {
	s.Done()
	return nil
}

// Invalid handler
func (s *spawnProto) HandlerError3(msg spawnMsg) (int, error) {
	return 0, nil
}

// Simple protocol to have messages transmit between nodes
const pingPongProtoName = "PingPongProtoTest"

type PingPongMsg struct{}

type pingPongProto struct {
	*TreeNodeInstance
	done         chan bool
	pingPongChan chan struct {
		*TreeNode
		PingPongMsg
	}
}

func newPingPongProto(tn *TreeNodeInstance) (ProtocolInstance, error) {
	cp := &pingPongProto{
		TreeNodeInstance: tn,
		done:             make(chan bool, len(tn.List())),
	}
	err := cp.RegisterChannelsLength(len(tn.Tree().List()), &cp.pingPongChan)
	return cp, err
}

func (cp *pingPongProto) Dispatch() error {
	defer cp.Done()

	select {
	case <-cp.pingPongChan:
		if !cp.IsRoot() {
			cp.SendToParent(&PingPongMsg{})
		}
	}

	cp.done <- true
	return nil
}

func (cp *pingPongProto) Start() error {
	// only called by the root
	err := cp.SendToChildren(&PingPongMsg{})
	if err != nil {
		return err
	}

	return nil
}
