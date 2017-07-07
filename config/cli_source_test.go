package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

func TestCLISource(t *testing.T) {
	gTest := "gtest"
	lTest := "ltest"
	gTestKey := "bump"
	gTestVal := "itup"
	cmdName := "covfefe"
	lTestKey := "crypto"
	lTestVal := "rules"

	unknownKey := "brahms"

	lSub := "sub"
	lSubKey := "config"
	lSubTotal := lSub + "." + lSubKey
	lSubVal := "totallynonimportantvalue"

	app := cli.NewApp()
	app.Flags = []cli.Flag{cli.StringFlag{Name: gTest}, cli.GenericFlag{Name: GenericFlagName, Value: &genericFlag{}}}
	app.Action = func(c *cli.Context) error {
		s := NewCliSource(c)
		require.True(t, s.Defined(gTest))
		require.Equal(t, gTest, s.String(gTest))
		require.True(t, s.Defined(gTestKey))
		require.Equal(t, gTestVal, s.String(gTestKey))
		return nil
	}
	local := cli.Command{
		Name:  cmdName,
		Flags: []cli.Flag{cli.StringFlag{Name: lTest}, GenericCliFlag},
		Action: func(c *cli.Context) error {
			s := NewCliSource(c)
			require.True(t, s.Defined(lTest))
			require.Equal(t, lTest, s.String(lTest))

			require.True(t, s.Defined(lTestKey))
			require.Equal(t, lTestVal, s.String(lTestKey))

			sub := s.Sub(lSub)
			require.Equal(t, lSubVal, sub.String(lSubKey))
			require.Equal(t, "", sub.String(unknownKey))

			return nil
		},
	}
	noGeneric := cli.Command{
		Name: cmdName + "no",
		Action: func(c *cli.Context) error {
			s := NewCliSource(c)
			require.False(t, s.Defined(unknownKey))
			require.Equal(t, "", s.String(unknownKey))
			return nil
		},
	}
	app.Commands = []cli.Command{local, noGeneric}

	// try global arguments
	args := []string{"app", f(gTest), gTest, f(GenericFlagName), gTestKey + "=" + gTestVal}
	app.Run(args)

	// try local arguments + unknown key + sub key
	args = []string{"app", cmdName, f(lTest), lTest,
		f(GenericFlagName), lTestKey + "=" + lTestVal,
		f(GenericFlagName), lSubTotal + "=" + lSubVal,
	}
	app.Run(args)

	args = []string{"app", cmdName + "no"}
	app.Run(args)

}

func TestGenericFlag(t *testing.T) {
	gf := &genericFlag{}
	good := 0
	for _, test := range []struct {
		ToSet  string
		Return error
		Key    string
		ToGet  string
	}{
		{"bob=alice", nil, "bob", "alice"},
		{"bob.alice=eve", nil, "bob.alice", "eve"},
		{"knocknock.alice:eve", ErrGenericFlagFormat, "knocknock.alice", ""},
		{"rabbit", ErrGenericFlagFormat, "rabbit", ""},
		{"===", ErrGenericFlagFormat, "=", ""},
	} {
		err := gf.Set(test.ToSet)
		require.Equal(t, test.Return, err)

		value, ok := gf.Get(test.Key)
		if err != nil {
			require.Equal(t, "", value)
			require.False(t, ok)
			continue
		}

		require.True(t, ok)
		require.Equal(t, test.ToGet, value)
		good += 1
	}

	require.Len(t, gf.pairs, good)
}

func f(flag string) string {
	return "--" + flag
}
