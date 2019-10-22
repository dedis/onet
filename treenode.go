package onet

import (
	"fmt"
	"reflect"
	"sync"

	"go.dedis.ch/onet/v4/ciphersuite"
	"go.dedis.ch/onet/v4/log"
	"go.dedis.ch/onet/v4/network"
	"golang.org/x/xerrors"
)

// TreeNodeInstance represents a protocol-instance in a given TreeNode. It embeds an
// Overlay where all the tree-structures are stored.
type TreeNodeInstance struct {
	overlay *Overlay
	token   *Token
	// cache for the TreeNode this Node is representing
	treeNode *TreeNode
	// cached list of all TreeNodes
	treeNodeList []*TreeNode
	// mutex to synchronise creation of treeNodeList
	mtx sync.Mutex

	// channels holds all channels available for the different message-types
	channels map[network.MessageTypeID]interface{}
	// registered handler-functions for that protocol
	handlers map[network.MessageTypeID]interface{}
	// flags for messages - only one channel/handler possible
	messageTypeFlags map[network.MessageTypeID]uint32
	// The protocolInstance belonging to that node
	instance ProtocolInstance
	// aggregate messages in order to dispatch them at once in the protocol
	// instance
	msgQueue map[network.MessageTypeID][]*ProtocolMsg
	// done callback
	onDoneCallback func() bool
	// queue holding msgs
	msgDispatchQueue []*ProtocolMsg
	// locking for msgqueue
	msgDispatchQueueMutex sync.Mutex
	// kicking off new message
	msgDispatchQueueWait chan bool
	// whether this node is closing
	closing bool

	protoIO MessageProxy

	// config is to be passed down in the first message of what the protocol is
	// sending if it is non nil. Set with `tni.SetConfig()`.
	config    *GenericConfig
	sentTo    map[TreeNodeID]bool
	configMut sync.Mutex

	// used for the CounterIO interface
	tx safeAdder
	rx safeAdder
}

type safeAdder struct {
	sync.RWMutex
	x uint64
}

func (a *safeAdder) add(x uint64) {
	a.Lock()
	a.x += x
	a.Unlock()
}

func (a *safeAdder) get() (x uint64) {
	a.RLock()
	x = a.x
	a.RUnlock()
	return
}

const (
	// AggregateMessages (if set) tells to aggregate messages from all children
	// before sending to the (parent) Node
	AggregateMessages = 1

	// DefaultChannelLength is the default number of messages that can wait
	// in a channel.
	DefaultChannelLength = 100
)

// MsgHandler is called upon reception of a certain message-type
type MsgHandler func([]*interface{})

// NewNode creates a new node
func newTreeNodeInstance(o *Overlay, tok *Token, tn *TreeNode, io MessageProxy) *TreeNodeInstance {
	n := &TreeNodeInstance{overlay: o,
		token:                tok,
		channels:             make(map[network.MessageTypeID]interface{}),
		handlers:             make(map[network.MessageTypeID]interface{}),
		messageTypeFlags:     make(map[network.MessageTypeID]uint32),
		msgQueue:             make(map[network.MessageTypeID][]*ProtocolMsg),
		treeNode:             tn,
		msgDispatchQueue:     make([]*ProtocolMsg, 0, 1),
		msgDispatchQueueWait: make(chan bool, 1),
		protoIO:              io,
		sentTo:               make(map[TreeNodeID]bool),
	}
	go n.dispatchMsgReader()
	return n
}

// Service returns the service and its name of the tree node instance.
func (n *TreeNodeInstance) Service() (Service, string) {
	return n.overlay.server.serviceManager.serviceByID(n.token.ServiceID)
}

// TreeNode gets the treeNode of this node. If there is no TreeNode for the
// Token of this node, the function will return nil
func (n *TreeNodeInstance) TreeNode() *TreeNode {
	return n.treeNode
}

// ServerIdentity returns our entity
func (n *TreeNodeInstance) ServerIdentity() *network.ServerIdentity {
	return n.treeNode.ServerIdentity
}

