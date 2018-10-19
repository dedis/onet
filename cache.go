package onet

import (
	"sync"
	"time"

	uuid "github.com/satori/go.uuid"
)

// Key-value cache with an expiration time for each pair
// defined at the cache creation
// The implementation is robust against concurrent call
type cacheTTL struct {
	entries         map[uuid.UUID]*cacheTTLEntry
	stopCh          chan (struct{})
	stopOnce        sync.Once
	cleanInterval   time.Duration
	entryExpiration time.Duration
	sync.Mutex
}

type cacheTTLEntry struct {
	item       interface{}
	expiration time.Time
}

// create a generic cache and start the cleaning routine
func newCacheTTL(interval, expiration time.Duration) *cacheTTL {
	c := &cacheTTL{
		entries:         make(map[uuid.UUID]*cacheTTLEntry),
		stopCh:          make(chan (struct{})),
		cleanInterval:   interval,
		entryExpiration: expiration,
	}
	go c.cleaner()
	return c
}

// Stop the cleaning routine
func (c *cacheTTL) stop() {
	c.stopOnce.Do(func() {
		close(c.stopCh)
	})
}

// Wait for either a clean or a stop order
func (c *cacheTTL) cleaner() {
	for {
		select {
		case <-time.After(c.cleanInterval):
			c.clean()
		case <-c.stopCh:
			return
		}
	}
}

// Check and delete expired items
func (c *cacheTTL) clean() {
	c.Lock()
	now := time.Now()
	for k, e := range c.entries {
		if now.After(e.expiration) {
			delete(c.entries, k)
		}
	}
	c.Unlock()
}

// add the item to the cache with the given key
func (c *cacheTTL) set(key uuid.UUID, item interface{}) {
	c.Lock()
	c.entries[key] = &cacheTTLEntry{
		item:       item,
		expiration: time.Now().Add(c.entryExpiration),
	}
	c.Unlock()
}

// returns the cached item or nil
func (c *cacheTTL) get(key uuid.UUID) interface{} {
	c.Lock()
	defer c.Unlock()

	entry, ok := c.entries[key]
	if ok && time.Now().Before(entry.expiration) {
		entry.expiration = time.Now().Add(c.entryExpiration)
		return entry.item
	}

	return nil
}

type treeCacheTTL struct {
	*cacheTTL
}

func newTreeCache(interval, expiration time.Duration) *treeCacheTTL {
	return &treeCacheTTL{
		cacheTTL: newCacheTTL(interval, expiration),
	}
}

// Set stores the given tree in the cache
func (c *treeCacheTTL) Set(tree *Tree) {
	c.set(uuid.UUID(tree.ID), tree)
}

// Get retrieves the tree with the given ID if it exists
// or returns nil
func (c *treeCacheTTL) Get(id TreeID) *Tree {
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

func newRosterCache(interval, expiration time.Duration) *rosterCacheTTL {
	return &rosterCacheTTL{
		cacheTTL: newCacheTTL(interval, expiration),
	}
}

// Set stores the roster in the cache
func (c *rosterCacheTTL) Set(roster *Roster) {
	c.set(uuid.UUID(roster.ID), roster)
}

// Get retrieves the Roster with the given ID if it exists
// or it returns nil
func (c *rosterCacheTTL) Get(id RosterID) *Roster {
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

func newTreeNodeCache(interval, expiration time.Duration) *treeNodeCacheTTL {
	return &treeNodeCacheTTL{
		cacheTTL: newCacheTTL(interval, expiration),
	}
}

func (c *treeNodeCacheTTL) Set(tree *Tree, treeNode *TreeNode) {
	c.Lock()
	ce, ok := c.entries[uuid.UUID(tree.ID)]
	if !ok {
		ce = &cacheTTLEntry{
			item:       make(map[TreeNodeID]*TreeNode),
			expiration: time.Now().Add(c.entryExpiration),
		}
	}
	treeNodeMap := ce.item.(map[TreeNodeID]*TreeNode)
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
	c.entries[uuid.UUID(tree.ID)] = ce
	c.Unlock()
}

func (c *treeNodeCacheTTL) GetFromToken(tok *Token) *TreeNode {
	c.Lock()
	defer c.Unlock()
	if tok == nil {
		return nil
	}
	ce, ok := c.entries[uuid.UUID(tok.TreeID)]
	if !ok || time.Now().After(ce.expiration) {
		// no tree cached for this token
		return nil
	}
	ce.expiration = time.Now().Add(c.entryExpiration)

	treeNodeMap := ce.item.(map[TreeNodeID]*TreeNode)
	tn, ok := treeNodeMap[tok.TreeNodeID]
	if !ok {
		// no treeNode cached for this token
		return nil
	}
	return tn
}
