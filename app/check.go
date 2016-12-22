package app

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/dedis/onet"
	"github.com/dedis/onet/app/status/service"
	"github.com/dedis/onet/log"
	"github.com/dedis/onet/network"
	"gopkg.in/urfave/cli.v1"
)

// RequestTimeOut is how long we're willing to wait for a signature.
var RequestTimeOut = time.Second * 10

// CheckConfigCLI reads the group-file and contacts all servers and verifies if
// it receives a valid signature from each.
func CheckConfigCLI(c *cli.Context) error {
	tomlFileName := c.String("g")
	if tomlFileName == "" {
		tomlFileName = c.Args().First()
	}
	if _, err := os.Stat(tomlFileName); err != nil {
		log.Fatal(err, "while trying to read group-toml")
	}
	return CheckConfig(tomlFileName, c.Bool("d"))
}

// CheckConfig contacts all servers and verifies if it receives a valid
// signature from each.
// If the roster is empty it will return an error.
// If a server doesn't reply in time, it will return an error.
func CheckConfig(tomlFileName string, detail bool) error {
	f, err := os.Open(tomlFileName)
	log.ErrFatal(err, "Couldn't open group definition file")
	group, err := ReadGroupDescToml(f)
	log.ErrFatal(err, "Error while reading group definition file", err)
	if len(group.Roster.List) == 0 {
		log.ErrFatalf(err, "Empty entity or invalid group defintion in: %s",
			tomlFileName)
	}
	log.Info("Checking the availability and responsiveness of the servers in the group...")
	return Servers(group, detail)
}

// Servers contacts all servers in the entity-list and then makes checks
// on each pair. If server-descriptions are available, it will print them
// along with the IP-address of the server.
// In case a server doesn't reply in time or there is an error in the
// signature, an error is returned.
func Servers(g *Group, detail bool) error {
	totalSuccess := true
	// First check all servers individually and write the working servers
	// in a list
	working := []*network.ServerIdentity{}
	for _, e := range g.Roster.List {
		desc := []string{"none", "none"}
		if d := g.GetDescription(e); d != "" {
			desc = []string{d, d}
		}
		el := onet.NewRoster([]*network.ServerIdentity{e})
		err := checkList(el, desc, true)
		if err == nil {
			working = append(working, e)
		} else {
			totalSuccess = false
		}
	}
	wn := len(working)
	if wn > 1 {
		// Check one big roster sqrt(len(working)) times.
		descriptions := make([]string, wn)
		rand.Seed(int64(time.Now().Nanosecond()))
		for j := 0; j <= int(math.Sqrt(float64(wn))); j++ {
			permutation := rand.Perm(wn)
			for i, si := range working {
				descriptions[permutation[i]] = g.GetDescription(si)
			}
			totalSuccess = checkList(onet.NewRoster(working), descriptions, detail) == nil && totalSuccess
		}

		// Then check pairs of servers if we want to have detail
		if detail {
			for i, first := range working {
				for _, second := range working[i+1:] {
					log.Lvl3("Testing connection between", first, second)
					desc := []string{"none", "none"}
					if d1 := g.GetDescription(first); d1 != "" {
						desc = []string{d1, g.GetDescription(second)}
					}
					es := []*network.ServerIdentity{first, second}
					totalSuccess = checkList(onet.NewRoster(es), desc, detail) == nil && totalSuccess
					es[0], es[1] = es[1], es[0]
					desc[0], desc[1] = desc[1], desc[0]
					totalSuccess = checkList(onet.NewRoster(es), desc, detail) == nil && totalSuccess
				}
			}
		}
	}
	if !totalSuccess {
		return errors.New("At least one of the tests failed")
	}
	return nil
}

// checkList counts the nodes in the cothority defined by list and
// waits for the reply.
// If the reply doesn't arrive in time, it will return an
// error.
func checkList(list *onet.Roster, descs []string, detail bool) error {
	serverStr := ""
	for i, s := range list.List {
		name := strings.Split(descs[i], " ")[0]
		if detail {
			serverStr += s.Address.NetworkAddress() + "_"
		}
		serverStr += name + " "
	}
	log.Lvl3("Sending message to: " + serverStr)

	fmt.Printf("Checking %d server(s) %s: ", len(list.List), serverStr)
	client := status.NewClient()
	children, cerr := client.Count(list, 10000)
	if children != len(list.List) {
		cerr = onet.NewClientError(fmt.Errorf("got only %d/%d children", children, len(list.List)))
	}
	if cerr == nil {
		fmt.Println("Success")
	} else {
		fmt.Println("Failed:", cerr.Error())
	}
	return cerr
}