// Parent returns the parent-TreeNode of ourselves
func (n *TreeNodeInstance) Parent() *TreeNode {
	return n.treeNode.Parent
}

// Children returns the children of ourselves
func (n *TreeNodeInstance) Children() []*TreeNode {
	return n.treeNode.Children
}

// Root returns the root-node of that tree
func (n *TreeNodeInstance) Root() *TreeNode {
	t := n.Tree()
	if t != nil {
		return t.Root
	}
	return nil
}

// IsRoot returns whether whether we are at the top of the tree
func (n *TreeNodeInstance) IsRoot() bool {
	return n.treeNode.Parent == nil
}

// IsLeaf returns whether whether we are at the bottom of the tree
func (n *TreeNodeInstance) IsLeaf() bool {
	return len(n.treeNode.Children) == 0
}

// SendTo sends to a given node
func (n *TreeNodeInstance) SendTo(to *TreeNode, msg interface{}) error {
	if to == nil {
		return xerrors.New("Sent to a nil TreeNode")
	}
	n.msgDispatchQueueMutex.Lock()
	if n.closing {
		n.msgDispatchQueueMutex.Unlock()
		return xerrors.New("is closing")
	}
	n.msgDispatchQueueMutex.Unlock()
	var c *GenericConfig
	// only sends the config once
	n.configMut.Lock()
	if !n.sentTo[to.ID] {
		c = n.config
		n.sentTo[to.ID] = true
	}
	n.configMut.Unlock()

	sentLen, err := n.overlay.SendToTreeNode(n.token, to, msg, n.protoIO, c)
	n.tx.add(sentLen)
	if err != nil {
		return xerrors.Errorf("sending: %v", err)
	}
	return nil
}

// Tree returns the tree of that node. Because the storage keeps the tree around
// until the protocol is done, this will never return a nil value. It will panic
// if the tree is nil.
func (n *TreeNodeInstance) Tree() *Tree {
	tree := n.overlay.treeStorage.Get(n.token.TreeID)
	if tree == nil {
		panic("tree should never be nil when called during a protocol; " +
			"it might be that Tree() has been called after Done() which " +
			"is wrong or the tree has not correctly been passed.")
	}

	return tree
}

// Roster returns the entity-list
func (n *TreeNodeInstance) Roster() *Roster {
	return n.Tree().Roster
}

// RegisterChannel is a compatibility-method for RegisterChannelLength
// and setting up a channel with length 100.
func (n *TreeNodeInstance) RegisterChannel(c interface{}) error {
	err := n.RegisterChannelLength(c, DefaultChannelLength)
	if err != nil {
		return xerrors.Errorf("registering channel length: %v", err)
	}
	return nil
}

// RegisterChannelLength takes a channel with a struct that contains two
// elements: a TreeNode and a message. The second argument is the length of
// the channel. It will send every message that are the
// same type to this channel.
// This function handles also
// - registration of the message-type
// - aggregation or not of messages: if you give a channel of slices, the
//   messages will be aggregated, else they will come one-by-one
func (n *TreeNodeInstance) RegisterChannelLength(c interface{}, length int) error {
	flags := uint32(0)
	cr := reflect.TypeOf(c)
	if cr.Kind() == reflect.Ptr {
		val := reflect.ValueOf(c).Elem()
		val.Set(reflect.MakeChan(val.Type(), length))
		return n.RegisterChannel(reflect.Indirect(val).Interface())
	} else if reflect.ValueOf(c).IsNil() {
		return xerrors.New("Can not Register a (value) channel not initialized")
	}
	// Check we have the correct channel-type
	if cr.Kind() != reflect.Chan {
		return xerrors.New("Input is not channel")
	}
	if cr.Elem().Kind() == reflect.Slice {
		flags += AggregateMessages
		cr = cr.Elem()
	}
	if cr.Elem().Kind() != reflect.Struct {
		return xerrors.New("Input is not channel of structure")
	}
	if cr.Elem().NumField() != 2 {
		return xerrors.New("Input is not channel of structure with 2 elements")
	}
	if cr.Elem().Field(0).Type != reflect.TypeOf(&TreeNode{}) {
		return xerrors.New("Input-channel doesn't have TreeNode as element")
	}
	// Automatic registration of the message to the network library.
	m := reflect.New(cr.Elem().Field(1).Type)
	typ := network.RegisterMessage(m.Interface())
	n.channels[typ] = c
	//typ := network.RTypeToUUID(cr.Elem().Field(1).Type) n.channels[typ] = c
	n.messageTypeFlags[typ] = flags
	log.Lvl4("Registered channel", typ, "with flags", flags)
	return nil
}

