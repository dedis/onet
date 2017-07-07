package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type mapSource struct {
	m         map[string]string
	namespace string
}

func newMapSource() *mapSource {
	return &mapSource{m: make(map[string]string)}
}

func (m *mapSource) Add(key, value string) {
	m.m[key] = value
}

func (m *mapSource) Defined(key string) bool {
	full := m.fullKey(key)
	for k := range m.m {
		if k == full {
			return true
		}
	}
	return false
}

func (m *mapSource) String(key string) string {
	full := m.fullKey(key)
	for k, v := range m.m {
		if k == full {
			return v
		}
	}
	return ""
}

func (m *mapSource) Sub(key string) Source {
	return &mapSource{m: m.m, namespace: m.fullKey(key)}
}

func (m *mapSource) fullKey(k string) string {
	if m.namespace != "" {
		return m.namespace + "." + k
	}
	return k
}

func TestMultiSource(t *testing.T) {
	s1 := newMapSource()
	s2 := newMapSource()

	// Test ordering
	s1.Add("bob", "alice")
	s2.Add("bob", "eve")
	// one way
	ms := NewMultiSource(s1, s2)
	require.True(t, ms.Defined("bob"))
	require.Equal(t, "alice", ms.String("bob"))
	// the other way around
	ms = NewMultiSource(s2, s1)
	require.Equal(t, "eve", ms.String("bob"))

	// test not defined / empty string
	require.False(t, ms.Defined("unknown"))
	require.Empty(t, ms.String("unknown"))

	// test sub
	s1.Add("one.two", "three")
	require.Equal(t, "three", ms.String("one.two"))

	mss := ms.Sub("one")
	require.Equal(t, "three", mss.String("two"))
}

func TestTypedSource(t *testing.T) {
	s := newMapSource()
	ts := NewTypedSource(s)

	// test sub
	s.Add("bob.alice", "10")
	tss := ts.Sub("bob")
	require.Equal(t, 10, tss.Int("alice"))
	require.Equal(t, 0, tss.Int("unknown"))

	// test int
	s.Add("int", "1")
	require.Equal(t, 1, ts.Int("int"))
	s.Add("wrongInt", "hello")
	require.Equal(t, 0, ts.Int("wrongInt"))

	// test duration
	s.Add("time", "10s")
	require.Equal(t, 10*time.Second, ts.Duration("time"))
	s.Add("wrongTime", "10s67minuteswhatever")
	require.Equal(t, 0*time.Second, ts.Duration("wrongTime"))

}
