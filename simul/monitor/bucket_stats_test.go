package monitor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBucketStats_Rules tests that a bucket rule can be used
// as expected and that it matches the correct hosts only.
func TestBucketStats_Rules(t *testing.T) {
	rr := bucketRules{}
	r, err := newBucketRule("0:1")
	require.NoError(t, err)
	rr = append(rr, r)

	r, err = newBucketRule("5:7")
	require.NoError(t, err)
	rr = append(rr, r)

	require.True(t, rr.Match(0))
	require.True(t, rr.Match(5))
	require.True(t, rr.Match(6))
	require.False(t, rr.Match(1))
	require.False(t, rr.Match(7))
	require.False(t, rr.Match(4))
	require.False(t, rr.Match(-1))

	_, err = newBucketRule("")
	require.Error(t, err)
	_, err = newBucketRule("123:")
	require.Error(t, err)
	_, err = newBucketRule(":123")
	require.Error(t, err)
	_, err = newBucketRule("a:123")
	require.Error(t, err)
	_, err = newBucketRule("123:a")
	require.Error(t, err)
}

// TestBucketStats_Buckets tests that the buckets are correctly
// created and that the measures are correctly splitted.
func TestBucketStats_Buckets(t *testing.T) {
	b := newBucketStats()

	err := b.Set(0, []string{"abc"}, NewStats(nil))
	require.Error(t, err)

	err = b.Set(0, []string{"10:20"}, NewStats(nil))
	require.NoError(t, err)

	err = b.Set(1, []string{"15:20"}, NewStats(nil))
	require.NoError(t, err)

	err = b.Set(2, []string{"5:10", "20:25"}, NewStats(nil))
	require.NoError(t, err)

	b.Update(newSingleMeasureWithHost("a", 5, 17))
	b.Update(newSingleMeasureWithHost("a", 10, 10))

	s := b.Get(0)
	require.Equal(t, 7.5, s.Value("a").Avg())

	s = b.Get(1)
	require.Equal(t, 5.0, s.Value("a").Avg())

	s = b.Get(2)
	require.Nil(t, s.Value("a"))

	s = b.Get(3)
	require.Nil(t, s)
}