// RegisterChannels registers a list of given channels by calling RegisterChannel above
func (n *TreeNodeInstance) RegisterChannels(channels ...interface{}) error {
	for _, ch := range channels {
		if err := n.RegisterChannel(ch); err != nil {
			return xerrors.Errorf("Error, could not register channel %T: %s",
				ch, err.Error())
		}
	}
	return nil
}

// RegisterChannelsLength is a convenience function to register a vararg of
// channels with a given length.
func (n *TreeNodeInstance) RegisterChannelsLength(length int, channels ...interface{}) error {
	for _, ch := range channels {
		if err := n.RegisterChannelLength(ch, length); err != nil {
			return xerrors.Errorf("Error, could not register channel %T: %s",
				ch, err.Error())
		}
	}
	return nil
}

// RegisterHandler takes a function which takes a struct as argument that contains two
// elements: a TreeNode and a message. It will send every message that are the
// same type to this channel.
//
// This function also handles:
//     - registration of the message-type
//     - aggregation or not of messages: if you give a channel of slices, the
//       messages will be aggregated, otherwise they will come one by one
func (n *TreeNodeInstance) RegisterHandler(c interface{}) error {
	flags := uint32(0)
	cr := reflect.TypeOf(c)
	// Check we have the correct channel-type
	if cr.Kind() != reflect.Func {
		return xerrors.New("Input is not function")
	}
	if cr.NumOut() != 1 {
		return xerrors.New("Need exactly one return argument of type error")
	}
	if cr.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
		return xerrors.New("return-type of message-handler needs to be error")
	}
	ci := cr.In(0)
	if ci.Kind() == reflect.Slice {
		flags += AggregateMessages
		ci = ci.Elem()
	}
	if ci.Kind() != reflect.Struct {
		return xerrors.New("Input is not a structure")
	}
	if ci.NumField() != 2 {
		return xerrors.New("Input is not a structure with 2 elements")
	}
	if ci.Field(0).Type != reflect.TypeOf(&TreeNode{}) {
		return xerrors.New("Input-handler doesn't have TreeNode as element")
	}
	// Automatic registration of the message to the network library.
	ptr := reflect.New(ci.Field(1).Type)
	typ := network.RegisterMessage(ptr.Interface())
	n.handlers[typ] = c
	n.messageTypeFlags[typ] = flags
	log.Lvl3("Registered handler", typ, "with flags", flags)
	return nil
}

// RegisterHandlers registers a list of given handlers by calling RegisterHandler above
func (n *TreeNodeInstance) RegisterHandlers(handlers ...interface{}) error {
	for _, h := range handlers {
		if err := n.RegisterHandler(h); err != nil {
			return xerrors.Errorf("Error, could not register handler %T: %s",
				h, err.Error())
		}
	}
	return nil
}

// ProtocolInstance returns the instance of the running protocol
func (n *TreeNodeInstance) ProtocolInstance() ProtocolInstance {
	return n.instance
}

// Dispatch - the standard dispatching function is empty
func (n *TreeNodeInstance) Dispatch() error {
	return nil
}

// Shutdown - standard Shutdown implementation. Define your own
// in your protocol (if necessary)
func (n *TreeNodeInstance) Shutdown() error {
	return nil
}

