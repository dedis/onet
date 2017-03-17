#!/usr/bin/env bash

for path in :github.com/dedis/cothority/sda:gopkg.in/dedis/onet.v1: \
    :github.com/dedis/cothority/network:gopkg.in/dedis/onet.v1/network: \
    :github.com/dedis/cothority/log:gopkg.in/dedis/onet.v1/log: \
    :github.com/dedis/cothority/monitor:gopkg.in/dedis/onet.v1/simul/monitor: \
    :github.com/dedis/cothority/crypto:gopkg.in/dedis/onet.v1/crypto: \
    :github.com/dedis/crypto:gopkg.in/dedis/crypto.v0: \
    :github.com/dedis/cothority/protocols/manage:gopkg.in/dedis/cothority.v1/messaging:; do
        find . -name "*go" | xargs perl -pi -e "s$path"
done

for oldnew in sda\\.:onet. \
	manage\\.:messaging. \
	network\\.Body:network.Message \
	onet\\.ProtocolRegisterName:onet.GlobalProtocolRegister \
	network\\.RegisterHandler:network.RegisterMessage \
	ServerIdentity\\.Addresses:ServerIdentity.Address \
	CreateProtocolService:CreateProtocol \
	CreateProtocolSDA:CreateProtocol \
    RegisterPacketType:RegisterMessage \
    network\\.Packet:network.Envelope sda\\.Conode:onet.Server \
    UnmarshalRegistered:Unmarshal MarshalRegisteredType:Marshal ; do
    	echo replacing $oldnew
        find . -name "*go" | xargs -n 1 perl -pi -e s:$oldnew:g
done
