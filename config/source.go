package config

import (
	"strconv"
	"time"
)

// Source represents any object that can retrieve a configuration string from
// a given key. Configuration strings are represented using a simple dot
// notation. For example, given a TOML configuration file such as:
//
//   [server]
//   ip = "192.168.0.1"
//
//   [cosi]
//   period = "20s"
//
// One can retrieve the ip of the server using "server.ip" as the key.
// Any source should be able to indicate if the given key is defined or not.
// Finally, any source should be able to restrict its scope by calling "Sub"
// with a given key, effectively reducing the number of prefixes of a key by one.
// Using the example above, with a general Source s for this TOML file, one can
// restrict the keys searchable for cosi by doing:
//
//   cosi := s.Sub("cosi")
//   period := cosi.String("period")
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

// SourceHub is a meta Source that aggregates a bunch of Sources together and
// enables to get typed configuration items out of those Sources.
//
// SourceHub can search through all its Sources for a key. The ORDER in which
// the sources are given is very important as it determines the priority amongst
// them. In short, the caller must give the highest priority Sources first when
// constructing the SourceHub. If the
// first Source has the key defined, SourceHub will use this key. If not, then
// SourceHub checks the second Source, etc. Typically, the caller should put
// the command line source first and then the configuration file source.
//
// On top of this aggregation mechanism, SourceHub offers two additional
// functionalities. Namely, it provides wrapper functions to cast any
// value to a certain type such as int, time.Duration, etc. If the key is not
// found, or an error happened during the conversion, then the default value of
// the type is returned.  SourceHub also provides the ability to give a default
// value to return in case the key is not defined in the Source. This is useful
// to provide a quick way of retrieving a value if we already know the default
// value:
//
//   source.StringOrDefault("ip","0.0.0.0")
//
type SourceHub struct {
	sources []Source
}

// NewSourceHub returns a SourceHub wrapped around the given Sources in this
// specific order.
func NewSourceHub(sources ...Source) *SourceHub {
	return &SourceHub{sources}
}

// Sub creates a new SourceHub by reducing the scope of all its inner Sources.
// It returns a Source to fulfill the Source interface. One can use
// `SubSourceHub` to get a *SourceHub return value.
func (sh *SourceHub) Sub(key string) Source {
	var s2 = make([]Source, len(sh.sources))
	for i, s := range sh.sources {
		s2[i] = s.Sub(key)
	}
	return NewSourceHub(s2...)
}

// SubSourceHub behaves the same as Sub but returns a *SourceHub value instead.
func (sh *SourceHub) SubSourceHub(key string) *SourceHub {
	return sh.Sub(key).(*SourceHub)
}

// String searches in linear order for the first Source that has this key
// defined.  If it finds one, it returns the value under that key, otherwise it
// returns the empty string.
func (sh *SourceHub) String(key string) string {
	for _, s := range sh.sources {
		if s.Defined(key) {
			return s.String(key)
		}
	}
	return ""
}

// Defined searches in linear order for the first Source that has this key
// defined, otherwise it returns false.
func (sh *SourceHub) Defined(key string) bool {
	for _, s := range sh.sources {
		if s.Defined(key) {
			return true
		}
	}
	return false
}

// Duration returns the value under the key as a time.Duration.
func (t *SourceHub) Duration(key string) time.Duration {
	d, err := t.duration(key)
	if err != nil {
		return 0 * time.Second
	}
	return d
}

// DurationOrDefault returns the value under the key as a time.Duration or the
// default value returned if the key is not defined in the Source.
func (t *SourceHub) DurationOrDefault(key string, def time.Duration) time.Duration {
	d, err := t.duration(key)
	if err != nil {
		return def
	}
	return d
}

func (t *SourceHub) duration(key string) (time.Duration, error) {
	s := t.String(key)
	return time.ParseDuration(s)
}

// Int returns the value under the key as a integer.
func (t *SourceHub) Int(key string) int {
	i, _ := strconv.Atoi(t.String(key))
	return i
}

// IntOrDefault returns the value under the key as an integer, or the default
// value given if the key is not defined in the source.
func (t *SourceHub) IntOrDefault(key string, def int) int {
	i, err := strconv.Atoi(t.String(key))
	if err != nil {
		return def
	}
	return i
}

// StringOrDefault returns the value under the given key if defined, otherwise
// it returns the default string
func (t *SourceHub) StringOrDefault(key, def string) string {
	if !t.Defined(key) {
		return def
	}
	return t.String(key)
}
