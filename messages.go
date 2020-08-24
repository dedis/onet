package onet

import (
	uuid "github.com/satori/go.uuid"
	"go.dedis.ch/onet/v3/network"
)

// ProtocolMsgID is to be embedded in every message that is made for a
// ID of ProtocolMsg message as registered in network
var ProtocolMsgID = network.RegisterMessage(ProtocolMsg{})

// RequestTreeMsgID of RequestTree message as registered in network
var RequestTreeMsgID = network.RegisterMessage(RequestTree{})

// ResponseTreeMsgID of TreeMarshal message as registered in network
var ResponseTreeMsgID = network.RegisterMessage(ResponseTree{})

// SendTreeMsgID of TreeMarshal message as registered in network
// Deprecated: use ResponseTreeMsgID
var SendTreeMsgID = TreeMarshalTypeID

// RequestRosterMsgID of RequestRoster message as registered in network
// Deprecated: only the tree is sent, not anymore the roster
var RequestRosterMsgID = network.RegisterMessage(RequestRoster{})

// SendRosterMsgID of Roster message as registered in network
// Deprecated: only the tree is sent, not anymore the roster
var SendRosterMsgID = RosterTypeID

// ConfigMsgID of the generic config message
var ConfigMsgID = network.RegisterMessage(ConfigMsg{})

// ProtocolMsg is to be embedded in every message that is made for a
// ProtocolInstance
type ProtocolMsg struct {
	// Token uniquely identify the protocol instance this msg is made for
	From *Token
	// The TreeNodeId Where the message goes to
	To *Token
	// NOTE: this is taken from network.NetworkMessage
	ServerIdentity *network.ServerIdentity
	// MsgType of the underlying data
	MsgType network.MessageTypeID
	// The interface to the actual Data
	Msg network.Message
	// The actual data as binary blob
	MsgSlice []byte
	// The size of the data
	Size network.Size
	// Config is the config passed to the protocol constructor.
	Config *GenericConfig
}

// ConfigMsg is sent by the overlay containing a generic slice of bytes to
// give to service in the `NewProtocol` method.
type ConfigMsg struct {
	Config GenericConfig
	Dest   TokenID
}

// RoundID uniquely identifies a round of a protocol run
type RoundID uuid.UUID

// String returns the canonical representation of the rounds ID (wrapper around // uuid.UUID.String())
func (rId RoundID) String() string {
	return uuid.UUID(rId).String()
}

// Equal returns true if and only if rID2 equals this RoundID.
func (rId RoundID) Equal(rID2 RoundID) bool {
	return uuid.Equal(uuid.UUID(rId), uuid.UUID(rID2))
}

// IsNil returns true iff the RoundID is Nil
func (rId RoundID) IsNil() bool {
	return rId.Equal(RoundID(uuid.Nil))
}

// TokenID uniquely identifies the start and end-point of a message by an ID
// (see Token struct)
type TokenID uuid.UUID

// String returns the canonical representation of the TokenID (wrapper around // uuid.UUID.String())
func (t TokenID) String() string {
	return uuid.UUID(t).String()
}

// Equal returns true if and only if t2 equals this TokenID.
func (t TokenID) Equal(t2 TokenID) bool {
	return uuid.Equal(uuid.UUID(t), uuid.UUID(t2))
}

// IsNil returns true iff the TokenID is Nil
func (t TokenID) IsNil() bool {
	return t.Equal(TokenID(uuid.Nil))
}

// A Token contains all identifiers needed to uniquely identify one protocol
// instance. It gets passed when a new protocol instance is created and get used
// by every protocol instance when they want to send a message. That way, the
// host knows how to create the ProtocolMsg message around the protocol's message
// with the right fields set.
type Token struct {
	RosterID RosterID
	TreeID   TreeID
	// TO BE REMOVED
	ProtoID   ProtocolID
	ServiceID ServiceID
	RoundID   RoundID
	// TreeNodeID is defined by the
	TreeNodeID TreeNodeID
}

// ID returns the TokenID which can be used to identify by token in map
func (t *Token) ID() TokenID {
	// TODO: This used to have a caching mechanism, but it caused data races.
	// See issue #239. When tuning performance, if this shows up as a hot path,
	// we need to add caching back in (safely, this time).
	url := network.NamespaceURL + "token/" + t.RosterID.String() +
		t.RoundID.String() + t.ServiceID.String() + t.ProtoID.String() + t.TreeID.String() +
		t.TreeNodeID.String()
	return TokenID(uuid.NewV5(uuid.NamespaceURL, url))
}

// Clone returns a new token out of this one
func (t *Token) Clone() *Token {
	t2 := *t
	return &t2
}

// ChangeTreeNodeID return a new Token containing a reference to the given
// TreeNode
func (t *Token) ChangeTreeNodeID(newid TreeNodeID) *Token {
	tOther := *t
	tOther.TreeNodeID = newid
	return &tOther
}

// TreeNodeInfo holds the sender and the destination of the message.
type TreeNodeInfo struct {
	To   *Token
	From *Token
}

// OverlayMsg contains all routing-information about the tree and the
// roster.
type OverlayMsg struct {
	TreeNodeInfo *TreeNodeInfo

	// Deprecated: roster is not sent/requested anymore, only the tree
	RequestRoster *RequestRoster
	// Deprecated: roster is not sent/requested anymore, only the tree
	Roster *Roster

	RequestTree  *RequestTree
	ResponseTree *ResponseTree
	// Deprecated: use ResponseTree to send the tree and the roster
	TreeMarshal *TreeMarshal

	Config *GenericConfig
}

// RequestRoster is used to ask the parent for a given Roster
type RequestRoster struct {
	RosterID RosterID
}

// RequestTree is used to ask the parent for a given Tree
type RequestTree struct {
	// The treeID of the tree we want
	TreeID TreeID
	// Version of the request tree
	Version uint32
}

// ResponseTree contains the information to build a tree
type ResponseTree struct {
	TreeMarshal *TreeMarshal
	Roster      *Roster
}

// RosterUnknown is used in case the entity list is unknown
type RosterUnknown struct {
}
