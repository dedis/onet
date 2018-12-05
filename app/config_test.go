package app

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/dedis/kyber/pairing"
	"github.com/dedis/onet"
	"github.com/dedis/onet/log"
	"github.com/dedis/onet/network"
	"github.com/stretchr/testify/require"
)

var o bytes.Buffer

func init() {
	onet.RegisterNewServiceWithSuite("skipchain", pairing.NewSuiteBn256(), func(c *onet.Context) (onet.Service, error) {
		return nil, nil
	})
}

func TestMain(m *testing.M) {
	out = &o
	log.MainTest(m)
}

var serverGroup = `Description = "Default Dedis Cothority"

[[servers]]
  Address = "tcp://5.135.161.91:2000"
  Public = "94b8255379e11df5167b8a7ae3b85f7e7eb5f13894abee85bd31b3270f1e4c65"
  [services]
  	[services.skipchain]
	public = "017f84a03a7833d74820be7cc3d8ad7adc29bf3af7025fd24176f5dc5b451ec23c8dc82bbf856f10b422bc14e840222c2a91e1537372ab218b6f4f5d69e8f21d302f814a6d03b740124c7e6249960a770af381ed82d8aa8dbed961d6aef49779db06e4726c4de6d6d81e0e6431d3814779b9f009f3a2e0f7775cf30a2844957172"
  Description = "Nikkolasg's server: spreading the love of singing"

[[servers]]
  Address = "tcp://185.26.156.40:61117"
  Suite = "Ed25519"
  Public = "6a921638a4ade8970ebcd9e371570f08d71a24987f90f12391b9f6c525be5be4"
  Description = "Ismail's server"
  URL = "https://ismail.example.com/conode"
`

func TestReadGroupDescToml(t *testing.T) {
	group, err := ReadGroupDescToml(strings.NewReader(serverGroup))
	if err != nil {
		t.Fatal(err)
	}

	if len(group.Roster.List) != 2 {
		t.Fatal("Should have 2 ServerIdentities")
	}
	nikkoAddr := group.Roster.List[0].Address
	if !nikkoAddr.Valid() || nikkoAddr != network.NewTCPAddress("5.135.161.91:2000") {
		t.Fatal("Address not valid " + group.Roster.List[0].Address.String())
	}
	if len(group.Description) != 2 {
		t.Fatal("Should have 2 descriptions")
	}
	if group.Description[group.Roster.List[1]] != "Ismail's server" {
		t.Fatal("This should be Ismail's server")
	}
	if group.Roster.List[1].URL != "https://ismail.example.com/conode" {
		t.Fatal("Did not find expected URL.")
	}
}

func TestParseCothority(t *testing.T) {
	log.SetDebugVisible(2)
	suite := "Ed25519"
	public := "6a921638a4ade8970ebcd9e371570f08d71a24987f90f12391b9f6c525be5be4"
	private := "6a921638a4ade8970ebcd9e371570f08d71a24987f90f12391b9f6c525be5be4"
	address := "tcp://1.2.3.4:1234"
	listenAddr := "127.0.0.1:0"
	description := "This is a description."
	scPublic := "017f84a03a7833d74820be7cc3d8ad7adc29bf3af7025fd24176f5dc5b451ec23c8dc" +
		"82bbf856f10b422bc14e840222c2a91e1537372ab218b6f4f5d69e8f21d302f814a6d03b74012" +
		"4c7e6249960a770af381ed82d8aa8dbed961d6aef49779db06e4726c4de6d6d81e0e6431d3814" +
		"779b9f009f3a2e0f7775cf30a2844957172"
	scPrivate := "622f20fbc7995dd48bab00b0f3d7d13220a9d71716c6be7a45b4b284836041a8"

	privateInfo := fmt.Sprintf(`Suite = "%s"
        Public = "%s"
        Private = "%s"
        Address = "%s"
        ListenAddress = "%s"
		Description = "%s"
		[services]
			[services.skipchain]
			suite = "bn256.adapter"
			public = "%s"
			private = "%s"`,
		suite, public, private, address, listenAddr,
		description, scPublic, scPrivate)

	privateToml, err := ioutil.TempFile("", "temp_private.toml")
	require.Nil(t, err)

	privateToml.WriteString(privateInfo)
	privateToml.Close()

	cothConfig, srv, err := ParseCothority(privateToml.Name())
	require.Nil(t, err)

	// Check basic information
	require.Equal(t, suite, cothConfig.Suite)
	require.Equal(t, public, cothConfig.Public)
	require.Equal(t, private, cothConfig.Private)
	require.Equal(t, address, cothConfig.Address.String())
	require.Equal(t, listenAddr, cothConfig.ListenAddress)
	require.Equal(t, description, cothConfig.Description)
	require.Equal(t, scPublic, cothConfig.Services["skipchain"].Public)

	srv.Close()
}