// closeDispatch shuts down the go-routine and calls the protocolInstance-shutdown
func (n *TreeNodeInstance) closeDispatch() error {
	defer func() {
		if r := recover(); r != nil {
			log.Error("Recovered panic:", r)
		}
	}()
	log.Lvl3("Closing node", n.Info())
	n.msgDispatchQueueMutex.Lock()
	n.closing = true
	close(n.msgDispatchQueueWait)
	n.msgDispatchQueueMutex.Unlock()
	log.Lvl3("Closed node", n.Info())
	pni := n.ProtocolInstance()
	if pni == nil {
		return xerrors.New("Can't shutdown empty ProtocolInstance")
	}
	err := pni.Shutdown()
	if err != nil {
		return xerrors.Errorf("shutdown: %v", err)
	}
	return nil
}

// ProtocolName will return the string representing that protocol
func (n *TreeNodeInstance) ProtocolName() string {
	return n.overlay.server.protocols.ProtocolIDToName(n.token.ProtoID)
}

func (n *TreeNodeInstance) dispatchHandler(msgSlice []*ProtocolMsg) error {
	mt := msgSlice[0].MsgType
	to := reflect.TypeOf(n.handlers[mt]).In(0)
	f := reflect.ValueOf(n.handlers[mt])
	var errV reflect.Value
	if n.hasFlag(mt, AggregateMessages) {
		msgs := reflect.MakeSlice(to, len(msgSlice), len(msgSlice))
		for i, msg := range msgSlice {
			m, err := n.createValueAndVerify(to.Elem(), msg)
			if err != nil {
				return xerrors.Errorf("processing message: %v", err)
			}
			msgs.Index(i).Set(m)
		}
		log.Lvl4("Dispatching aggregation to", n.ServerIdentity().Address)
		errV = f.Call([]reflect.Value{msgs})[0]
	} else {
		for _, msg := range msgSlice {
			if errV.IsValid() && !errV.IsNil() {
				// Before overwriting an error, print it out
				log.Errorf("%s: error while dispatching message %s: %s",
					n.Name(), reflect.TypeOf(msg.Msg),
					errV.Interface().(error))
			}
			log.Lvl4("Dispatching", msg, "to", n.ServerIdentity().Address)
			m, err := n.createValueAndVerify(to, msg)
			if err != nil {
				return xerrors.Errorf("processing message: %v", err)
			}
			errV = f.Call([]reflect.Value{m})[0]
		}
	}
	log.Lvlf4("%s Done with handler for %s", n.Name(), f.Type())
	if !errV.IsNil() {
		return xerrors.Errorf("handler: %v", errV.Interface())
	}
	return nil
}

func (n *TreeNodeInstance) createValueAndVerify(t reflect.Type, msg *ProtocolMsg) (reflect.Value, error) {
	m := reflect.Indirect(reflect.New(t))
	tr := n.Tree()
	if t != nil {
		tn := tr.Search(msg.From.TreeNodeID)
		if tn != nil {
			m.Field(0).Set(reflect.ValueOf(tn))
			m.Field(1).Set(reflect.Indirect(reflect.ValueOf(msg.Msg)))
		}
		// Check whether the sender treenode actually is the same as the node who sent it.
		// We can trust msg.ServerIdentity, because it is written in Router.handleConn and
		// is not writable by the sending node.
		if msg.ServerIdentity != nil && tn != nil && !tn.ServerIdentity.Equal(msg.ServerIdentity) {
			return m, xerrors.Errorf("ServerIdentity in the tree node referenced by the message (%v) does not match the ServerIdentity of the message originator (%v)",
				tn.ServerIdentity, msg.ServerIdentity)
		}
	}
	return m, nil
}

