package config

import (
	"bytes"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestViperSource(t *testing.T) {
	viper.SetConfigType("TOML")
	cosiRate := "10s"

	var tomlExample = []byte(`
[server]
ip = "0.0.0.0"

[cosi]
period = "10s"

`)

	viper.ReadConfig(bytes.NewBuffer(tomlExample))
	require.Equal(t, cosiRate, viper.Get("cosi.period"))

	s := NewViperSource()
	require.True(t, s.Defined("cosi.period"))
	require.Equal(t, cosiRate, s.String("cosi.period"))

	cosi := s.Sub("cosi")
	require.True(t, cosi.Defined("period"))
	require.Equal(t, cosiRate, cosi.String("period"))

	require.False(t, cosi.Defined("server.ip"))
	require.Empty(t, cosi.String("server.ip"))
}
