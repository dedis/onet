// Status takes in a file containing a list of servers and returns the status reports of all of the servers.
// A status is a list of connections and packets sent and received for each server in the file.
package main

import (
	"os"

	"github.com/dedis/onet/log"

	"sort"
	"strings"

	"github.com/dedis/onet/app"
	"github.com/dedis/onet/app/status/service"
	"gopkg.in/urfave/cli.v1"
)

func main() {
	cliApp := cli.NewApp()
	cliApp.Name = "Status"
	cliApp.Usage = "Get and print status of all servers of a file."
	cliApp.Commands = []cli.Command{
		app.CmdCheck,
		{
			Name:      "status",
			Aliases:   []string{"s"},
			Usage:     "print all status-messages from the group",
			ArgsUsage: "group-file",
			Action:    getStatus,
		},
	}
	cliApp.Flags = []cli.Flag{app.FlagDebug}
	cliApp.Before = func(c *cli.Context) error {
		log.SetDebugVisible(c.Int("d"))
		return nil
	}
	cliApp.Action = func(c *cli.Context) error {
		return getStatus(c)
	}
	cliApp.Run(os.Args)
}

// getStatus will contact all cothorities in the group-file and print
// the status-report of each one.
func getStatus(c *cli.Context) error {
	if c.NArg() == 0 {
		log.Fatal("Please give a group-file")
	}
	gtFile, err := os.Open(c.Args().First())
	log.ErrFatal(err)
	groupToml, err := app.ReadGroupDescToml(gtFile)
	log.ErrFatal(err)
	cl := status.NewClient()
	for _, si := range groupToml.Roster.List {
		log.Lvl3("Contacting", si)
		sr, _ := cl.Request(si)
		var a []string
		for key, value := range sr.Msg["Status"].Field {
			a = append(a, (key + ": " + value + "\n"))
		}
		sort.Strings(a)
		strings.Join(a, "\n")
		log.Print(a)
	}
	return nil
}
