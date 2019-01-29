package network

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/kyber/v3/util/key"
	"go.dedis.ch/onet/v3/log"
)

func TestServerIdentity(t *testing.T) {
	log.OutputToBuf()
	defer log.OutputToOs()
	kp1 := key.NewKeyPair(tSuite)
	kp2 := key.NewKeyPair(tSuite)

	si1 := NewServerIdentity(kp1.Public, NewLocalAddress("1"))
	si2 := NewServerIdentity(kp2.Public, NewLocalAddress("2"))

	if si1.Equal(si2) || !si1.Equal(si1) {
		t.Error("Stg's wrong with ServerIdentity")
	}

	if si1.ID.Equal(si2.ID) || !si1.ID.Equal(si1.ID) {
		t.Error("Stg's wrong with ServerIdentityID")
	}

	t1 := si1.Toml(tSuite)
	if t1.Address != si1.Address || t1.Address == "" {
		t.Error("stg wrong with Toml()")
	}

	si11 := t1.ServerIdentity(tSuite)
	if si11.Address != si1.Address || !si11.Public.Equal(si1.Public) {
		t.Error("Stg wrong with toml -> Si")
	}
	t1.Public = ""
	si12 := t1.ServerIdentity(tSuite)
	if si12.Public != nil && si12.Public.Equal(si1.Public) {
		t.Error("stg wrong with wrong toml -> wrong si")
	}

}

func TestGlobalBind(t *testing.T) {
	gb, err := GlobalBind("127.0.0.1:2000")
	if err != nil {
		t.Fatal("global bind err", err)
	}
	if gb != ":2000" {
		t.Fatal("Wrong result", gb)
	}

	_, err = GlobalBind("127.0.0.12000")
	if err == nil {
		t.Fatal("Missing error for global bind")
	}

	// IPv6
	gb, err = GlobalBind("[::1]:2000")
	if err != nil {
		t.Fatal("global bind err", err)
	}
	if gb != ":2000" {
		t.Fatal("Wrong result", gb)
	}
}

// TestServiceIdentity checks that service identities are instantiated
// correctly and that we can access the keys
func TestServiceIdentity(t *testing.T) {
	kp := key.NewKeyPair(tSuite)
	si := NewServerIdentity(kp.Public, NewLocalAddress("1"))
	si.SetPrivate(kp.Private)

	pub := tSuite.Point()
	priv := tSuite.Scalar()
	kp2 := key.NewKeyPair(tSuite)
	si.ServiceIdentities = append(si.ServiceIdentities, NewServiceIdentity("a", tSuite, pub, priv))
	si.ServiceIdentities = append(si.ServiceIdentities, NewServiceIdentityFromPair("b", tSuite, kp2))

	require.Equal(t, pub, si.ServicePublic("a"))
	require.Equal(t, priv, si.ServicePrivate("a"))
	require.Equal(t, kp2.Public, si.ServicePublic("b"))
	require.Equal(t, kp2.Private, si.ServicePrivate("b"))
	require.Equal(t, kp.Public, si.ServicePublic("c"))
	require.Equal(t, kp.Private, si.ServicePrivate("c"))
	require.True(t, si.HasServiceKeyPair("a"))
	require.True(t, si.HasServiceKeyPair("b"))
	require.False(t, si.HasServiceKeyPair("c"))
}
