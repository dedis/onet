package onet

import (
	"sync"
	"time"
)

// Key-value cache with an expiration time for each pair
// defined at the cache creation
// The implementation is robust against concurrent call
type cacheTTL struct {
	entries         map[interface{}]*cacheTTLEntry
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
		entries:         make(map[interface{}]*cacheTTLEntry),
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
func (c *cacheTTL) set(key interface{}, item interface{}) {
	c.Lock()
	c.entries[key] = &cacheTTLEntry{
		item:       item,
		expiration: time.Now().Add(c.entryExpiration),
	}
	c.Unlock()
}

// returns the cached item or nil
func (c *cacheTTL) get(key interface{}) interface{} {
	c.Lock()
	defer c.Unlock()

	entry := c.entries[key]
	if entry != nil {
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
	c.cacheTTL.set(tree.ID, tree)
}

// Get retrieves the tree with the given ID if it exists
// or returns nil
func (c *treeCacheTTL) Get(id TreeID) *Tree {
	tree := c.cacheTTL.get(id)
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
	c.cacheTTL.set(roster.ID, roster)
}

// Get retrieves the Roster with the given ID if it exists
// or it returns nil
func (c *rosterCacheTTL) Get(id RosterID) *Roster {
	roster := c.cacheTTL.get(id)
	if roster != nil {
		return roster.(*Roster)
	}

	return nil
}

// GetFromToken does the same as Get but with a token
func (c *rosterCacheTTL) GetFromToken(token *Token) *Roster {
	return c.Get(token.RosterID)
}
