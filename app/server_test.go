package app

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/onet/v4"
	"go.dedis.ch/onet/v4/log"
)

func TestInteractiveConfig(t *testing.T) {
	tmp, err := ioutil.TempDir("", "conode")
	log.ErrFatal(err)

	setInput("127.0.0.1:2000\nConode1\n" + tmp)
	InteractiveConfig(testBuilder, tmp+"/config.bin")

	builder := onet.NewDefaultBuilder()
	builder.SetSuite(testSuite)

	cc, _, err := ParseCothority(builder, tmp+"/private.toml")
	require.NoError(t, err)
	require.NotNil(t, cc.Services[testServiceName])
	require.Equal(t, cc.Description, "Conode1")
	require.Equal(t, "tls://127.0.0.1:2000", cc.Address.String())

	gFile, err := os.Open(tmp + "/public.toml")
	require.NoError(t, err)
	gc, err := ReadGroupDescToml(gFile)
	require.NoError(t, err)
	require.Equal(t, 1, len(gc.Roster.List))
	require.Equal(t, 1, len(gc.Roster.List[0].ServiceIdentities))

	log.ErrFatal(os.RemoveAll(tmp))
}
