package onet

import (
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const treeStoreTimeout = 200 * time.Millisecond

func checkLeakingGoroutines(t *testing.T) {
	buf := make([]byte, 2<<20)
	buf = buf[:runtime.Stack(buf, true)]
	require.False(t, strings.Contains(string(buf), "(*treeStorage).Remove"))
}

// Tests the main use cases
func TestTreeStorage_SimpleCase(t *testing.T) {
	store := newTreeStorage(treeStoreTimeout)

	tree := &Tree{ID: TreeID{1}}
	require.False(t, store.IsRegistered(tree.ID))

	store.Register(tree.ID)
	require.True(t, store.IsRegistered(tree.ID))

	require.Nil(t, store.Get(tree.ID))

	store.Set(tree)
	require.NotNil(t, store.Get(tree.ID))

	store.Remove(tree.ID)
	require.NotNil(t, store.Get(tree.ID))

	time.Sleep(treeStoreTimeout + 50*time.Millisecond)
	require.Nil(t, store.Get(tree.ID))
	require.False(t, store.IsRegistered(tree.ID))
}

// Tests the behaviour of Unregister
func TestTreeStorage_Registration(t *testing.T) {
	store := newTreeStorage(treeStoreTimeout)

	tree := &Tree{ID: TreeID{1}}

	store.Register(tree.ID)
	require.True(t, store.IsRegistered(tree.ID))

	store.Unregister(tree.ID)
	require.False(t, store.IsRegistered(tree.ID))

	store.Set(tree)
	store.Unregister(tree.ID)
	require.True(t, store.IsRegistered(tree.ID))
}

func TestTreeStorage_GetRoster(t *testing.T) {
	store := newTreeStorage(treeStoreTimeout)

	store.Set(&Tree{Roster: &Roster{ID: RosterID{1}}})
	store.Set(&Tree{Roster: &Roster{ID: RosterID{2}}})

	require.NotNil(t, store.GetRoster(RosterID{2}))
	require.Nil(t, store.GetRoster(RosterID{3}))
}

// Tests that the tree won't be removed if it is set again after a remove
func TestTreeStorage_CancelDeletion(t *testing.T) {
	store := newTreeStorage(treeStoreTimeout)

	tree := &Tree{ID: TreeID{1}}
	store.Set(tree)

	store.Remove(tree.ID)

	time.Sleep(50 * time.Millisecond)
	store.Set(tree)

	time.Sleep(treeStoreTimeout + 50*time.Millisecond)
	require.NotNil(t, store.Get(tree.ID))
}

// Tests if planned removals are correctly stopped
func TestTreeStorage_Close(t *testing.T) {
	store := newTreeStorage(treeStoreTimeout)

	trees := []*Tree{
		&Tree{ID: TreeID{1}},
		&Tree{ID: TreeID{2}},
		&Tree{ID: TreeID{3}},
	}

	for _, tree := range trees {
		store.Set(tree)
		store.Remove(tree.ID)
		store.Remove(tree.ID)
	}

	store.Close()
	// make sure several calls are ok
	store.Close()

	checkLeakingGoroutines(t)
}

// This test is intented to be run with -race to detect race conditions
func TestTreeStorage_Race(t *testing.T) {
	store := newTreeStorage(treeStoreTimeout)

	trees := []*Tree{
		&Tree{ID: TreeID{1}},
		&Tree{ID: TreeID{2}},
		&Tree{ID: TreeID{3}},
	}

	n := 1000
	k := 20
	wg := sync.WaitGroup{}
	lock := sync.Mutex{}

	for i := 0; i < k; i++ {
		wg.Add(1)

		go func() {
			for j := 0; j < n; j++ {
				store.Set(trees[0])
				store.Remove(trees[2].ID)
				store.Get(trees[1].ID)
				store.Set(trees[2])
				store.Set(trees[1])
				store.Get(trees[2].ID)
				store.Get(trees[1].ID)
				store.Remove(trees[0].ID)
			}

			lock.Lock()
			wg.Done()
			lock.Unlock()
		}()
	}

	wg.Wait()
	store.wg.Wait()
	require.Equal(t, 2, len(store.trees))

	checkLeakingGoroutines(t)
}
