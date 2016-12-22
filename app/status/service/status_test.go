package status

import (
	"testing"

	"github.com/dedis/onet"
	"github.com/dedis/onet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	log.MainTest(m)
}

func TestServiceStatus(t *testing.T) {
	local := onet.NewTCPTest()
	// generate 5 hosts
	_, el, _ := local.GenTree(5, false)
	defer local.CloseAll()

	// Send a request to the service
	client := NewClient()
	log.Lvl1("Sending request to service...")
	stat, cerr := client.Request(el.List[0])
	log.Lvl1(el.List[0])
	log.ErrFatal(cerr)
	log.Lvl1(stat)
	assert.NotEmpty(t, stat.Msg["Status"].Field["Available_Services"])
}

func TestServiceCount(t *testing.T) {
	local := onet.NewTCPTest()
	nbrNodes := 5
	_, el, _ := local.GenTree(nbrNodes, false)
	defer local.CloseAll()
	client := NewClient()

	children, cerr := client.Count(el, 1000)
	log.ErrFatal(cerr)
	require.Equal(t, nbrNodes, children)
}
