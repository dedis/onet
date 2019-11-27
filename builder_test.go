package onet

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/onet/v3/ciphersuite"
)

func TestDefaultBuilder_SimpleUsageTCP(t *testing.T) {
	b := NewDefaultBuilder()
	b.SetPort(2000)
	b.SetSuite(testSuite)

	si := b.Identity()
	require.Equal(t, si.Address.String(), "tcp://127.0.0.1:2000")

	srv := b.Build()
	require.NotNil(t, srv)
	srv.Close()

	require.Equal(t, "tcp://127.0.0.1:2000", srv.ServerIdentity.Address.String())
	require.Equal(t, "http://127.0.0.1:2001", srv.ServerIdentity.URL)
}

func TestDefaultBuilder_SimpleUsageTLS(t *testing.T) {
	b := NewDefaultBuilder()
	b.SetPort(2000)
	b.SetSuite(testSuite)
	b.UseTLS()

	si := b.Identity()
	require.Equal(t, si.Address.String(), "tls://127.0.0.1:2000")

	srv := b.Build()
	require.NotNil(t, srv)
	srv.Close()

	require.Equal(t, "tls://127.0.0.1:2000", srv.ServerIdentity.Address.String())
	// still HTTP as there is no certificate.
	require.Equal(t, "http://127.0.0.1:2001", srv.ServerIdentity.URL)
}

func TestDefaultBuilder_WSS(t *testing.T) {
	cert, key, err := getSelfSignedCertificateAndKey()
	require.NoError(t, err)

	b := NewDefaultBuilder()
	b.SetPort(2000)
	b.SetSuite(testSuite)
	b.SetSSLCertificate(cert, key, false)

	srv := b.Build()
	require.NotNil(t, srv)
	srv.Close()

	require.Equal(t, "https://127.0.0.1:2001", srv.ServerIdentity.URL)
}

func TestDefaultBuilder_PortUndefined(t *testing.T) {
	b := NewDefaultBuilder()
	b.SetSuite(testSuite)

	srv := b.Build()
	require.NotNil(t, srv)
	srv.Close()

	require.Contains(t, srv.ServerIdentity.Address.String(), "tcp://127.0.0.1:")
	require.NotEqual(t, srv.ServerIdentity.Address.Port(), "0")
}

func TestDefaultBuilder_UseServerIdentity(t *testing.T) {
	cert, key, err := getSelfSignedCertificateAndKey()
	require.NoError(t, err)

	b := NewDefaultBuilder()
	b.SetSuite(testSuite)
	b.SetPort(2000)
	b.UseTLS()

	si := b.Identity()
	b2 := NewDefaultBuilder()
	b2.SetSuite(testSuite)
	b2.SetIdentity(si)
	b2.SetSSLCertificate(cert, key, false)

	srv := b2.Build()
	require.NotNil(t, srv)
	srv.Close()

	require.Equal(t, "tls://127.0.0.1:2000", srv.ServerIdentity.Address.String())
}

type fakeCipherSuite struct {
	*ciphersuite.Ed25519CipherSuite
}

func (c *fakeCipherSuite) Name() ciphersuite.Name {
	return "TEST_FAKE_CIPHER_SUITE"
}

func TestDefaultBuilder_RegisterServices(t *testing.T) {
	anotherSuite := ciphersuite.NewHope()

	b := NewDefaultBuilder()
	b.SetSuite(testSuite)
	b.SetService("fakeService1", nil, func(c *Context, s ciphersuite.CipherSuite) (Service, error) {
		if s != testSuite {
			t.Error("should be the same suite pointer")
		}
		return newDummyService2(c, s)
	})

	b.SetService("fakeService2", anotherSuite, func(c *Context, s ciphersuite.CipherSuite) (Service, error) {
		require.NotNil(t, c.Service("fakeService1"))
		if s != testSuite {
			t.Error("should be the same suite pointer")
		}
		return newDummyService2(c, s)
	})
	b.SetService("fakeService3", &fakeCipherSuite{}, func(c *Context, s ciphersuite.CipherSuite) (Service, error) {
		require.NotNil(t, c.Service("fakeService1"))
		require.NotNil(t, c.Service("fakeService2"))
		if s == testSuite {
			t.Error("should be a different suite pointer")
		}
		return newDummyService2(c, s)
	})

	srv := b.Build()
	srv.Close()

	require.Equal(t, 3, len(srv.serviceManager.services))
}
