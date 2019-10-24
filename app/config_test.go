package app

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/onet/v4"
	"go.dedis.ch/onet/v4/ciphersuite"
	"go.dedis.ch/onet/v4/log"
	"go.dedis.ch/onet/v4/network"
)

var o bytes.Buffer

var testSuite = &ciphersuite.UnsecureCipherSuite{}
var testBuilder = onet.NewDefaultBuilder()

const testServiceName = "OnetConfigTestService"

func TestMain(m *testing.M) {
	out = &o
	testBuilder.SetSuite(testSuite)
	testBuilder.SetService(testServiceName, testSuite, func(c *onet.Context, suite ciphersuite.CipherSuite) (onet.Service, error) {
		return nil, nil
	})
	log.MainTest(m)
}

var serverGroup = `Description = "Default Dedis Cothority"

[[servers]]
  Address = "tcp://5.135.161.91:2000"
  Public = "150000004349504845525f53554954455f554e5345435552452c82b7c526b8092c2c56f993a5c734f8"
  Description = "Nikkolasg's server: spreading the love of singing"
  [servers.Services]
	[servers.Services.OnetConfigTestService]
	Public = "150000004349504845525f53554954455f554e5345435552452c82b7c526b8092c2c56f993a5c734f8"
	[servers.Services.abc]
	Public = "150000004349504845525f53554954455f554e5345435552452c82b7c526b8092c2c56f993a5c734f8"

[[servers]]
  Address = "tcp://185.26.156.40:61117"
  Public = "150000004349504845525f53554954455f554e5345435552452c82b7c526b8092c2c56f993a5c734f8"
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

	require.Equal(t, 2, len(group.Roster.List[0].ServiceIdentities))
}

// TestSaveGroup checks that the group is correctly written into the file
func TestSaveGroup(t *testing.T) {
	group, err := ReadGroupDescToml(strings.NewReader(serverGroup))
	require.NoError(t, err)

	tmp, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	defer os.RemoveAll(tmp)

	filename := path.Join(tmp, "public.toml")

	err = group.Save(filename)
	require.NoError(t, err)

	data, err := ioutil.ReadFile(filename)
	require.NoError(t, err)
	fmt.Print(string(data))
	require.Contains(t, string(data), serverGroup[strings.LastIndex(serverGroup, "[[servers]]"):])
}

func TestParseCothority(t *testing.T) {
	public := "150000004349504845525f53554954455f554e5345435552452c82b7c526b8092c2c56f993a5c734f8"
	private := "150000004349504845525f53554954455f554e5345435552452c82b7c526b8092c2c56f993a5c734f8"
	address := "tcp://1.2.3.4:1234"
	description := "This is a description."
	scPublic := "150000004349504845525f53554954455f554e5345435552452c82b7c526b8092c2c56f993a5c734f8"
	scPrivate := "150000004349504845525f53554954455f554e5345435552452c82b7c526b8092c2c56f993a5c734f8"

	privateInfo := fmt.Sprintf(`
        Public = "%s"
        Private = "%s"
        Address = "%s"
		Description = "%s"
		[services]
			[services.%s]
			public = "%s"
			private = "%s"
			[services.abc]
			public = "150000004349504845525f53554954455f554e5345435552452c82b7c526b8092c2c56f993a5c734f8"`,
		public, private, address,
		description, testServiceName, scPublic, scPrivate)

	privateToml, err := ioutil.TempFile("", "temp_private.toml")
	require.Nil(t, err)

	privateToml.WriteString(privateInfo)
	privateToml.Close()

	builder := onet.NewDefaultBuilder()
	builder.SetSuite(testSuite)

	cothConfig, srv, err := ParseCothority(builder, privateToml.Name())
	require.Nil(t, err)

	// Check basic information
	publicStr, err := cothConfig.Public.MarshalText()
	require.NoError(t, err)
	require.Equal(t, public, string(publicStr))
	privateStr, err := cothConfig.Private.MarshalText()
	require.NoError(t, err)
	require.Equal(t, private, string(privateStr))
	require.Equal(t, address, cothConfig.Address.String())
	require.Equal(t, description, cothConfig.Description)
	require.Equal(t, 2, len(srv.ServerIdentity.ServiceIdentities))
	publicStr, err = cothConfig.Services[testServiceName].Public.MarshalText()
	require.NoError(t, err)
	require.Equal(t, scPublic, string(publicStr))
	privateStr, err = cothConfig.Services[testServiceName].Private.MarshalText()
	require.NoError(t, err)
	require.Equal(t, scPrivate, string(privateStr))

	srv.Close()
}

func TestParseCothorityWithTLSWebSocket(t *testing.T) {
	public := "150000004349504845525f53554954455f554e5345435552452c82b7c526b8092c2c56f993a5c734f8"
	private := "150000004349504845525f53554954455f554e5345435552452c82b7c526b8092c2c56f993a5c734f8"
	address := "tcp://1.2.3.4:1234"
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
		fmt.Sprintf(`
            Public = "%s"
            Private = "%s"
            Address = "%s"
            Description = "%s"
            WebSocketTLSCertificate = """string://%s"""
            WebSocketTLSCertificateKey = """string://%s"""`,
			public, private, address,
			description, wsTLSCert, wsTLSCertKey),
		fmt.Sprintf(`
            Public = "%s"
            Private = "%s"
            Address = "%s"
            Description = "%s"
            WebSocketTLSCertificate = "file://%s"
            WebSocketTLSCertificateKey = "file://%s"`,
			public, private, address,
			description, certFile.Name(), keyFile.Name()),
		fmt.Sprintf(`
            Public = "%s"
            Private = "%s"
            Address = "%s"
            Description = "%s"
            WebSocketTLSCertificate = "%s"
            WebSocketTLSCertificateKey = "%s"`,
			public, private, address,
			description, certFile.Name(), keyFile.Name()),
	}

	for i, privateInfo := range privateInfos {
		privateToml, err := ioutil.TempFile("", "temp_private.toml")
		require.Nil(t, err)

		privateToml.WriteString(privateInfo)
		privateToml.Close()

		builder := onet.NewDefaultBuilder()
		builder.SetSuite(testSuite)

		cothConfig, srv, err := ParseCothority(builder, privateToml.Name())
		require.Nil(t, err)

		// Check basic information
		publicStr, err := cothConfig.Public.MarshalText()
		require.NoError(t, err)
		require.Equal(t, public, string(publicStr))
		privateStr, err := cothConfig.Private.MarshalText()
		require.NoError(t, err)
		require.Equal(t, private, string(privateStr))
		require.Equal(t, address, cothConfig.Address.String())
		require.Equal(t, description, cothConfig.Description)

		// Check content of certificate and key
		certContent, err := cothConfig.WebSocketTLSCertificate.Content()
		require.Nil(t, err)
		require.Equal(t, wsTLSCert, string(certContent))

		keyContent, err := cothConfig.WebSocketTLSCertificateKey.Content()
		require.Nil(t, err)
		require.Equal(t, wsTLSCertKey, string(keyContent))

		if i != 0 {
			// Check when the certificate is a file.
			require.NotNil(t, srv.WebSocket.TLSConfig.GetCertificate)

			cert, err := srv.WebSocket.TLSConfig.GetCertificate(nil)
			require.NoError(t, err)
			require.NotNil(t, cert)
		}

		srv.Close()

		err = os.Remove(privateToml.Name())
		require.Nil(t, err)
	}
}