// dispatchChannel takes a message and sends it to a channel
func (n *TreeNodeInstance) dispatchChannel(msgSlice []*ProtocolMsg) error {
	mt := msgSlice[0].MsgType
	defer func() {
		// In rare occasions we write to a closed channel which throws a panic.
		// Catch it here so we can find out better why this happens.
		if r := recover(); r != nil {
			log.Error("Shouldn't happen, please report an issue:", n.Info(), mt, r)
		}
	}()
	to := reflect.TypeOf(n.channels[mt])
	if n.hasFlag(mt, AggregateMessages) {
		log.Lvl4("Received aggregated message of type:", mt)
		to = to.Elem()
		out := reflect.MakeSlice(to, len(msgSlice), len(msgSlice))
		for i, msg := range msgSlice {
			log.Lvl4("Dispatching aggregated to", to)
			m, err := n.createValueAndVerify(to.Elem(), msg)
			if err != nil {
				return xerrors.Errorf("processing message: %v", err)
			}
			log.Lvl4("Adding msg", m, "to", n.ServerIdentity().Address)
			out.Index(i).Set(m)
		}
		reflect.ValueOf(n.channels[mt]).Send(out)
	} else {
		for _, msg := range msgSlice {
			out := reflect.ValueOf(n.channels[mt])
			m, err := n.createValueAndVerify(to.Elem(), msg)
			if err != nil {
				return xerrors.Errorf("processing message: %v", err)
			}
			log.Lvl4(n.Name(), "Dispatching msg type", mt, " to", to, " :", m.Field(1).Interface())
			if out.Len() < out.Cap() {
				n.msgDispatchQueueMutex.Lock()
				closing := n.closing
				n.msgDispatchQueueMutex.Unlock()
				if !closing {
					out.Send(m)
				}
			} else {
				return xerrors.Errorf("channel too small for msg %s in %s: "+
					"please use RegisterChannelLength()",
					mt, n.ProtocolName())
			}
		}
	}
	return nil
}

// ProcessProtocolMsg takes a message and puts it into a queue for later processing.
// This allows a protocol to have a backlog of messages.
func (n *TreeNodeInstance) ProcessProtocolMsg(msg *ProtocolMsg) {
	log.Lvl4(n.Info(), "Received message")
	n.msgDispatchQueueMutex.Lock()
	defer n.msgDispatchQueueMutex.Unlock()
	if n.closing {
		log.Lvl3("Received message for closed protocol")
		return
	}
	n.msgDispatchQueue = append(n.msgDispatchQueue, msg)
	n.notifyDispatch()
}

func (n *TreeNodeInstance) notifyDispatch() {
	select {
	case n.msgDispatchQueueWait <- true:
		return
	default:
		// Channel write would block: already been notified.
		// So, nothing to do here.
	}
}

func (n *TreeNodeInstance) dispatchMsgReader() {
	log.Lvl3("Starting node", n.Info())
	for {
		n.msgDispatchQueueMutex.Lock()
		if n.closing {
			log.Lvl3("Closing reader")
			n.msgDispatchQueueMutex.Unlock()
			return
		}
		if len(n.msgDispatchQueue) > 0 {
			log.Lvl4(n.Info(), "Read message and dispatching it",
				len(n.msgDispatchQueue))
			msg := n.msgDispatchQueue[0]
			n.msgDispatchQueue = n.msgDispatchQueue[1:]
			n.msgDispatchQueueMutex.Unlock()
			err := n.dispatchMsgToProtocol(msg)
			if err != nil {
				log.Errorf("%s: error while dispatching message %s: %s",
					n.Name(), reflect.TypeOf(msg.Msg), err)
			}
		} else {
			n.msgDispatchQueueMutex.Unlock()
			log.Lvl4(n.Info(), "Waiting for message")
			// Allow for closing of the channel
			select {
			case <-n.msgDispatchQueueWait:
			}
		}
	}
}

// dispatchMsgToProtocol will dispatch this onet.Data to the right instance
func (n *TreeNodeInstance) dispatchMsgToProtocol(onetMsg *ProtocolMsg) error {

	n.rx.add(uint64(onetMsg.Size))

	// if message comes from parent, dispatch directly
	// if messages come from children we must aggregate them
	// if we still need to wait for additional messages, we return
	msgType, msgs, done := n.aggregate(onetMsg)
	if !done {
		log.Lvl3(n.Name(), "Not done aggregating children msgs")
		return nil
	}
	log.Lvlf5("%s->%s: Message is: %+v", onetMsg.From, n.Name(), onetMsg.Msg)

	var err error
	switch {
	case n.channels[msgType] != nil:
		log.Lvl4(n.Name(), "Dispatching to channel")
		err = n.dispatchChannel(msgs)
	case n.handlers[msgType] != nil:
		log.Lvl4("Dispatching to handler", n.ServerIdentity().Address)
		err = n.dispatchHandler(msgs)
	default:
		return xerrors.Errorf("message-type not handled by the protocol: %s", reflect.TypeOf(onetMsg.Msg))
	}
	if err != nil {
		return xerrors.Errorf("dispatch: %v", err)
	}
	return nil
}

