package onet

import (
	"sync"
	"time"
)

type treeStorage struct {
	timeout       time.Duration
	lock          sync.Mutex
	wg            sync.WaitGroup
	trees         map[TreeID]*Tree
	cancellations map[TreeID]chan struct{}
}

func newTreeStorage(t time.Duration) *treeStorage {
	return &treeStorage{
		timeout:       t,
		trees:         make(map[TreeID]*Tree),
		cancellations: make(map[TreeID]chan struct{}),
	}
}

// Register creates the key for tree so it is known
func (ts *treeStorage) Register(id TreeID) {
	ts.lock.Lock()
	defer ts.lock.Unlock()

	ts.trees[id] = nil
}

// Unregister makes sure the tree is either set or the key is removed
func (ts *treeStorage) Unregister(id TreeID) {
	ts.lock.Lock()
	defer ts.lock.Unlock()

	if tree := ts.trees[id]; tree == nil {
		// if another goroutine set the tree inbetween, we keep the tree
		// but if it is nil, we need to remove the key
		delete(ts.trees, id)
	}
}

// IsRegistered returns true when the tree has previously been registered
func (ts *treeStorage) IsRegistered(id TreeID) bool {
	ts.lock.Lock()
	defer ts.lock.Unlock()

	_, ok := ts.trees[id]
	return ok
}

// Get returns the tree if it exists or nil
func (ts *treeStorage) Get(id TreeID) *Tree {
	ts.lock.Lock()
	defer ts.lock.Unlock()

	return ts.trees[id]
}

// Set sets the given tree adn cancel potential removal
func (ts *treeStorage) Set(tree *Tree) {
	ts.lock.Lock()
	defer ts.lock.Unlock()

	c := ts.cancellations[tree.ID]
	if c != nil {
		c <- struct{}{}
		delete(ts.cancellations, tree.ID)
	}

	ts.trees[tree.ID] = tree
}

// Remove starts a timeout to remove the tree from the storage
func (ts *treeStorage) Remove(id TreeID) {
	ts.lock.Lock()
	defer ts.lock.Unlock()

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
		// TODO: could this be optimized ?!
		case <-timer.C:
			ts.lock.Lock()
			delete(ts.trees, id)
			delete(ts.cancellations, id)
			ts.lock.Unlock()
		case <-c:
			timer.Stop()
			return
		}
	}()
}

// GetRoster looks for the roster in the list of trees or returns nil
func (ts *treeStorage) GetRoster(id RosterID) *Roster {
	ts.lock.Lock()
	defer ts.lock.Unlock()

	for _, tree := range ts.trees {
		if tree.Roster.ID.Equal(id) {
			return tree.Roster
		}
	}

	return nil
}

// Close forces cleaning goroutines to be shutdown
func (ts *treeStorage) Close() {
	ts.lock.Lock()
	defer ts.lock.Unlock()

	for k, c := range ts.cancellations {
		c <- struct{}{}
		delete(ts.cancellations, k)
	}

	ts.wg.Wait()
}
