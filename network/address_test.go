package network

import (
	"testing"

	"github.com/dedis/onet/log"
	"github.com/stretchr/testify/assert"
	"net"
	"sort"
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

func TestAddress(t *testing.T) {
	localhostNetworkAddresses, _ := net.LookupHost("localhost")
	sort.Strings(localhostNetworkAddresses)
	facebookNetworkAddresses, _ := net.LookupHost("facebook.com")
	sort.Strings(facebookNetworkAddresses)
	googleNetworkAddresses, _ := net.LookupHost("google.com")
	sort.Strings(googleNetworkAddresses)
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

		// In order to make these three tests work, I need to put the IP address of the hosts
		// in the "Address" and "Host" parts. The fact that the exact
		// IP address that is returned for each hostname is not deterministic (not always
		// the same IP address is returned) is handled by ordering the IP addresses
		// returned by LookupHost and by taking the first one
		{"tcp://localhost:80", true, PlainTCP, net.JoinHostPort(localhostNetworkAddresses[0], "80"), localhostNetworkAddresses[0], "80", false},
		{"tcp://facebook.com:8080", true, PlainTCP, net.JoinHostPort(facebookNetworkAddresses[0], "8080"), facebookNetworkAddresses[0], "8080", true},
		{"tls://google.com:80", true, TLS, net.JoinHostPort(googleNetworkAddresses[0], "80"), googleNetworkAddresses[0], "80", true},
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

// just a temporary test case
func TestDNSNames(t *testing.T) {
	assert.True(t, validHostname("myhost.secondlabel.org"), "valid")
	assert.True(t, validHostname("www.asd.lol.xd"), "valid")
	assert.True(t, validHostname("a.a"), "valid")
	assert.True(t, validHostname("localhost"), "valid")
	assert.True(t, validHostname("www.asd.lol.xd"), "valid")
	assert.True(t, validHostname("randomtext"), "valid")

	assert.False(t, validHostname("www.asd.lol.x-d"), "not valid")
	assert.False(t, validHostname("192.168.1.1"), "not valid")
	assert.False(t, validHostname("..a"), "not valid")
	assert.False(t, validHostname("a..a"), "not valid")
	assert.False(t, validHostname("123213.213"), "not valid") // look into this again
	assert.False(t, validHostname("-23.dwe"), "not valid")
	assert.False(t, validHostname("..."), "not valid")
	assert.False(t, validHostname("www.asd.lol.xd-"), "not valid")
}