func TestParseCothorityWithTLSWebSocket(t *testing.T) {
	suite := "Ed25519"
	public := "6a921638a4ade8970ebcd9e371570f08d71a24987f90f12391b9f6c525be5be4"
	private := "6a921638a4ade8970ebcd9e371570f08d71a24987f90f12391b9f6c525be5be4"
	address := "tcp://1.2.3.4:1234"
	listenAddr := "127.0.0.1:0"
	description := "This is a description."

	// Certificate and key examples taken from
	// 'https://gist.github.com/blinksmith/579b2650a09f128a03ca'
	wsTLSCert := `-----BEGIN CERTIFICATE-----
MIICEzCCAXygAwIBAgIQMIMChMLGrR+QvmQvpwAU6zANBgkqhkiG9w0BAQsFADAS
MRAwDgYDVQQKEwdBY21lIENvMCAXDTcwMDEwMTAwMDAwMFoYDzIwODQwMTI5MTYw
MDAwWjASMRAwDgYDVQQKEwdBY21lIENvMIGfMA0GCSqGSIb3DQEBAQUAA4GNADCB
iQKBgQDuLnQAI3mDgey3VBzWnB2L39JUU4txjeVE6myuDqkM/uGlfjb9SjY1bIw4
iA5sBBZzHi3z0h1YV8QPuxEbi4nW91IJm2gsvvZhIrCHS3l6afab4pZBl2+XsDul
rKBxKKtD1rGxlG4LjncdabFn9gvLZad2bSysqz/qTAUStTvqJQIDAQABo2gwZjAO
BgNVHQ8BAf8EBAMCAqQwEwYDVR0lBAwwCgYIKwYBBQUHAwEwDwYDVR0TAQH/BAUw
AwEB/zAuBgNVHREEJzAlggtleGFtcGxlLmNvbYcEfwAAAYcQAAAAAAAAAAAAAAAA
AAAAATANBgkqhkiG9w0BAQsFAAOBgQCEcetwO59EWk7WiJsG4x8SY+UIAA+flUI9
tyC4lNhbcF2Idq9greZwbYCqTTTr2XiRNSMLCOjKyI7ukPoPjo16ocHj+P3vZGfs
h1fIw3cSS2OolhloGw/XM6RWPWtPAlGykKLciQrBru5NAPvCMsb/I1DAceTiotQM
fblo6RBxUQ==
-----END CERTIFICATE-----`
	wsTLSCertKey := `-----BEGIN RSA PRIVATE KEY-----
MIICXgIBAAKBgQDuLnQAI3mDgey3VBzWnB2L39JUU4txjeVE6myuDqkM/uGlfjb9
SjY1bIw4iA5sBBZzHi3z0h1YV8QPuxEbi4nW91IJm2gsvvZhIrCHS3l6afab4pZB
l2+XsDulrKBxKKtD1rGxlG4LjncdabFn9gvLZad2bSysqz/qTAUStTvqJQIDAQAB
AoGAGRzwwir7XvBOAy5tM/uV6e+Zf6anZzus1s1Y1ClbjbE6HXbnWWF/wbZGOpet
3Zm4vD6MXc7jpTLryzTQIvVdfQbRc6+MUVeLKwZatTXtdZrhu+Jk7hx0nTPy8Jcb
uJqFk541aEw+mMogY/xEcfbWd6IOkp+4xqjlFLBEDytgbIECQQDvH/E6nk+hgN4H
qzzVtxxr397vWrjrIgPbJpQvBsafG7b0dA4AFjwVbFLmQcj2PprIMmPcQrooz8vp
jy4SHEg1AkEA/v13/5M47K9vCxmb8QeD/asydfsgS5TeuNi8DoUBEmiSJwma7FXY
fFUtxuvL7XvjwjN5B30pNEbc6Iuyt7y4MQJBAIt21su4b3sjXNueLKH85Q+phy2U
fQtuUE9txblTu14q3N7gHRZB4ZMhFYyDy8CKrN2cPg/Fvyt0Xlp/DoCzjA0CQQDU
y2ptGsuSmgUtWj3NM9xuwYPm+Z/F84K6+ARYiZ6PYj013sovGKUFfYAqVXVlxtIX
qyUBnu3X9ps8ZfjLZO7BAkEAlT4R5Yl6cGhaJQYZHOde3JEMhNRcVFMO8dJDaFeo
f9Oeos0UUothgiDktdQHxdNEwLjQf7lJJBzV+5OtwswCWA==
-----END RSA PRIVATE KEY-----`

	// Write files containing cert and key (+ be sure to delete them at the end)
	certFile, err := ioutil.TempFile("", "temp_cert.pem")
	defer func() {
		err := os.Remove(certFile.Name())
		require.Nil(t, err)
	}()
	require.Nil(t, err)
	certFile.WriteString(wsTLSCert)
	certFile.Close()

	keyFile, err := ioutil.TempFile("", "temp_key.pem")
	defer func() {
		err := os.Remove(keyFile.Name())
		require.Nil(t, err)
	}()
	require.Nil(t, err)
	keyFile.WriteString(wsTLSCertKey)
	keyFile.Close()

	// Testing different ways of putting TLS info.
	privateInfos := []string{
		fmt.Sprintf(`Suite = "%s"
            Public = "%s"
            Private = "%s"
            Address = "%s"
            ListenAddress = "%s"
            Description = "%s"
            WebSocketTLSCertificate = """string://%s"""
            WebSocketTLSCertificateKey = """string://%s"""`,
			suite, public, private, address, listenAddr,
			description, wsTLSCert, wsTLSCertKey),
		fmt.Sprintf(`Suite = "%s"
            Public = "%s"
            Private = "%s"
            Address = "%s"
            ListenAddress = "%s"
            Description = "%s"
            WebSocketTLSCertificate = "file://%s"
            WebSocketTLSCertificateKey = "file://%s"`,
			suite, public, private, address, listenAddr,
			description, certFile.Name(), keyFile.Name()),
		fmt.Sprintf(`Suite = "%s"
            Public = "%s"
            Private = "%s"
            Address = "%s"
            ListenAddress = "%s"
            Description = "%s"
            WebSocketTLSCertificate = "%s"
            WebSocketTLSCertificateKey = "%s"`,
			suite, public, private, address, listenAddr,
			description, certFile.Name(), keyFile.Name()),
	}

	for _, privateInfo := range privateInfos {
		privateToml, err := ioutil.TempFile("", "temp_private.toml")
		require.Nil(t, err)

		privateToml.WriteString(privateInfo)
		privateToml.Close()

		cothConfig, srv, err := ParseCothority(privateToml.Name())
		require.Nil(t, err)

		// Check basic information
		require.Equal(t, suite, cothConfig.Suite)
		require.Equal(t, public, cothConfig.Public)
		require.Equal(t, private, cothConfig.Private)
		require.Equal(t, address, cothConfig.Address.String())
		require.Equal(t, listenAddr, cothConfig.ListenAddress)
		require.Equal(t, description, cothConfig.Description)

		// Check content of certificate and key
		certContent, err := cothConfig.WebSocketTLSCertificate.Content()
		require.Nil(t, err)
		require.Equal(t, wsTLSCert, string(certContent))

		keyContent, err := cothConfig.WebSocketTLSCertificateKey.Content()
		require.Nil(t, err)
		require.Equal(t, wsTLSCertKey, string(keyContent))

		srv.Close()

		err = os.Remove(privateToml.Name())
		require.Nil(t, err)
	}
}
