package app

import (
	"os"

	"github.com/dedis/onet/log"
	"gopkg.in/urfave/cli.v1"
)

// DefaultConfig is the name of the binary we produce and is used to create a directory
// folder with this name
const DefaultConfig = "cothorityd"

// CmdSetup is used to setup the cothority
var CmdSetup = cli.Command{
	Name:    "setup",
	Aliases: []string{"s"},
	Usage:   "Setup the configuration for the server (interactive)",
	Action: func(c *cli.Context) error {
		InteractiveConfig("cothorityd")
		return nil
	},
}

// CmdServer is used to start the server
var CmdServer = cli.Command{
	Name:  "server",
	Usage: "Run the cothority server",
	Action: func(c *cli.Context) {
		runServer(c)
	},
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "config, c",
			Value: GetDefaultConfigFile(DefaultConfig),
			Usage: "Configuration file of the server",
		},
	},
}

// CmdCheck is used to check all nodes in the group-file
var CmdCheck = cli.Command{
	Name:    "check",
	Aliases: []string{"c"},
	Usage:   "Check if the servers in the group definition are up and running",
	Action:  CheckConfigCLI,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "g",
			Usage: "Cothority group definition file",
		},
		cli.BoolFlag{
			Name:  "detail, d",
			Usage: "give more detail in searching for errors",
		},
	},
}

// FlagDebug offers a debug-flag
var FlagDebug = cli.IntFlag{
	Name:  "debug, d",
	Value: 0,
	Usage: "debug-level: 1 for terse, 5 for maximal",
}

// FlagConfig indicates where the configuration-file is stored
var FlagConfig = cli.StringFlag{
	Name:  "config, c",
	Value: GetDefaultConfigFile(DefaultConfig),
	Usage: "Configuration file of the server",
}

// Cothority creates a stand-alone cothority-binary
func Cothority() {
	cliApp := cli.NewApp()
	cliApp.Name = "Cothorityd server"
	cliApp.Usage = "Serve a cothority"

	cliApp.Commands = []cli.Command{
		CmdSetup,
		CmdServer,
		CmdCheck,
	}
	cliApp.Flags = []cli.Flag{
		FlagDebug,
		FlagConfig,
	}

	cliApp.Before = func(c *cli.Context) error {
		log.SetDebugVisible(c.Int("d"))
		return nil
	}

	// default action
	cliApp.Action = func(c *cli.Context) error {
		runServer(c)
		return nil
	}

	err := cliApp.Run(os.Args)
	log.ErrFatal(err)
}

// RunServer starts the server
func runServer(ctx *cli.Context) {
	// first check the options
	config := ctx.String("config")
	RunServer(config)
}
