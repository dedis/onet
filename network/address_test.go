package network

import (
	"testing"

	"github.com/dedis/onet/log"
	"github.com/stretchr/testify/assert"
)

func TestConnType(t *testing.T) {
	var tests = []struct {
		Value    string
		Expected ConnType
	}{
		{"tcp", PlainTCP},
		{"tls", TLS},
		{"purb", PURB},
		{"tcp4", InvalidConnType},
		{"_tls", InvalidConnType},
	}

	for _, str := range tests {
		if connType(str.Value) != str.Expected {
			t.Error("Wrong ConnType for " + str.Value)
		}
	}
}

var staticHostIPMapping = make(map[string]string)

func dummyResolver(s string) ([]string, error) {
	return []string{staticHostIPMapping[s]}, nil
}

func TestAddress(t *testing.T) {
	lookupHost = dummyResolver
	staticHostIPMapping["google.com"] = "8.8.8.8"
	staticHostIPMapping["facebook.com"] = "20.20.20.20"
	staticHostIPMapping["epfl.ch"] = "100.100.100.100"
	staticHostIPMapping["localhost"] = "127.0.0.1"
	var tests = []struct {
		Value   string
		Valid   bool
		Type    ConnType
		Address string
		Host    string
		Port    string
		Public  bool
	}{
		{"tls://10.0.0.4:2000", true, TLS, "10.0.0.4:2000", "10.0.0.4", "2000", false},
		{"tcp://10.0.0.4:2000", true, PlainTCP, "10.0.0.4:2000", "10.0.0.4", "2000", false},
		{"tcp://67.43.129.85:2000", true, PlainTCP, "67.43.129.85:2000", "67.43.129.85", "2000", true},
		{"purb://10.0.0.4:2000", true, PURB, "10.0.0.4:2000", "10.0.0.4", "2000", false},
		{"tls://[::]:1000", true, TLS, "[::]:1000", "[::]", "1000", false},
		{"tls4://10.0.0.4:2000", false, InvalidConnType, "", "", "", false},
		{"tls://1000.0.0.4:2000", false, InvalidConnType, "", "", "", false},
		{"tls://10.0.0.4:20000000", false, InvalidConnType, "", "", "", false},
		{"tls://10.0.0.4:-10", false, InvalidConnType, "", "", "", false},
		{"tlsx10.0.0.4:2000", false, InvalidConnType, "", "", "", false},
		{"tls:10.0.0.4x2000", false, InvalidConnType, "", "", "", false},
		{"tlsx10.0.0.4x2000", false, InvalidConnType, "", "", "", false},
		{"tlxblurdie", false, InvalidConnType, "", "", "", false},
		{"tls://blublublu", false, InvalidConnType, "", "", "", false},

		// dummy values for the IP addresses, defined by dummyResolver
		{"tcp://localhost:80", true, PlainTCP, "127.0.0.1:80", "127.0.0.1", "80", false},
		{"tcp://facebook.com:8080", true, PlainTCP, "20.20.20.20:8080", "20.20.20.20", "8080", true},
		{"tls://google.com:80", true, TLS, "8.8.8.8:80", "8.8.8.8", "80", true},
		{"tcp://epfl.ch:8080", true, PlainTCP, "100.100.100.100:8080", "100.100.100.100", "8080", true},

	}

	for i, str := range tests {
		log.Lvl1("Testing", str)
		add := Address(str.Value)
		assert.Equal(t, str.Valid, add.Valid(), "Address (%d) %s", i, str.Value)
		assert.Equal(t, str.Type, add.ConnType(), "Address (%d) %s", i, str.Value)
		assert.Equal(t, str.Address, add.NetworkAddress())
		assert.Equal(t, str.Host, add.Host())
		assert.Equal(t, str.Port, add.Port())
		assert.Equal(t, str.Public, add.Public())
	}
}

// Isolated test case for validHostname
func TestDNSNames(t *testing.T) {
	assert.True(t, validHostname("myhost.secondlabel.org"))
	assert.True(t, validHostname("www.asd.lol.xd"))
	assert.True(t, validHostname("a.a"))
	assert.True(t, validHostname("localhost"))
	assert.True(t, validHostname("www.asd.lol.xd"))
	assert.True(t, validHostname("randomtext"))

	assert.False(t, validHostname("www.asd.lol.x-d"))
	assert.False(t, validHostname("192.168.1.1"))
	assert.False(t, validHostname("..a"))
	assert.False(t, validHostname("a..a"))
	assert.False(t, validHostname("123213.213")) // look into this again
	assert.False(t, validHostname("-23.dwe"))
	assert.False(t, validHostname("..."))
	assert.False(t, validHostname("www.asd.lol.xd-"))
}
