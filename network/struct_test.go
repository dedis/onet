package network

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/onet/v4/ciphersuite"
	"go.dedis.ch/onet/v4/log"
)

func TestServerIdentity(t *testing.T) {
	log.OutputToBuf()
	defer log.OutputToOs()
	pk1, _ := unsecureSuite.KeyPair()
	pk2, _ := unsecureSuite.KeyPair()

	si1 := NewServerIdentity(pk1.Pack(), NewLocalAddress("1"))
	si2 := NewServerIdentity(pk2.Pack(), NewLocalAddress("2"))

	if si1.Equal(si2) || !si1.Equal(si1) {
		t.Error("Stg's wrong with ServerIdentity")
	}

	if si1.ID.Equal(si2.ID) || !si1.ID.Equal(si1.ID) {
		t.Error("Stg's wrong with ServerIdentityID")
	}

	t1 := si1.Toml()
	if t1.Address != si1.Address || t1.Address == "" {
		t.Error("stg wrong with Toml()")
	}

	si11 := t1.ServerIdentity()
	if si11.Address != si1.Address || !si11.PublicKey.Equal(si1.PublicKey) {
		t.Error("Stg wrong with toml -> Si")
	}
	t1.PublicKey = &ciphersuite.CipherData{}
	si12 := t1.ServerIdentity()
	if si12.PublicKey != nil && si12.PublicKey.Equal(si1.PublicKey) {
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
	pk, sk := unsecureSuite.KeyPair()
	si := NewServerIdentity(pk.Pack(), NewLocalAddress("1"))
	si.SetPrivate(sk.Pack())

	spk, sks := unsecureSuite.KeyPair()
	si.ServiceIdentities = append(si.ServiceIdentities, NewServiceIdentity("a", spk.Pack(), sks.Pack()))
	si.ServiceIdentities = append(si.ServiceIdentities, NewServiceIdentity("d", spk.Pack(), nil))

	require.Equal(t, spk.Pack(), si.ServicePublic("a"))
	require.Equal(t, sks.Pack(), si.ServicePrivate("a"))
	require.Equal(t, pk.Pack(), si.ServicePublic("c"))
	require.Equal(t, sk.Pack(), si.ServicePrivate("c"))
	require.True(t, si.HasServiceKeyPair("a"))
	require.False(t, si.HasServiceKeyPair("b"))
	require.False(t, si.HasServiceKeyPair("c"))

	require.True(t, si.HasServicePublic("a"))
	require.False(t, si.HasServicePublic("c"))
	require.True(t, si.HasServicePublic("d"))
}
