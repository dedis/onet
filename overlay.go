package onet

import (
	"fmt"
	"sync"
	"time"

	"go.dedis.ch/onet/v4/ciphersuite"
	"go.dedis.ch/onet/v4/log"
	"go.dedis.ch/onet/v4/network"
	"golang.org/x/xerrors"
	uuid "gopkg.in/satori/go.uuid.v1"
)

// timeout used to clean up protocol state so that children can keep
// asking for resources for this range of time (i.e. tree)
const globalProtocolTimeout = 10 * time.Minute

// Overlay keeps all trees and entity-lists for a given Server. It creates
// Nodes and ProtocolInstances upon request and dispatches the messages.
type Overlay struct {
	server *Server
	cr     *ciphersuite.Registry

	treeStorage *treeStorage

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
func NewOverlay(c *Server, cr *ciphersuite.Registry) *Overlay {
	if c == nil {
		panic("expecting a server to be provided")
	}

	if cr == nil {
		panic("expecting a cipher suite registry to be provided")
	}

	o := &Overlay{
		server:             c,
		cr:                 cr,
		treeStorage:        newTreeStorage(globalProtocolTimeout),
		instances:          make(map[TokenID]*TreeNodeInstance),
		instancesInfo:      make(map[TokenID]bool),
		protocolInstances:  make(map[TokenID]ProtocolInstance),
		pendingTreeMarshal: make(map[RosterID][]*TreeMarshal),
		pendingConfigs:     make(map[TokenID]*GenericConfig),
	}
	o.protoIO = newMessageProxyStore(c, o)
	// messages going to protocol instances
	c.RegisterProcessor(o,
		ProtocolMsgID,     // protocol instance's messages
		RequestTreeMsgID,  // request a tree
		ResponseTreeMsgID, // send a tree back to a request
		ConfigMsgID)       // fetch config information
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
	// Get the tree if it exists and prevent any pending deletion
	// if required. The tree will be clean when this instance is
	// over (or the last instance using the tree).
	tree := o.treeStorage.getAndRefresh(onetMsg.To.TreeID)
	if tree == nil {
		// request anyway because we need to store the pending message
		// the following routine will take care of requesting once
		err := o.requestTree(onetMsg.ServerIdentity, onetMsg, io)
		if err != nil {
			return xerrors.Errorf("requesting tree: %v", err)
		}
		return nil
	}

	o.transmitMux.Lock()
	defer o.transmitMux.Unlock()
	// TreeNodeInstance
	var pi ProtocolInstance
	o.instancesLock.Lock()
	pi, ok := o.protocolInstances[onetMsg.To.ID()]
	done := o.instancesInfo[onetMsg.To.ID()]
	o.instancesLock.Unlock()
	if done {
		log.Lvl5("Message for TreeNodeInstance that is already finished")
		return nil
	}
	// if the TreeNodeInstance is not there, creates it
	if !ok {
		log.Lvlf4("Creating TreeNodeInstance at %s %x", o.server.ServerIdentity, onetMsg.To.ID())
		tn, err := o.TreeNodeFromTree(tree, onetMsg.To.TreeNodeID)
		if err != nil {
			return xerrors.New("No TreeNode defined in this tree here")
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
			return xerrors.Errorf("creating protocol: %v", err)
		}
		if pi == nil {
			return nil
		}
		go func() {
			defer func() {
				if r := recover(); r != nil {
					svc := ServiceFactory.Name(tni.Token().ServiceID)
					log.Errorf("Panic in call to protocol <%s>.Dispatch() from service <%s> at address %s: %v",
						tni.ProtocolName(), svc, o.server.ServerIdentity, r)
					log.Error(log.Stack())
				}
			}()

			err := pi.Dispatch()
			if err != nil {
				svc := ServiceFactory.Name(tni.Token().ServiceID)
				log.Errorf("%v %s.Dispatch() returned error %s", o.server.ServerIdentity, svc, err)
			}
		}()
		if err := o.RegisterProtocolInstance(pi); err != nil {
			return xerrors.New("Error Binding TreeNodeInstance and ProtocolInstance:" +
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

// checkPendingTreeMarshal is called each time we add a new Roster to the
// system. It checks if some treeMarshal use this entityList so they can be
// converted to Tree.
func (o *Overlay) checkPendingTreeMarshal(ro *Roster) {
	o.pendingTreeLock.Lock()
	sl, ok := o.pendingTreeMarshal[ro.GetID()]
	if !ok {
		// no tree for this roster
		return
	}
	for _, tm := range sl {
		tree, err := tm.MakeTree(ro)
		if err != nil {
			log.Error("Tree from Roster failed")
			continue
		}
		// add the tree into our "database"
		o.RegisterTree(tree)
	}
	o.pendingTreeLock.Unlock()
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

	// try to prepare the message before locking the storage
	msg, err := io.Wrap(nil, &OverlayMsg{
		RequestTree: &RequestTree{TreeID: onetMsg.To.TreeID, Version: 1},
	})
	if err != nil {
		return xerrors.Errorf("wrapping message: %v", err)
	}

	if o.treeStorage.IsRegistered(onetMsg.To.TreeID) {
		// request already sent
		return nil
	}

	// register the tree as known (can be stored)
	o.treeStorage.Register(onetMsg.To.TreeID)

	// no need to record sentLen because Overlay uses Server's CounterIO
	_, err = o.server.Send(si, msg)
	if err != nil {
		o.treeStorage.Unregister(onetMsg.To.TreeID)
		return xerrors.Errorf("sending tree request: %v", err)
	}

	return nil
}

// RegisterTree takes a tree and puts it in the map
func (o *Overlay) RegisterTree(t *Tree) {
	o.treeStorage.Set(t)

	o.checkPendingMessages(t)
}

// TreeNodeFromTree returns the treeNode corresponding to the id
func (o *Overlay) TreeNodeFromTree(tree *Tree, id TreeNodeID) (*TreeNode, error) {
	tn := tree.Search(id)
	if tn == nil {
		return nil, xerrors.New("didn't find treenode")
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

// Send the tree or do nothing when it is not known
func (o *Overlay) handleRequestTree(si *network.ServerIdentity, req *RequestTree, io MessageProxy) {
	tree := o.treeStorage.Get(req.TreeID)
	if tree == nil {
		// XXX Should we think of a way of sending back an "error" ?
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
		log.Error("couldn't wrap ResponseTree:", err)
		return
	}

	_, err = o.server.Send(si, msg)
	if err != nil {
		log.Error("Couldn't send tree:", err)
	}
}

// Receive a tree from a peer
func (o *Overlay) handleSendTree(si *network.ServerIdentity, rt *ResponseTree, io MessageProxy) {
	if rt.TreeMarshal == nil || rt.TreeMarshal.TreeID.IsNil() {
		log.Error("received an empty tree")
		return
	}

	if rt.Roster == nil {
		log.Error("received an empty roster")
		return
	}

	if !o.treeStorage.IsRegistered(rt.TreeMarshal.TreeID) {
		// we only accept known trees to prevent a denial of service
		// by filling up the storage
		log.Error("ignoring unknown tree")
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
			return totSentLen, xerrors.Errorf("sending: %v", err)
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
		return totSentLen, xerrors.Errorf("wrapping message: %v", err)
	}

	sentLen, err := o.server.Send(to.ServerIdentity, final)
	totSentLen += sentLen
	if err != nil {
		return totSentLen, xerrors.Errorf("sending: %v", err)
	}
	return totSentLen, nil
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

// checks if another instance is using the same tree and clean it
// only if not. Note that this function assumes that o.instances
// is locked (e.g. Overlay.nodeDone)
func (o *Overlay) cleanTreeStorage(token *Token) {
	notUsed := true
	for _, inst := range o.instances {
		if inst.token.TreeID.Equal(token.TreeID) {
			// another instance is using the same tree
			notUsed = false
		}
	}

	if notUsed {
		o.treeStorage.Remove(token.TreeID)
	}
}

// Close calls all nodes, deletes them from the list and closes them
func (o *Overlay) Close() {
	o.instancesLock.Lock()
	defer o.instancesLock.Unlock()
	for _, tni := range o.instances {
		log.Lvl4(o.server.Address(), "Closing TNI", tni.TokenID())
		o.nodeDelete(tni.Token())
	}

	// force cleaning routines to shutdown
	o.treeStorage.Close()
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
		return nil, xerrors.Errorf("instantiating protocol: %v", err)
	}
	if err = o.RegisterProtocolInstance(pi); err != nil {
		return nil, xerrors.Errorf("registering protocol instance: %v", err)
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
		return nil, xerrors.Errorf("creating protocol: %v", err)
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
	return pi, nil
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
		RosterID:   t.Roster.GetID(),
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
		RosterID:   t.Roster.GetID(),
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
var ErrWrongTreeNodeInstance = xerrors.New("This TreeNodeInstance doesn't exist")

// ErrProtocolRegistered is when the protocolinstance is already registered to
// the overlay
var ErrProtocolRegistered = xerrors.New("a ProtocolInstance already has been registered using this TreeNodeInstance")

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
type defaultProtoIO struct{}

// Wrap implements the MessageProxy interface for the Overlay.
func (d *defaultProtoIO) Wrap(msg interface{}, info *OverlayMsg) (interface{}, error) {
	if msg != nil {
		buff, err := network.Marshal(msg)
		if err != nil {
			return nil, xerrors.Errorf("marshaling: %v", err)
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
	switch {
	case info.RequestTree != nil:
		returnMsg = info.RequestTree
	case info.ResponseTree != nil:
		returnMsg = info.ResponseTree
	case info.TreeMarshal != nil:
		returnMsg = info.TreeMarshal
	case info.RequestRoster != nil:
		returnMsg = info.RequestRoster
	case info.Roster != nil:
		returnMsg = info.Roster
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
		_, protoMsg, err := network.Unmarshal(onetMsg.MsgSlice)
		if err != nil {
			return nil, nil, xerrors.Errorf("unmarshaling: %v", err)
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
	case *TreeMarshal:
		returnOverlay.TreeMarshal = inner
	case *RequestRoster:
		returnOverlay.RequestRoster = inner
	case *Roster:
		returnOverlay.Roster = inner
	default:
		err = xerrors.New("default protoIO: unwraping an unknown message type")
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
