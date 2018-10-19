package onet

import (
	"sync"
	"testing"
	"time"

	"github.com/satori/go.uuid"
	"github.com/stretchr/testify/require"
)

const nbrOfItems = 20

func initTrees(tt []*Tree) {
	for i := range tt {
		tt[i] = &Tree{}
		id, _ := uuid.NewV1()
		tt[i].ID = TreeID(id)
	}
}

func TestTreeCache(t *testing.T) {
	cache := newTreeCache(25*time.Millisecond, 100*time.Millisecond)
	defer cache.stop()

	trees := make([]*Tree, nbrOfItems)
	initTrees(trees)

	var wg sync.WaitGroup
	wg.Add(2)

	f := func() {
		for _, tree := range trees {
			cache.Set(tree)
			require.NotNil(t, cache.Get(tree.ID))
			time.Sleep(10 * time.Millisecond)
		}
		wg.Done()
	}

	go f()
	time.Sleep(100 * time.Millisecond)
	go f()

	wg.Wait()

	token := &Token{}
	for _, tree := range trees[nbrOfItems-5:] {
		token.TreeID = tree.ID
		require.Equal(t, tree, cache.GetFromToken(token))
	}

	time.Sleep(125 * time.Millisecond)

	for _, tree := range trees {
		token.TreeID = tree.ID
		require.Nil(t, cache.GetFromToken(token))
	}
}

func TestRosterCache(t *testing.T) {
	cache := newRosterCache(25*time.Millisecond, 100*time.Millisecond)
	defer cache.stop()

	r := &Roster{}
	id, _ := uuid.NewV1()
	r.ID = RosterID(id)

	cache.Set(r)

	token := &Token{}
	token.RosterID = r.ID
	require.Equal(t, r, cache.GetFromToken(token))
}

func TestExpirationAndCleaning(t *testing.T) {
	cache := newTreeCache(25*time.Millisecond, 100*time.Millisecond)
	defer cache.stop()

	tt := make([]*Tree, 2)
	initTrees(tt)

	cache.Set(tt[0])
	time.Sleep(50 * time.Millisecond)
	cache.Set(tt[1])
	time.Sleep(75 * time.Millisecond)

	token := &Token{}
	token.TreeID = tt[1].ID
	require.Equal(t, tt[1], cache.GetFromToken(token))
	token.TreeID = tt[0].ID
	require.Nil(t, cache.GetFromToken(token))
}
