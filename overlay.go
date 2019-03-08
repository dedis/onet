package onet

import (
	"errors"
	"fmt"
	"sync"

	"go.dedis.ch/onet/v3/log"
	"go.dedis.ch/onet/v3/network"
	"gopkg.in/satori/go.uuid.v1"
)

// Overlay keeps all trees and entity-lists for a given Server. It creates
// Nodes and ProtocolInstances upon request and dispatches the messages.
type Overlay struct {
	server *Server

	treeStorage     map[TreeID]*Tree
	treeStorageLock sync.Mutex

	// TreeNodeInstance part
	instances         map[TokenID]*TreeNodeInstance
	instancesInfo     map[TokenID]bool
	instancesLock     sync.Mutex
	protocolInstances map[TokenID]ProtocolInstance

	// treeMarshal that needs to be converted to Tree but host does not have the
	// entityList associated yet.
	// map from Roster.ID => trees that use this entity list
	pendingTreeMarshal map[RosterID][]*TreeMarshal
	// lock associated with pending TreeMarshal
	pendingTreeLock sync.Mutex

	// pendingMsg is a list of message we received that does not correspond
	// to any local Tree or/and Roster. We first request theses so we can
	// instantiate properly protocolInstance that will use these ProtocolMsg msg.
	pendingMsg []pendingMsg
	// lock associated with pending ProtocolMsg
	pendingMsgLock sync.Mutex

	transmitMux sync.Mutex

	protoIO *messageProxyStore

	pendingConfigs    map[TokenID]*GenericConfig
	pendingConfigsMut sync.Mutex
}

// NewOverlay creates a new overlay-structure
func NewOverlay(c *Server) *Overlay {
	o := &Overlay{
		server:             c,
		treeStorage:        make(map[TreeID]*Tree),
		instances:          make(map[TokenID]*TreeNodeInstance),
		instancesInfo:      make(map[TokenID]bool),
		protocolInstances:  make(map[TokenID]ProtocolInstance),
		pendingTreeMarshal: make(map[RosterID][]*TreeMarshal),
		pendingConfigs:     make(map[TokenID]*GenericConfig),
	}
	o.protoIO = newMessageProxyStore(c.suite, c, o)
	// messages going to protocol instances
	c.RegisterProcessor(o,
		ProtocolMsgID,    // protocol instance's messages
		RequestTreeMsgID, // request a tree
		SendTreeMsgID,    // send a tree back to a request
		ConfigMsgID)      // fetch config information
	return o
}

// Process implements the Processor interface so it process the messages that it
// wants.
func (o *Overlay) Process(env *network.Envelope) {
	// Messages handled by the overlay directly without any messageProxyIO
	if env.MsgType.Equal(ConfigMsgID) {
		o.handleConfigMessage(env)
		return
	}

	// get messageProxy or default one
	io := o.protoIO.getByPacketType(env.MsgType)
	inner, info, err := io.Unwrap(env.Msg)
	if err != nil {
		log.Error("unwrapping: ", err)
		return
	}
	switch {
	case info.RequestTree != nil:
		o.handleRequestTree(env.ServerIdentity, info.RequestTree, io)
	case info.ResponseTree != nil:
		o.handleSendTree(env.ServerIdentity, info.ResponseTree, io)
	default:
		typ := network.MessageType(inner)
		protoMsg := &ProtocolMsg{
			From:           info.TreeNodeInfo.From,
			To:             info.TreeNodeInfo.To,
			ServerIdentity: env.ServerIdentity,
			Msg:            inner,
			MsgType:        typ,
			Size:           env.Size,
		}
		err = o.TransmitMsg(protoMsg, io)
		if err != nil {
			log.Errorf("Msg %s from %s produced error: %s", protoMsg.MsgType,
				protoMsg.ServerIdentity, err.Error())
		}
	}
}

