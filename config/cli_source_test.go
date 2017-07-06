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
			return nil
		},
	}
	app.Commands = []cli.Command{local}

	// try global arguments
	args := []string{"app", f(gTest), gTest, f(GenericFlagName), gTestKey + "=" + gTestVal}
	app.Run(args)

	// try local arguments
	args = []string{"app", cmdName, f(lTest), lTest, f(GenericFlagName), lTestKey + "=" + lTestVal}
	app.Run(args)
}

func f(flag string) string {
	return "--" + flag
}
