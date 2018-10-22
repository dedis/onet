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
			// test if it persists with the Get request
			cache.Get(trees[0].ID)
		}
		wg.Done()
	}

	go f()
	time.Sleep(100 * time.Millisecond)
	go f()

	wg.Wait()

	token := &Token{}
	for _, tree := range trees[nbrOfItems-8:] {
		token.TreeID = tree.ID
		require.Equal(t, tree, cache.GetFromToken(token))
	}

	require.Equal(t, trees[0], cache.Get(trees[0].ID))
	require.Nil(t, cache.Get(trees[1].ID))

	time.Sleep(125 * time.Millisecond)

	for _, tree := range trees {
		token.TreeID = tree.ID
		require.Nil(t, cache.GetFromToken(token))
	}
}

func TestRosterCache(t *testing.T) {
	cache := newRosterCache(1*time.Minute, 50*time.Millisecond)
	defer cache.stop()

	r := &Roster{}
	id, _ := uuid.NewV1()
	r.ID = RosterID(id)

	cache.Set(r)

	token := &Token{}
	token.RosterID = r.ID
	require.Equal(t, r, cache.GetFromToken(token))

	// test that get ignore expired item
	time.Sleep(100 * time.Millisecond)
	require.Nil(t, cache.GetFromToken(token))
}

func generateID() uuid.UUID {
	id, _ := uuid.NewV1()
	return id
}

func TestTreeNodeCache(t *testing.T) {
	cache := newTreeNodeCache(1*time.Minute, 50*time.Millisecond)
	defer cache.stop()

	tree := &Tree{ID: TreeID(generateID())}
	tn1 := &TreeNode{ID: TreeNodeID(generateID())}
	tn2 := &TreeNode{ID: TreeNodeID(generateID())}

	cache.Set(tree, tn1)
	cache.Set(tree, tn2)

	tok := &Token{TreeID: tree.ID, TreeNodeID: tn1.ID}
	require.Equal(t, tn1, cache.GetFromToken(tok))
	tok.TreeNodeID = tn2.ID
	require.Equal(t, tn2, cache.GetFromToken(tok))
	tok.TreeNodeID = TreeNodeID(generateID())
	require.Nil(t, cache.GetFromToken(tok))

	require.Nil(t, cache.GetFromToken(&Token{}))

	// test that get ignore expired item
	time.Sleep(100 * time.Millisecond)
	require.Nil(t, cache.GetFromToken(tok))
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
