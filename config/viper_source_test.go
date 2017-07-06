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
rate = "10s"

`)

	viper.ReadConfig(bytes.NewBuffer(tomlExample))
	require.Equal(t, cosiRate, viper.Get("cosi.rate"))

	s := NewViperSource()
	require.True(t, s.Defined("cosi.rate"))
	require.Equal(t, cosiRate, s.String("cosi.rate"))

	cosi := s.Sub("cosi")
	require.True(t, cosi.Defined("rate"))
	require.Equal(t, cosiRate, cosi.String("rate"))

	require.False(t, cosi.Defined("server.ip"))
	require.Empty(t, cosi.String("server.ip"))
}