// TransmitMsg takes a message received from the host and treats it. It might
// - ask for the identityList
// - ask for the Tree
// - create a new protocolInstance
// - pass it to a given protocolInstance
// io is the messageProxy to use if a specific wireformat protocol is used.
// It can be nil: in that case it fall backs to default wire protocol.
func (o *Overlay) TransmitMsg(onetMsg *ProtocolMsg, io MessageProxy) error {
	o.treeStorageLock.Lock()
	_, ok := o.treeStorage[onetMsg.To.TreeID]
	o.treeStorageLock.Unlock()
	if !ok {
		return o.requestTree(onetMsg.ServerIdentity, onetMsg, io)
	}

	o.transmitMux.Lock()
	defer o.transmitMux.Unlock()
	// TreeNodeInstance
	var pi ProtocolInstance
	o.instancesLock.Lock()
	pi, ok = o.protocolInstances[onetMsg.To.ID()]
	done := o.instancesInfo[onetMsg.To.ID()]
	o.instancesLock.Unlock()
	if done {
		log.Lvl5("Message for TreeNodeInstance that is already finished")
		return nil
	}
	// if the TreeNodeInstance is not there, creates it
	if !ok {
		log.Lvlf4("Creating TreeNodeInstance at %s %x", o.server.ServerIdentity, onetMsg.To.ID())
		tn, err := o.TreeNodeFromToken(onetMsg.To)
		if err != nil {
			return errors.New("No TreeNode defined in this tree here")
		}
		tni := o.newTreeNodeInstanceFromToken(tn, onetMsg.To, io)
		// retrieve the possible generic config for this message
		config := o.getConfig(onetMsg.To.ID())
		// request the PI from the Service and binds the two
		pi, err = o.server.serviceManager.newProtocol(tni, config)
		if err != nil {
			o.instancesLock.Lock()
			o.nodeDelete(onetMsg.To)
			o.instancesLock.Unlock()
			return err
		}
		if pi == nil {
			return nil
		}
		go func() {
			defer func() {
				if r := recover(); r != nil {
					svc := ServiceFactory.Name(tni.Token().ServiceID)
					log.Errorf("Panic in %v %s.Dispatch(): %v", svc, o.server.ServerIdentity, r)
				}
			}()

			err := pi.Dispatch()
			if err != nil {
				svc := ServiceFactory.Name(tni.Token().ServiceID)
				log.Errorf("%v %s.Dispatch() returned error %s", o.server.ServerIdentity, svc, err)
			}
		}()
		if err := o.RegisterProtocolInstance(pi); err != nil {
			return errors.New("Error Binding TreeNodeInstance and ProtocolInstance:" +
				err.Error())
		}
		log.Lvl4(o.server.Address(), "Overlay created new ProtocolInstace msg => ",
			fmt.Sprintf("%+v", onetMsg.To))
	}
	// TODO Check if TreeNodeInstance is already Done
	pi.ProcessProtocolMsg(onetMsg)
	return nil
}

// addPendingTreeMarshal adds a treeMarshal to the list.
// This list is checked each time we receive a new Roster
// so trees using this Roster can be constructed.
func (o *Overlay) addPendingTreeMarshal(tm *TreeMarshal) {
	o.pendingTreeLock.Lock()
	var sl []*TreeMarshal
	var ok bool
	// initiate the slice before adding
	if sl, ok = o.pendingTreeMarshal[tm.RosterID]; !ok {
		sl = make([]*TreeMarshal, 0)
	}
	sl = append(sl, tm)
	o.pendingTreeMarshal[tm.RosterID] = sl
	o.pendingTreeLock.Unlock()
}

// checkPendingMessages is called each time we receive a new tree if there are
// some pending ProtocolMessage messages using this tree. If there are, we can
// make an instance of a protocolinstance and give it the message.
func (o *Overlay) checkPendingMessages(t *Tree) {
	// This goroutine has no recover because the underlying code should never panic
	// and TransmitMsg does its own recovering
	go func() {
		o.pendingMsgLock.Lock()

		var newPending []pendingMsg
		var remaining []pendingMsg
		// Keep msg not related to that tree in the pending list
		for _, msg := range o.pendingMsg {
			if t.ID.Equal(msg.To.TreeID) {
				remaining = append(remaining, msg)
			} else {
				newPending = append(newPending, msg)
			}
		}

		o.pendingMsg = newPending
		o.pendingMsgLock.Unlock()

		for _, msg := range remaining {
			err := o.TransmitMsg(msg.ProtocolMsg, msg.MessageProxy)
			if err != nil {
				log.Error("TransmitMsg failed:", err)
				continue
			}
		}
	}()
}

