package config

import (
	"errors"
	"strings"

	"github.com/urfave/cli"
)

// CliSource is an implementation of a Source that reads the key / value pairs
// from the command line arguments using urfave/cli framework.
//
// In urface/cli, there is two ways to define a flag: either define global flags
// on the `app` or define flags on commands. Flags are then retrieved using the
// `cli.Context` structure. In the former approach, one can retrieve the flag
// using `context.GlobalString(key)`. In the latter approach, one can retrieve
// the flag using `context.String(key)`.
//
// Finally, CliSource implements a feature to be able to define any key/value
// pair on the command line. To use this feature, you need to register a
// GenericFlag to the `cli.App` or `cli.Command` such as
//
//    Flags: []cli.Flag{Name: "generic", Value: &config.GenericFlag{}}
//
// This way, one can write any key/pair on the command line using the following
// format:
//
//   ./MyApp --generic "globalKey=value" <command> --generic "key=value"
//
// The CliSource first check the global flags, then the local flags, then the
// generic flags in that order.
//
// WARNING: If you set a default value to any flag, then this default value will
// always be returned first. For example, suppose a default value has been set on a
// global flag. Even if a user sets a generic flag with the same key, CliSource
// always returns the global default value set. This is because the urfave/cli
// treats a default value as a defined value. An issue has been reported at
// https://github.com/urfave/cli/issues/642.
type CliSource struct {
	namespace string
	c         *cli.Context
}

// NewCliSource returns a new CliSource out of the given cli.Context. Note that
// the cli.Context must be the one from the actual command which is ran,
// otherwise only the global flags will be detected.
func NewCliSource(c *cli.Context) Source {
	return &CliSource{"", c}
}

// Defined checks first is the key is defined in the global flags, then in the
// "local" flags, and finally checks if a generic flag has been used.
func (c *CliSource) Defined(key string) bool {
	_, ok := c.value(key)
	return ok
}

/// String checks first is the key is defined in the global flags, then in the
// "local" flags, and finally checks if a generic flag has been used.
func (c *CliSource) String(key string) string {
	s, _ := c.value(key)
	return s
}

// Sub returns a new CliSource with a restricted scope
func (c *CliSource) Sub(key string) Source {
	return &CliSource{
		namespace: c.fullKey(key),
		c:         c.c,
	}
}

func (c *CliSource) fullKey(key string) string {
	if c.namespace != "" {
		return c.namespace + "." + key
	}
	return key
}

func (c *CliSource) value(key string) (string, bool) {
	if c.c.GlobalIsSet(key) {
		return c.c.GlobalString(key), true
	}
	if c.c.IsSet(key) {
		return c.c.String(key), true
	}
	var i interface{}
	if c.c.GlobalIsSet("generic") {
		i = c.c.GlobalGeneric("generic")
	} else if c.c.IsSet("generic") {
		i = c.c.Generic("generic")
	} else {
		return "", false
	}

	g := i.(*genericFlag)
	// namespace is automatically provided if any, and namespace is
	// automatically saved in genericFlag
	str, ok := g.Get(c.fullKey(key))
	return str, ok
}

// GenericFlagName is the name given to the flag of the command line option
var GenericFlagName = "generic"

// GenericCliFlag is a wrapper around a cli.GenericFlag that provides the
// generic tag capability
var GenericCliFlag = cli.GenericFlag{Name: GenericFlagName, Value: &genericFlag{}}

// genericFlags holds all value of the form "key=value"
type genericFlag struct {
	pairs []*pair
}

type pair struct {
	root  string
	value string
}

// ErrGenericFlagFormat is triggered when a flag don't have the right format.
var ErrGenericFlagFormat = errors.New("generic flag format: `key=value`")

// Set implements the generic flag interface from urfave/cli
func (g *genericFlag) Set(value string) error {
	strs := strings.Split(value, "=")
	if len(strs) != 2 {
		return ErrGenericFlagFormat
	}
	p := &pair{strs[0], strs[1]}
	g.pairs = append(g.pairs, p)
	return nil
}

// String implements the generic flag interface from urfave/cli
func (g *genericFlag) String() string {
	return "generic flag setting: --" + GenericFlagName + " \"<key>=<value>\""
}

// Get returns the value stored under the given key if a generic flag has been
// defined with this key.
func (g *genericFlag) Get(key string) (string, bool) {
	for _, p := range g.pairs {
		if p.root == key {
			return p.value, true
		}
	}
	return "", false
}
