package onet

import (
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/onet/v3/ciphersuite"
	"go.dedis.ch/onet/v3/log"
	"golang.org/x/xerrors"
)

var simTestBuilder = NewDefaultBuilder()

func init() {
	simTestBuilder.SetSuite(testSuite)
	simTestBuilder.SetService("simulationTestService", testSuite, func(c *Context, suite ciphersuite.CipherSuite) (Service, error) {
		return nil, nil
	})
	simTestBuilder.SetService("simulationTestService2", testSuite, func(c *Context, suite ciphersuite.CipherSuite) (Service, error) {
		return nil, nil
	})
}

func TestSimulationBF(t *testing.T) {
	sc, _, err := createBFTree(7, 2, false, []string{"test1", "test2"})
	if err != nil {
		t.Fatal(err)
	}
	addresses := []string{
		"test1:2000",
		"test2:2000",
		"test1:2002",
		"test2:2002",
		"test1:2004",
		"test2:2004",
		"test1:2006",
	}

	for i, a := range sc.Roster.List {
		if !strings.Contains(string(a.Address), addresses[i]) {
			t.Fatal("Address", string(a.Address), "should be", addresses[i])
		}
	}
	if !sc.Tree.IsBinary(sc.Tree.Root) {
		t.Fatal("Created tree is not binary")
	}

	sc, _, err = createBFTree(13, 3, false, []string{"test1", "test2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(sc.Tree.Root.Children) != 3 {
		t.Fatal("Branching-factor 3 tree has not 3 children")
	}
	if !sc.Tree.IsNary(sc.Tree.Root, 3) {
		t.Fatal("Created tree is not binary")
	}
}

func TestSimulationBigTree(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	for i := uint(4); i < 8; i++ {
		_, _, err := createBFTree(1<<i-1, 2, false, []string{"test1", "test2"})
		require.Nil(t, err)
	}
}

func TestSimulationLoadSave(t *testing.T) {
	sc, _, err := createBFTree(7, 2, false, []string{"127.0.0.1", "127.0.0.2"})
	if err != nil {
		t.Fatal(err)
	}
	dir, err := ioutil.TempDir("", "example")
	log.ErrFatal(err)
	defer os.RemoveAll(dir)
	sc.Save(dir)
	sc2, err := LoadSimulationConfig(simTestBuilder, dir, sc.Roster.List[0].Address.NetworkAddress())
	if err != nil {
		t.Fatal(err)
	}
	if !sc2[0].Tree.ID.Equal(sc.Tree.ID) {
		t.Fatal("Tree-id is not correct")
	}

	for key, privKeys := range sc.PrivateKeys {
		require.Equal(t, privKeys.Private.String(), sc2[0].PrivateKeys[key].Private.String())
		require.Equal(t, 2, len(sc2[0].PrivateKeys[key].Services))
		require.Equal(t, privKeys.Services[0].String(), sc2[0].PrivateKeys[key].Services[0].String())
		require.Equal(t, privKeys.Services[1].String(), sc2[0].PrivateKeys[key].Services[1].String())
	}

	closeAll(sc2)
}

func TestSimulationMultipleInstances(t *testing.T) {
	sc, _, err := createBFTree(7, 2, false, []string{"127.0.0.1", "127.0.0.2"})
	if err != nil {
		t.Fatal(err)
	}
	dir, err := ioutil.TempDir("", "example")
	log.ErrFatal(err)
	defer os.RemoveAll(dir)
	sc.Save(dir)
	sc2, err := LoadSimulationConfig(simTestBuilder, dir, sc.Roster.List[0].Address.Host())
	if err != nil {
		t.Fatal(err)
	}
	defer closeAll(sc2)
	if len(sc2) != 4 {
		t.Fatal("We should have 4 local1-hosts but have", len(sc2))
	}
	if sc2[0].Server.ServerIdentity.ID.Equal(sc2[1].Server.ServerIdentity.ID) {
		t.Fatal("Hosts are not copies")
	}
}

func closeAll(scs []*SimulationConfig) {
	for _, s := range scs {
		if err := s.Server.Close(); err != nil {
			log.Error("Error closing host", s.Server.ServerIdentity, err)
		}

		for s.Server.Router.Listening() {
			log.Lvl2("Sleeping while waiting for router to be closed")
			time.Sleep(20 * time.Millisecond)
		}
	}
}

func createBFTree(hosts, bf int, tls bool, addresses []string) (*SimulationConfig, *SimulationBFTree, error) {
	sc := &SimulationConfig{Builder: simTestBuilder}
	sb := &SimulationBFTree{
		Hosts: hosts,
		BF:    bf,
		TLS:   tls,
	}
	sb.CreateRoster(sc, addresses, 2000)
	if len(sc.Roster.List) != hosts {
		return nil, nil, xerrors.New("Didn't get correct number of entities")
	}
	err := sb.CreateTree(sc)
	if err != nil {
		return nil, nil, err
	}
	if !sc.Tree.IsNary(sc.Tree.Root, bf) {
		return nil, nil, xerrors.New("Tree isn't " + strconv.Itoa(bf) + "-ary")
	}

	return sc, sb, nil
}