func (o *Overlay) savePendingMsg(onetMsg *ProtocolMsg, io MessageProxy) {
	o.pendingMsgLock.Lock()
	o.pendingMsg = append(o.pendingMsg, pendingMsg{
		ProtocolMsg:  onetMsg,
		MessageProxy: io,
	})
	o.pendingMsgLock.Unlock()

}

// requestTree will ask for the tree the ProtocolMessage is related to.
// it will put the message inside the pending list of ProtocolMessage waiting to
// have their trees.
// io is the wrapper to use to send the message, it can be nil.
func (o *Overlay) requestTree(si *network.ServerIdentity, onetMsg *ProtocolMsg, io MessageProxy) error {
	o.savePendingMsg(onetMsg, io)

	var msg interface{}
	om := &OverlayMsg{
		RequestTree: &RequestTree{onetMsg.To.TreeID},
	}
	msg, err := io.Wrap(nil, om)
	if err != nil {
		return err
	}

	// no need to record sentLen because Overlay uses Server's CounterIO
	_, err = o.server.Send(si, msg)
	return err
}

// RegisterTree takes a tree and puts it in the map
func (o *Overlay) RegisterTree(t *Tree) {
	o.treeStorageLock.Lock()
	o.treeStorage[t.ID] = t
	o.treeStorageLock.Unlock()

	o.checkPendingMessages(t)
}

// TreeNodeFromToken returns the treeNode corresponding to a token
func (o *Overlay) TreeNodeFromToken(t *Token) (*TreeNode, error) {
	o.treeStorageLock.Lock()
	defer o.treeStorageLock.Unlock()

	if t == nil {
		return nil, errors.New("didn't find tree-node: No token given")
	}
	tree, ok := o.treeStorage[t.TreeID]
	if !ok {
		return nil, errors.New("didn't find tree")
	}
	tn := tree.Search(t.TreeNodeID)
	if tn == nil {
		return nil, errors.New("didn't find treenode")
	}

	return tn, nil
}

// Rx implements the CounterIO interface, should be the same as the server
func (o *Overlay) Rx() uint64 {
	return o.server.Rx()
}

// Tx implements the CounterIO interface, should be the same as the server
func (o *Overlay) Tx() uint64 {
	return o.server.Tx()
}

func (o *Overlay) handleRequestTree(si *network.ServerIdentity, req *RequestTree, io MessageProxy) {
	o.treeStorageLock.Lock()
	tree, ok := o.treeStorage[req.TreeID]
	o.treeStorageLock.Unlock()
	if !ok {
		// XXX Take care here for we must verify at the other side that
		// the tree is Nil. Should we think of a way of sending back an
		// "error" ?
		log.Error("couldn't find the tree")
		return
	}

	treeM := tree.MakeTreeMarshal()

	msg, err := io.Wrap(nil, &OverlayMsg{
		ResponseTree: &ResponseTree{
			TreeMarshal: treeM,
			Roster:      tree.Roster,
		},
	})

	if err != nil {
		log.Error("couldn't wrap TreeMarshal:", err)
		return
	}

	_, err = o.server.Send(si, msg)
	if err != nil {
		log.Error("Couldn't send tree:", err)
	}
}

func (o *Overlay) handleSendTree(si *network.ServerIdentity, rt *ResponseTree, io MessageProxy) {
	if rt.TreeMarshal == nil || rt.TreeMarshal.TreeID.IsNil() {
		log.Error("Received an empty Tree")
		return
	}

	if rt.Roster == nil {
		log.Error("received an empty roster")
		return
	}

	tree, err := rt.TreeMarshal.MakeTree(rt.Roster)
	if err != nil {
		log.Error("Couldn't create tree:", err)
		return
	}
	log.Lvl4("Received new tree")
	o.RegisterTree(tree)
}