// setFlag makes sure a given flag is set
func (n *TreeNodeInstance) setFlag(mt network.MessageTypeID, f uint32) {
	n.messageTypeFlags[mt] |= f
}

// clearFlag makes sure a given flag is removed
func (n *TreeNodeInstance) clearFlag(mt network.MessageTypeID, f uint32) {
	n.messageTypeFlags[mt] &^= f
}

// hasFlag returns true if the given flag is set
func (n *TreeNodeInstance) hasFlag(mt network.MessageTypeID, f uint32) bool {
	return n.messageTypeFlags[mt]&f != 0
}

// aggregate store the message for a protocol instance such that a protocol
// instances will get all its children messages at once.
// node is the node the host is representing in this Tree, and onetMsg is the
// message being analyzed.
func (n *TreeNodeInstance) aggregate(onetMsg *ProtocolMsg) (network.MessageTypeID, []*ProtocolMsg, bool) {
	mt := onetMsg.MsgType
	fromParent := !n.IsRoot() && onetMsg.From.TreeNodeID.Equal(n.Parent().ID)
	if fromParent || !n.hasFlag(mt, AggregateMessages) {
		return mt, []*ProtocolMsg{onetMsg}, true
	}
	// store the msg according to its type
	if _, ok := n.msgQueue[mt]; !ok {
		n.msgQueue[mt] = make([]*ProtocolMsg, 0)
	}
	msgs := append(n.msgQueue[mt], onetMsg)
	n.msgQueue[mt] = msgs
	log.Lvl4(n.ServerIdentity().Address, "received", len(msgs), "of", len(n.Children()), "messages")

	// do we have everything yet or no
	// get the node this host is in this tree
	// OK we have all the children messages
	if len(msgs) == len(n.Children()) {
		// erase
		delete(n.msgQueue, mt)
		return mt, msgs, true
	}
	// no we still have to wait!
	return mt, nil, false
}

// startProtocol calls the Start() on the underlying protocol which in turn will
// initiate the first message to its children
func (n *TreeNodeInstance) startProtocol() error {
	err := n.instance.Start()
	if err != nil {
		return xerrors.Errorf("starting protocol: %v", err)
	}
	return nil
}

// Done calls onDoneCallback if available and only finishes when the return-
// value is true.
func (n *TreeNodeInstance) Done() {
	if n.onDoneCallback != nil {
		ok := n.onDoneCallback()
		if !ok {
			return
		}
	}
	log.Lvl3(n.Info(), "has finished. Deleting its resources")
	n.overlay.nodeDone(n.token)
}

// OnDoneCallback should be called if we want to control the Done() of the node.
// It is used by protocols that uses others protocols inside and that want to
// control when the final Done() should be called.
// the function should return true if the real Done() has to be called otherwise
// false.
func (n *TreeNodeInstance) OnDoneCallback(fn func() bool) {
	n.onDoneCallback = fn
}

// SecretKey returns the private key of the service entity
func (n *TreeNodeInstance) SecretKey() ciphersuite.SecretKey {
	_, name := n.overlay.server.serviceManager.serviceByID(n.token.ServiceID)

	data := n.Host().ServerIdentity.ServicePrivate(name)
	sk, err := n.overlay.cr.UnpackSecretKey(data)
	if err != nil {
		log.Error(err)
		panic("Couldn't unpack the secret key of the server. Please check the " +
			"configuration of the server")
	}
	return sk
}

