package onet

import (
	"sync"
	"time"
)

type treeStorage struct {
	sync.Mutex
	timeout       time.Duration
	wg            sync.WaitGroup
	trees         map[TreeID]*Tree
	cancellations map[TreeID]chan struct{}
	closed        bool
}

func newTreeStorage(t time.Duration) *treeStorage {
	return &treeStorage{
		timeout:       t,
		trees:         make(map[TreeID]*Tree),
		cancellations: make(map[TreeID]chan struct{}),
		closed:        false,
	}
}

// Register creates the key for tree so it is known
func (ts *treeStorage) Register(id TreeID) {
	ts.Lock()
	ts.trees[id] = nil
	ts.Unlock()
}

// Unregister makes sure the tree is either set or the key is removed
func (ts *treeStorage) Unregister(id TreeID) {
	ts.Lock()
	defer ts.Unlock()

	if tree := ts.trees[id]; tree == nil {
		// if another goroutine set the tree inbetween, we keep the tree
		// but if it is nil, we need to remove the key
		delete(ts.trees, id)
	}
}

// IsRegistered returns true when the tree has previously been registered
func (ts *treeStorage) IsRegistered(id TreeID) bool {
	ts.Lock()
	defer ts.Unlock()

	_, ok := ts.trees[id]
	return ok
}

// Get returns the tree if it exists or nil
func (ts *treeStorage) Get(id TreeID) *Tree {
	ts.Lock()
	defer ts.Unlock()

	return ts.trees[id]
}

// getAndRefresh cancels any pending remove and return the
// tree if it exists.
func (ts *treeStorage) getAndRefresh(id TreeID) *Tree {
	ts.Lock()
	defer ts.Unlock()

	ts.cancelDeletion(id)

	return ts.trees[id]
}

// Set sets the given tree and cancel potential removal
func (ts *treeStorage) Set(tree *Tree) {
	ts.Lock()
	defer ts.Unlock()

	ts.cancelDeletion(tree.ID)

	ts.trees[tree.ID] = tree
}

// Remove starts a timeout to remove the tree from the storage
func (ts *treeStorage) Remove(id TreeID) {
	ts.Lock()
	defer ts.Unlock()

	if ts.closed {
		// server is closing so we avoid creating new goroutines
		return
	}

	_, ok := ts.cancellations[id]
	if ok {
		// already planned to be removed
		return
	}

	ts.wg.Add(1)
	c := make(chan struct{})
	ts.cancellations[id] = c

	go func() {
		defer ts.wg.Done()

		timer := time.NewTimer(ts.timeout)

		select {
		// other distant node instances of the protocol could ask for the tree even
		// after we're done locally and then it needs to be kept around for some time
		case <-timer.C:
			ts.Lock()
			delete(ts.trees, id)
			delete(ts.cancellations, id)
			ts.Unlock()
		case <-c:
			timer.Stop()
			return
		}
	}()
}

// GetRoster looks for the roster in the list of trees or returns nil
func (ts *treeStorage) GetRoster(id RosterID) *Roster {
	ts.Lock()
	defer ts.Unlock()

	for _, tree := range ts.trees {
		if tree.Roster.ID.Equal(id) {
			return tree.Roster
		}
	}

	return nil
}

// Close forces cleaning goroutines to be shutdown
func (ts *treeStorage) Close() {
	ts.Lock()
	defer ts.Unlock()

	// prevent further call to remove because the server is closing anyway
	ts.closed = true

	for k, c := range ts.cancellations {
		close(c)
		delete(ts.cancellations, k)
	}

	ts.wg.Wait()
}

// cancelDeletion prevents any pending remove request
// to be triggered for the tree.
// Caution: caller is reponsible for holding the lock.
func (ts *treeStorage) cancelDeletion(id TreeID) {
	c := ts.cancellations[id]
	if c != nil {
		close(c)
		delete(ts.cancellations, id)
	}
}