// handleConfigMessage stores the config message so it can be dispatched
// alongside with the protocol message later to the service.
func (o *Overlay) handleConfigMessage(env *network.Envelope) {
	config, ok := env.Msg.(*ConfigMsg)
	if !ok {
		// This should happen only if a bad packet gets through
		log.Error(o.server.Address(), "Wrong config type, most likely invalid packet got through.")
		return
	}

	o.pendingConfigsMut.Lock()
	defer o.pendingConfigsMut.Unlock()
	o.pendingConfigs[config.Dest] = &config.Config
}

// getConfig returns the generic config corresponding to this node if present,
// and removes it from the list of pending configs.
func (o *Overlay) getConfig(id TokenID) *GenericConfig {
	o.pendingConfigsMut.Lock()
	defer o.pendingConfigsMut.Unlock()
	c := o.pendingConfigs[id]
	delete(o.pendingConfigs, id)
	return c
}

// SendToTreeNode sends a message to a treeNode
// from is the sender token
// to is the treenode of the destination
// msg is the message to send
// io is the messageproxy used to correctly create the wire format
// c is the generic config that should be sent beforehand in order to get passed
// in the `NewProtocol` method if a Service has created the protocol and set the
// config with `SetConfig`. It can be nil.
func (o *Overlay) SendToTreeNode(from *Token, to *TreeNode, msg network.Message, io MessageProxy, c *GenericConfig) (uint64, error) {
	tokenTo := from.ChangeTreeNodeID(to.ID)
	var totSentLen uint64

	// first send the config if present
	if c != nil {
		sentLen, err := o.server.Send(to.ServerIdentity, &ConfigMsg{*c, tokenTo.ID()})
		totSentLen += sentLen
		if err != nil {
			log.Error("sending config failed:", err)
			return totSentLen, err
		}
	}
	// then send the message
	var final interface{}
	info := &OverlayMsg{
		TreeNodeInfo: &TreeNodeInfo{
			From: from,
			To:   tokenTo,
		},
	}
	final, err := io.Wrap(msg, info)
	if err != nil {
		return totSentLen, err
	}

	sentLen, err := o.server.Send(to.ServerIdentity, final)
	totSentLen += sentLen
	return totSentLen, err
}

// nodeDone is called by node to signify that its work is finished and its
// ressources can be released
func (o *Overlay) nodeDone(tok *Token) {
	o.instancesLock.Lock()
	o.nodeDelete(tok)
	o.instancesLock.Unlock()
}

// nodeDelete needs to be separated from nodeDone, as it is also called from
// Close, but due to locking-issues here we don't lock.
func (o *Overlay) nodeDelete(token *Token) {
	tok := token.ID()
	tni, ok := o.instances[tok]
	if !ok {
		log.Lvlf2("Node %s already gone", tok)
		return
	}
	log.Lvl4("Closing node", tok)
	err := tni.closeDispatch()
	if err != nil {
		log.Error("Error while closing node:", err)
	}
	delete(o.protocolInstances, tok)
	delete(o.instances, tok)

	o.cleanTreeStorage(token)

	// mark it done !
	o.instancesInfo[tok] = true
}

func (o *Overlay) cleanTreeStorage(token *Token) {
	notUsed := true
	for _, inst := range o.instances {
		if inst.token.TreeID.Equal(token.TreeID) {
			// another instance is using the same tree
			notUsed = false
		}
	}

	o.treeStorageLock.Lock()
	defer o.treeStorageLock.Unlock()

	tree := o.treeStorage[token.TreeID]

	if notUsed && tree != nil {
		// TODO: handle the case where children ask for the tree but the protocol is done on the root side
		//delete(o.treeStorage, token.TreeID)
	}
}

func (o *Overlay) suite() network.Suite {
	return o.server.Suite()
}

// Close calls all nodes, deletes them from the list and closes them
func (o *Overlay) Close() {
	o.instancesLock.Lock()
	defer o.instancesLock.Unlock()
	for _, tni := range o.instances {
		log.Lvl4(o.server.Address(), "Closing TNI", tni.TokenID())
		o.nodeDelete(tni.Token())
	}
}

