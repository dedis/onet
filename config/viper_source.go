package config

import "github.com/spf13/viper"

// ViperSource implements the Source interface using the viper package for
// configuration file.
type ViperSource struct {
	v *viper.Viper
}

// NewViperSource returns a new ViperSource from the top level Viper object,i.e.
// it calls viper.GetViper(). The caller must configure the viper package to
// give the path and names of the config files to search for. It can be done
// with:
//
//    viper.SetConfig("name")
//    viper.SetConfigPath(".")
//
func NewViperSource() *ViperSource {
	return &ViperSource{viper.GetViper()}
}

// Defined returns true if the key is defined in the configuration file
func (v *ViperSource) Defined(key string) bool {
	return v.v.IsSet(key)
}

// Sub returns a viper source which has a tighter scope
func (v *ViperSource) Sub(key string) Source {
	return &ViperSource{v.v.Sub(key)}
}

// String returns the given value under this key
func (v *ViperSource) String(key string) string {
	return v.v.GetString(key)
}