// PublicKey returns the public key of the service, either the specific
// or the default if not available
func (n *TreeNodeInstance) PublicKey() ciphersuite.PublicKey {
	_, name := n.overlay.server.serviceManager.serviceByID(n.token.ServiceID)

	data := n.Host().ServerIdentity.ServicePublic(name)
	pk, err := n.overlay.cr.UnpackPublicKey(data)
	if err != nil {
		log.Error(err)
		panic("Couldn't unpack the public key of the server. Please check the " +
			"configuration of the server.")
	}
	return pk
}

// PublicKeyIndex returns the index of the tree node in the roster or
// -1 if it is not found.
func (n *TreeNodeInstance) PublicKeyIndex() int {
	for i, si := range n.Roster().List {
		if si.Equal(n.ServerIdentity()) {
			return i
		}
	}
	return -1
}

// PublicKeys makes a list of public keys for the service
// associated with the instance
func (n *TreeNodeInstance) PublicKeys() []ciphersuite.PublicKey {
	_, name := n.overlay.server.serviceManager.serviceByID(n.token.ServiceID)

	pubkeys, err := n.Roster().PublicKeys(NewRegistryMapper(n.overlay.cr, name))
	if err != nil {
		log.Error(err)
		panic("Couldn't read the public keys for a service. Please check the " +
			"configuration of the server")
	}

	return pubkeys
}

// NodePublic returns the public key associated with the node's service
// stored in the given server identity
func (n *TreeNodeInstance) NodePublic(si *network.ServerIdentity) ciphersuite.PublicKey {
	_, name := n.overlay.server.serviceManager.serviceByID(n.token.ServiceID)

	data := si.ServicePublic(name)
	pk, err := n.overlay.cr.UnpackPublicKey(data)
	if err != nil {
		log.Error(err)
		panic("Couldn't unpack the public key of the distant node. Please check " +
			"the of the server.")
	}
	return pk
}

// CloseHost closes the underlying onet.Host (which closes the overlay
// and sends Shutdown to all protocol instances)
// NOTE: It is to be used VERY carefully and is likely to disappear in the next
// releases.
func (n *TreeNodeInstance) CloseHost() error {
	n.Host().callTestClose()
	err := n.Host().Close()
	if err != nil {
		return xerrors.Errorf("closing host: %v", err)
	}
	return nil
}

// Name returns a human readable name of this Node (IP address).
func (n *TreeNodeInstance) Name() string {
	return n.ServerIdentity().Address.String()
}

// Info returns a human readable representation name of this Node
// (IP address and TokenID).
func (n *TreeNodeInstance) Info() string {
	tid := n.TokenID()
	name := protocols.ProtocolIDToName(n.token.ProtoID)
	if name == "" {
		name = n.overlay.server.protocols.ProtocolIDToName(n.token.ProtoID)
	}
	return fmt.Sprintf("%s (%s): %s", n.ServerIdentity().Address, tid.String(), name)
}

// TokenID returns the TokenID of the given node (to uniquely identify it)
func (n *TreeNodeInstance) TokenID() TokenID {
	return n.token.ID()
}

// Token returns a CLONE of the underlying onet.Token struct.
// Useful for unit testing.
func (n *TreeNodeInstance) Token() *Token {
	return n.token.Clone()
}

// List returns the list of TreeNodes cached in the node (creating it if necessary)
func (n *TreeNodeInstance) List() []*TreeNode {
	n.mtx.Lock()
	t := n.Tree()
	if t != nil && n.treeNodeList == nil {
		n.treeNodeList = t.List()
	}
	n.mtx.Unlock()
	return n.treeNodeList
}

// Index returns the index of the node in the Roster
func (n *TreeNodeInstance) Index() int {
	return n.TreeNode().RosterIndex
}

// Broadcast sends a given message from the calling node directly to all other TreeNodes
func (n *TreeNodeInstance) Broadcast(msg interface{}) []error {
	var errs []error
	for _, node := range n.List() {
		if !node.Equal(n.TreeNode()) {
			if err := n.SendTo(node, msg); err != nil {
				errs = append(errs, xerrors.Errorf("sending: %v", err))
			}
		}
	}
	return errs
}