// CreateProtocol creates a ProtocolInstance, registers it to the Overlay.
// Additionally, if sid is different than NilServiceID, sid is added to the token
// so the protocol will be picked up by the correct service and handled by its
// NewProtocol method. If the sid is NilServiceID, then the protocol is handled by onet alone.
func (o *Overlay) CreateProtocol(name string, t *Tree, sid ServiceID) (ProtocolInstance, error) {
	io := o.protoIO.getByName(name)
	tni := o.NewTreeNodeInstanceFromService(t, t.Root, ProtocolNameToID(name), sid, io)
	pi, err := o.server.protocolInstantiate(tni.token.ProtoID, tni)
	if err != nil {
		return nil, err
	}
	if err = o.RegisterProtocolInstance(pi); err != nil {
		return nil, err
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("Panic in %s.Dispatch(): %v", name, r)
			}
		}()

		err := pi.Dispatch()
		if err != nil {
			log.Errorf("%s.Dispatch() created in service %s returned error %s",
				name, ServiceFactory.Name(sid), err)
		}
	}()
	return pi, err
}

// StartProtocol will create and start a ProtocolInstance.
func (o *Overlay) StartProtocol(name string, t *Tree, sid ServiceID) (ProtocolInstance, error) {
	pi, err := o.CreateProtocol(name, t, sid)
	if err != nil {
		return nil, err
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("Panic in %s.Start(): %v", name, r)
			}
		}()

		err := pi.Start()
		if err != nil {
			log.Error("Error while starting:", err)
		}
	}()
	return pi, err
}

// NewTreeNodeInstanceFromProtoName takes a protocol name and a tree and
// instantiate a TreeNodeInstance for this protocol.
func (o *Overlay) NewTreeNodeInstanceFromProtoName(t *Tree, name string) *TreeNodeInstance {
	io := o.protoIO.getByName(name)
	return o.NewTreeNodeInstanceFromProtocol(t, t.Root, ProtocolNameToID(name), io)
}

// NewTreeNodeInstanceFromProtocol takes a tree and a treenode (normally the
// root) and and protocolID and returns a fresh TreeNodeInstance.
func (o *Overlay) NewTreeNodeInstanceFromProtocol(t *Tree, tn *TreeNode, protoID ProtocolID, io MessageProxy) *TreeNodeInstance {
	tok := &Token{
		TreeNodeID: tn.ID,
		TreeID:     t.ID,
		RosterID:   t.Roster.ID,
		ProtoID:    protoID,
		RoundID:    RoundID(uuid.NewV4()),
	}
	tni := o.newTreeNodeInstanceFromToken(tn, tok, io)
	o.RegisterTree(t)
	return tni
}

// NewTreeNodeInstanceFromService takes a tree, a TreeNode and a service ID and
// returns a TNI.
func (o *Overlay) NewTreeNodeInstanceFromService(t *Tree, tn *TreeNode, protoID ProtocolID, servID ServiceID, io MessageProxy) *TreeNodeInstance {
	tok := &Token{
		TreeNodeID: tn.ID,
		TreeID:     t.ID,
		RosterID:   t.Roster.ID,
		ProtoID:    protoID,
		ServiceID:  servID,
		RoundID:    RoundID(uuid.NewV4()),
	}
	tni := o.newTreeNodeInstanceFromToken(tn, tok, io)
	o.RegisterTree(t)
	return tni
}

// ServerIdentity Returns the entity of the Host
func (o *Overlay) ServerIdentity() *network.ServerIdentity {
	return o.server.ServerIdentity
}

// newTreeNodeInstanceFromToken is to be called by the Overlay when it receives
// a message it does not have a treenodeinstance registered yet. The protocol is
// already running so we should *not* generate a new RoundID.
func (o *Overlay) newTreeNodeInstanceFromToken(tn *TreeNode, tok *Token, io MessageProxy) *TreeNodeInstance {
	tni := newTreeNodeInstance(o, tok, tn, io)
	o.instancesLock.Lock()
	defer o.instancesLock.Unlock()
	o.instances[tok.ID()] = tni
	return tni
}

