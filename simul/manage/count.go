package manage

import (
	"sync"
	"time"

	"go.dedis.ch/onet/v3"
	"go.dedis.ch/onet/v3/log"
	"go.dedis.ch/onet/v3/network"
	"golang.org/x/xerrors"
)

/*
The count-protocol returns the number of nodes reachable in a given
timeout. To correctly wait for the whole tree, every node that receives
a message sends a message to the root before contacting its children.
As long as the root receives those messages, he knows the counting
still goes on.
*/

func init() {
	network.RegisterMessage(PrepareCount{})
	network.RegisterMessage(Count{})
	network.RegisterMessage(NodeIsUp{})
	onet.GlobalProtocolRegister("Count", NewCount)
}

// ProtocolCount holds all channels. If a timeout occurs or the counting
// is done, the Count-channel receives the number of nodes reachable in
// the tree.
type ProtocolCount struct {
	*onet.TreeNodeInstance
	Replies          int
	Count            chan int
	Quit             chan bool
	timeout          time.Duration
	timeoutMu        sync.Mutex
	PrepareCountChan chan struct {
		*onet.TreeNode
		PrepareCount
	}
	CountChan    chan []CountMsg
	NodeIsUpChan chan struct {
		*onet.TreeNode
		NodeIsUp
	}
}

// PrepareCount is sent so that every node can contact the root to say
// the counting is still going on.
type PrepareCount struct {
	Timeout time.Duration
}

// NodeIsUp - if it is received by the root it will reset the counter.
type NodeIsUp struct{}

// Count sends the number of children to the parent node.
type Count struct {
	Children int32
}

// CountMsg is wrapper around the Count-structure
type CountMsg struct {
	*onet.TreeNode
	Count
}

// NewCount returns a new protocolInstance
func NewCount(n *onet.TreeNodeInstance) (onet.ProtocolInstance, error) {
	p := &ProtocolCount{
		TreeNodeInstance: n,
		Quit:             make(chan bool),
		timeout:          1 * time.Second,
	}
	p.Count = make(chan int, 1)
	t := n.Tree()
	if t == nil {
		return nil, xerrors.New("cannot find tree")
	}
	if err := p.RegisterChannelsLength(len(t.List()),
		&p.CountChan, &p.PrepareCountChan, &p.NodeIsUpChan); err != nil {
		log.Error("Couldn't reister channel:", err)
	}
	return p, nil
}

// Start the protocol
func (p *ProtocolCount) Start() error {
	// Send an empty message
	log.Lvl3("Starting to count")
	p.FuncPC()
	return nil
}

// Dispatch listens for all channels and waits for a timeout in case nothing
// happens for a certain duration
func (p *ProtocolCount) Dispatch() error {
	running := true
	for running {
		log.Lvl3(p.Info(), "waiting for message for", p.Timeout())
		select {
		case pc := <-p.PrepareCountChan:
			log.Lvl3(p.Info(), "received from", pc.TreeNode.ServerIdentity.Address,
				"timeout", pc.Timeout)
			p.SetTimeout(pc.Timeout)
			p.FuncPC()
		case c := <-p.CountChan:
			p.FuncC(c)
			running = false
		case _ = <-p.NodeIsUpChan:
			if p.Parent() != nil {
				err := p.SendTo(p.Parent(), &NodeIsUp{})
				if err != nil {
					log.Lvl2(p.Info(), "couldn't send to parent",
						p.Parent().Name(), err)
				}
			} else {
				p.Replies++
			}
		case <-time.After(time.Duration(p.Timeout())):
			log.Lvl3(p.Info(), "timed out while waiting for", p.Timeout())
			if p.IsRoot() {
				log.Lvl2("Didn't get all children in time:", p.Replies)
				p.Count <- p.Replies
				running = false
			}
		}
	}
	p.Done()
	return nil
}

// FuncPC handles PrepareCount messages. These messages go down the tree and
// every node that receives one will reply with a 'NodeIsUp'-message
func (p *ProtocolCount) FuncPC() {
	if !p.IsRoot() {
		err := p.SendTo(p.Parent(), &NodeIsUp{})
		if err != nil {
			log.Lvl2(p.Info(), "couldn't send to parent",
				p.Parent().Name(), err)
		}
	}
	if !p.IsLeaf() {
		for _, child := range p.Children() {
			go func(c *onet.TreeNode) {
				log.Lvl3(p.Info(), "sending to", c.ServerIdentity.Address, c.ID, "timeout", p.timeout)
				err := p.SendTo(c, &PrepareCount{Timeout: p.timeout})
				if err != nil {
					log.Lvl2(p.Info(), "couldn't send to child",
						c.Name())
				}
			}(child)
		}
	} else {
		p.CountChan <- nil
	}
}

// FuncC creates a Count-message that will be received by all parents and
// count the total number of children
func (p *ProtocolCount) FuncC(cc []CountMsg) {
	count := 1
	for _, c := range cc {
		count += int(c.Count.Children)
	}
	if !p.IsRoot() {
		log.Lvl3(p.Info(), "Sends to", p.Parent().ID, p.Parent().ServerIdentity.Address)
		if err := p.SendTo(p.Parent(), &Count{int32(count)}); err != nil {
			log.Lvl2(p.Name(), "coouldn't send to parent",
				p.Parent().Name())
		}
	} else {
		p.Count <- count
	}
	log.Lvl3(p.ServerIdentity().Address, "Done")
}

// SetTimeout sets the new timeout
func (p *ProtocolCount) SetTimeout(t time.Duration) {
	p.timeoutMu.Lock()
	p.timeout = t
	p.timeoutMu.Unlock()
}

// Timeout returns the current timeout
func (p *ProtocolCount) Timeout() time.Duration {
	p.timeoutMu.Lock()
	defer p.timeoutMu.Unlock()
	return p.timeout
}