// Multicast ... XXX: should probably have a parallel more robust version like "SendToChildrenInParallel"
func (n *TreeNodeInstance) Multicast(msg interface{}, nodes ...*TreeNode) []error {
	var errs []error
	for _, node := range nodes {
		if err := n.SendTo(node, msg); err != nil {
			errs = append(errs, xerrors.Errorf("sending: %v", err))
		}
	}
	return errs
}

// SendToParent sends a given message to the parent of the calling node (unless it is the root)
func (n *TreeNodeInstance) SendToParent(msg interface{}) error {
	if n.IsRoot() {
		return nil
	}
	err := n.SendTo(n.Parent(), msg)
	if err != nil {
		return xerrors.Errorf("sending: %v", err)
	}
	return nil
}

// SendToChildren sends a given message to all children of the calling node.
// It stops sending if sending to one of the children fails. In that case it
// returns an error. If the underlying node is a leaf node this function does
// nothing.
func (n *TreeNodeInstance) SendToChildren(msg interface{}) error {
	if n.IsLeaf() {
		return nil
	}
	for _, node := range n.Children() {
		if err := n.SendTo(node, msg); err != nil {
			return xerrors.Errorf("sending: %v", err)
		}
	}
	return nil
}

// SendToChildrenInParallel sends a given message to all children of the calling
// node. It has the following differences to node.SendToChildren:
// The actual sending happens in a go routine (in parallel).
// It continues sending to the other nodes if sending to one of the children
// fails. In that case it will collect all errors in a slice.
// If the underlying node is a leaf node this function does
// nothing.
func (n *TreeNodeInstance) SendToChildrenInParallel(msg interface{}) []error {
	if n.IsLeaf() {
		return nil
	}
	children := n.Children()
	var errs []error
	eMut := sync.Mutex{}
	wg := sync.WaitGroup{}
	for _, node := range children {
		name := node.Name()
		wg.Add(1)
		go func(n2 *TreeNode) {
			if err := n.SendTo(n2, msg); err != nil {
				eMut.Lock()
				errs = append(errs, xerrors.Errorf("%s: %v", name, err))
				eMut.Unlock()
			}
			wg.Done()
		}(node)
	}
	wg.Wait()
	return errs
}

// CreateProtocol instantiates a new protocol of name "name" and
// returns it with any error that might have happened during the creation. If
// the TreeNodeInstance calling this is attached to a service, the new protocol
// will also be attached to this same service. Else the new protocol will only
// be handled by onet.
func (n *TreeNodeInstance) CreateProtocol(name string, t *Tree) (ProtocolInstance, error) {
	pi, err := n.overlay.CreateProtocol(name, t, n.Token().ServiceID)
	if err != nil {
		return nil, xerrors.Errorf("creating protocol: %v", err)
	}
	return pi, nil
}

// Host returns the underlying Host of this node.
//
// WARNING: you should not play with that feature unless you know what you are
// doing. This feature is meant to access the low level parts of the API. For
// example it is used to add a new tree config / new entity list to the Server.
func (n *TreeNodeInstance) Host() *Server {
	return n.overlay.server
}

// TreeNodeInstance returns itself (XXX quick hack for this services2 branch
// version for the tests)
func (n *TreeNodeInstance) TreeNodeInstance() *TreeNodeInstance {
	return n
}

// SetConfig sets the GenericConfig c to be passed down in the first message
// alongside with the protocol if it is non nil. This config can later be read
// by Services in the NewProtocol method.
func (n *TreeNodeInstance) SetConfig(c *GenericConfig) error {
	n.configMut.Lock()
	defer n.configMut.Unlock()
	if n.config != nil {
		return xerrors.New("Can't set config twice")
	}
	n.config = c
	return nil
}

// Rx implements the CounterIO interface
func (n *TreeNodeInstance) Rx() uint64 {
	return n.rx.get()
}

// Tx implements the CounterIO interface
func (n *TreeNodeInstance) Tx() uint64 {
	return n.tx.get()
}

func (n *TreeNodeInstance) isBound() bool {
	return n.instance != nil
}

func (n *TreeNodeInstance) bind(pi ProtocolInstance) {
	n.instance = pi
}