// ErrWrongTreeNodeInstance is returned when you already binded a TNI with a PI.
var ErrWrongTreeNodeInstance = errors.New("This TreeNodeInstance doesn't exist")

// ErrProtocolRegistered is when the protocolinstance is already registered to
// the overlay
var ErrProtocolRegistered = errors.New("a ProtocolInstance already has been registered using this TreeNodeInstance")

// RegisterProtocolInstance takes a PI and stores it for dispatching the message
// to it.
func (o *Overlay) RegisterProtocolInstance(pi ProtocolInstance) error {
	o.instancesLock.Lock()
	defer o.instancesLock.Unlock()
	var tni *TreeNodeInstance
	var tok = pi.Token()
	var ok bool
	// if the TreeNodeInstance doesn't exist
	if tni, ok = o.instances[tok.ID()]; !ok {
		return ErrWrongTreeNodeInstance
	}

	if tni.isBound() {
		return ErrProtocolRegistered
	}

	tni.bind(pi)
	o.protocolInstances[tok.ID()] = pi
	log.Lvlf4("%s registered ProtocolInstance %x", o.server.Address(), tok.ID())
	return nil
}

// RegisterMessageProxy registers a message proxy only for this overlay
func (o *Overlay) RegisterMessageProxy(m MessageProxy) {
	o.protoIO.RegisterMessageProxy(m)
}

// pendingMsg is used to store messages destined for ProtocolInstances but when
// the tree designated is not known to the Overlay. When the tree is sent to the
// overlay, then the pendingMsg that are relying on this tree will get
// processed.
type pendingMsg struct {
	*ProtocolMsg
	MessageProxy
}

// defaultProtoIO implements the ProtocoIO interface but using the "regular/old"
// wire format protocol,i.e. it wraps a message into a ProtocolMessage
type defaultProtoIO struct {
	suite network.Suite
}

// Wrap implements the MessageProxy interface for the Overlay.
func (d *defaultProtoIO) Wrap(msg interface{}, info *OverlayMsg) (interface{}, error) {
	if msg != nil {
		buff, err := network.Marshal(msg)
		if err != nil {
			return nil, err
		}
		typ := network.MessageType(msg)
		protoMsg := &ProtocolMsg{
			From:     info.TreeNodeInfo.From,
			To:       info.TreeNodeInfo.To,
			MsgSlice: buff,
			MsgType:  typ,
		}
		return protoMsg, nil
	}
	var returnMsg interface{}
	switch true {
	case info.RequestTree != nil:
		returnMsg = info.RequestTree
	case info.ResponseTree != nil:
		returnMsg = info.ResponseTree
	default:
		panic("overlay: default wrapper has nothing to wrap")
	}
	return returnMsg, nil
}

// Unwrap implements the MessageProxy interface for the Overlay.
func (d *defaultProtoIO) Unwrap(msg interface{}) (interface{}, *OverlayMsg, error) {
	var returnMsg interface{}
	var returnOverlay = new(OverlayMsg)
	var err error

	switch inner := msg.(type) {
	case *ProtocolMsg:
		onetMsg := inner
		var err error
		_, protoMsg, err := network.Unmarshal(onetMsg.MsgSlice, d.suite)
		if err != nil {
			return nil, nil, err
		}
		// Put the msg into ProtocolMsg
		returnOverlay.TreeNodeInfo = &TreeNodeInfo{
			To:   onetMsg.To,
			From: onetMsg.From,
		}
		returnMsg = protoMsg
	case *RequestTree:
		returnOverlay.RequestTree = inner
	case *ResponseTree:
		returnOverlay.ResponseTree = inner
	default:
		err = errors.New("default protoIO: unwraping an unknown message type")
	}
	return returnMsg, returnOverlay, err
}

// Unwrap implements the MessageProxy interface for the Overlay.
func (d *defaultProtoIO) PacketType() network.MessageTypeID {
	return network.MessageTypeID([16]byte{})
}

// Name implements the MessageProxy interface. It returns the value "default".
func (d *defaultProtoIO) Name() string {
	return "default"
}
