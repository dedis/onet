package onet

import (
	"sync"
	"time"

	uuid "github.com/satori/go.uuid"
	"gopkg.in/dedis/onet.v2/log"
)

// Key-value cache with an expiration time for each pair
// defined at the cache creation
// The implementation is robust against concurrent call
type cacheTTL struct {
	entries         map[uuid.UUID]*cacheTTLEntry
	head            *cacheTTLEntry
	tail            *cacheTTLEntry
	entryExpiration time.Duration
	size            int
	sync.Mutex
}

type cacheTTLEntry struct {
	key        uuid.UUID
	item       interface{}
	expiration time.Time
	prev       *cacheTTLEntry
	next       *cacheTTLEntry
}

// create a generic cache and start the cleaning routine
func newCacheTTL(expiration time.Duration, size int) *cacheTTL {
	if size == 0 {
		log.Error("Cannot instantiate a cache with a size of 0")
		return nil
	}

	return &cacheTTL{
		entries:         make(map[uuid.UUID]*cacheTTLEntry),
		entryExpiration: expiration,
		size:            size,
	}
}

// add the item to the cache with the given key
func (c *cacheTTL) set(key uuid.UUID, item interface{}) {
	entry := c.entries[key]
	if entry != nil {
		entry.expiration = time.Now().Add(c.entryExpiration)
		entry.item = item
	} else {
		entry = &cacheTTLEntry{
			key:        key,
			item:       item,
			expiration: time.Now().Add(c.entryExpiration),
		}

		c.clean() // clean before checking the size
		if len(c.entries) >= c.size && c.tail != nil {
			// deletes the oldest entry and ignore edge cases with size = 0
			delete(c.entries, c.tail.key)
			c.tail = c.tail.next
			c.tail.prev = nil
		}
	}

	c.moveToHead(entry)
	c.entries[key] = entry
}

// returns the cached item or nil
func (c *cacheTTL) get(key uuid.UUID) interface{} {
	entry, ok := c.entries[key]
	if ok && time.Now().Before(entry.expiration) {
		entry.expiration = time.Now().Add(c.entryExpiration)
		c.moveToHead(entry)
		return entry.item
	}

	c.clean() // defensive cleaning
	return nil
}

func (c *cacheTTL) moveToHead(e *cacheTTLEntry) {
	if c.head == e {
		// already at the top of the list
		return
	}

	if c.tail == e && e.next != nil {
		// item was the tail so we need to assign the new tail
		c.tail = e.next
	}
	// remove the list entry from its previous position
	if e.next != nil {
		e.next.prev = e.prev
	}
	if e.prev != nil {
		e.prev.next = e.next
	}

	// assign the entry at the top of the list
	if c.head == nil {
		c.head = e
		if c.tail == nil {
			c.tail = c.head
		}
	} else {
		c.head.next = e
		e.prev = c.head
		e.next = nil
		c.head = e
	}
}

func (c *cacheTTL) clean() {
	now := time.Now()

	for c.tail != nil && now.After(c.tail.expiration) {
		delete(c.entries, c.tail.key)

		if c.head == c.tail {
			c.head = nil
			c.tail = nil
		} else {
			c.tail = c.tail.next
		}
	}
}

type treeCacheTTL struct {
	*cacheTTL
}

func newTreeCache(expiration time.Duration, size int) *treeCacheTTL {
	return &treeCacheTTL{
		cacheTTL: newCacheTTL(expiration, size),
	}
}

// Set stores the given tree in the cache
func (c *treeCacheTTL) Set(tree *Tree) {
	c.Lock()
	c.set(uuid.UUID(tree.ID), tree)
	c.Unlock()
}

// Get retrieves the tree with the given ID if it exists
// or returns nil
func (c *treeCacheTTL) Get(id TreeID) *Tree {
	c.Lock()
	defer c.Unlock()

	tree := c.get(uuid.UUID(id))
	if tree != nil {
		return tree.(*Tree)
	}

	return nil
}

// GetFromToken does the same as Get but with a token
func (c *treeCacheTTL) GetFromToken(token *Token) *Tree {
	return c.Get(token.TreeID)
}

type rosterCacheTTL struct {
	*cacheTTL
}

func newRosterCache(expiration time.Duration, size int) *rosterCacheTTL {
	return &rosterCacheTTL{
		cacheTTL: newCacheTTL(expiration, size),
	}
}

// Set stores the roster in the cache
func (c *rosterCacheTTL) Set(roster *Roster) {
	c.Lock()
	c.set(uuid.UUID(roster.ID), roster)
	c.Unlock()
}

// Get retrieves the Roster with the given ID if it exists
// or it returns nil
func (c *rosterCacheTTL) Get(id RosterID) *Roster {
	c.Lock()
	defer c.Unlock()

	roster := c.get(uuid.UUID(id))
	if roster != nil {
		return roster.(*Roster)
	}

	return nil
}

// GetFromToken does the same as Get but with a token
func (c *rosterCacheTTL) GetFromToken(token *Token) *Roster {
	return c.Get(token.RosterID)
}

// treeNodeCacheTTL is a cache that maps from token to treeNode. Since
// the mapping is not 1-1 (many Tokens can point to one TreeNode, but
// one token leads to one TreeNode), we have to do certain lookup, but
// that's better than searching the tree each time.
type treeNodeCacheTTL struct {
	*cacheTTL
}

func newTreeNodeCache(expiration time.Duration, size int) *treeNodeCacheTTL {
	return &treeNodeCacheTTL{
		cacheTTL: newCacheTTL(expiration, size),
	}
}

func (c *treeNodeCacheTTL) Set(tree *Tree, treeNode *TreeNode) {
	c.Lock()

	var treeNodeMap map[TreeNodeID]*TreeNode
	e := c.get(uuid.UUID(tree.ID))
	if e == nil {
		treeNodeMap = make(map[TreeNodeID]*TreeNode)
	} else {
		treeNodeMap = e.(map[TreeNodeID]*TreeNode)
	}

	// add treenode
	treeNodeMap[treeNode.ID] = treeNode
	// add parent if not root
	if treeNode.Parent != nil {
		treeNodeMap[treeNode.Parent.ID] = treeNode.Parent
	}
	// add children
	for _, c := range treeNode.Children {
		treeNodeMap[c.ID] = c
	}
	// add cache
	c.set(uuid.UUID(tree.ID), treeNodeMap)
	c.Unlock()
}

func (c *treeNodeCacheTTL) GetFromToken(tok *Token) *TreeNode {
	c.Lock()
	defer c.Unlock()

	if tok == nil {
		return nil
	}

	e := c.get(uuid.UUID(tok.TreeID))
	if e == nil {
		return nil
	}

	treeNodeMap := e.(map[TreeNodeID]*TreeNode)
	tn, ok := treeNodeMap[tok.TreeNodeID]
	if !ok {
		// no treeNode cached for this token
		return nil
	}
	return tn
}
