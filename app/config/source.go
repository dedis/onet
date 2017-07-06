package config

import (
	"strconv"
	"time"
)

// Source represents any object that can retrieve any configuration string from
// a given key.  Configuration strings are represented using a simple dot
// notation. For example, given a TOML configuration file such as:
//
//   [server]
//   ip = "192.168.0.1"
//
//   [cosi]
//   rate = "20s"
//
// One can retrieve the ip of the server using "server.ip" as the key.
// Any source should be able to say of the given key is defined or not.
// Finally, any source should be able to restrict its scope by calling "Sub"
// with a given key, effectively reducing the number of prefixes of a key by one.
// Using the example above, with a general Source s for this TOML file, one can
// restrict the keys searchable for cosi by doing:
//
//   cosi := s.Sub("cosi")
//   rate := cosi.String("rate")
//   cosi.Defined("server.ip") // returns false
//
type Source interface {
	// Defined returns true if the given key is defined in that Source
	Defined(key string) bool
	// String returns the string representation of the value stored under the
	// given key
	String(key string) string
	// Sub returns a new Source with a reduced scope
	Sub(key string) Source
}

// MultiSource is a Source that aggregates multiple sources together. All
// methods are searching in linear order in all the Sources that this
// MultiSource manages.
type MultiSource struct {
	sources []Source
}

// NewMultiSource returns a Source that is searching through all given Source
// for a key. The ORDER in which the sources are given is very important as it
// determines the priority amongst them. In short, the caller must put the
// higher priority Sources first. If the first Source has the key
// defined, MultiSource will use this key. If not, then MultiSource checks the
// second Source, etc. Typically, the caller should put the command line source
// first and then the configuration file source.
func NewMultiSource(sources ...Source) *MultiSource {
	return &MultiSource{
		sources: sources,
	}
}

// Sub creates a new MultiSource by reducing the scope of all its inner Sources.
func (c *MultiSource) Sub(key string) Source {
	var s2 = make([]Source, len(c.sources))
	for i, s := range c.sources {
		s2[i] = s.Sub(key)
	}
	return NewMultiSource(s2...)
}

// String searches in linear order for the first Source that has this key
// defined.  If it finds one, it returns the value under that key, otherwise it
// returns the empty string.
func (c *MultiSource) String(key string) string {
	for _, s := range c.sources {
		if s.Defined(key) {
			return s.String(key)
		}
	}
	return ""
}

// Defined searches in linear order for the first Source that has this key
// defined, otherwise it returns false.
func (c *MultiSource) Defined(key string) bool {
	for _, s := range c.sources {
		if s.Defined(key) {
			return true
		}
	}
	return false
}

// TypedSource adds additional functionality to a Source. Namely, it provides
// wrapper function to cast any value to a certain type such as int,
// time.Duration, etc. If the key is not found, or an error happened during the
// conversion, then the default value of the type is returned.
type TypedSource struct {
	Source
}

// NewTypedSource returns a TypedSource wrapped around the given Source.
func NewTypedSource(s Source) *TypedSource {
	return &TypedSource{s}
}

// Duration returns the value under the key as a time.Duration.
func (t TypedSource) Duration(key string) time.Duration {
	s := t.String(key)
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0 * time.Second
	}
	return d
}

// Int returns the value under the key as a integer.
func (t TypedSource) Int(key string) int {
	i, _ := strconv.Atoi(t.String(key))
	return i
}
