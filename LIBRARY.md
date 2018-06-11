Navigation: [DEDIS](https://github.com/dedis/doc/tree/master/README.md) ::
[Onet](README.md) ::
Library Overview

# Library Overview

This page is to describe the high level view of the cothority framework. In very
broad terms, onet allows you to set up the following three elements:
- *protocols*: short lived set of messages being passed back and forth between
one or more conodes
- *services*: define an api usable by client programs and instantiate protocols
- *apps*: communicate with the service-api of one or more conodes

Because onet comes from a research institute, we also provide a set of methods
to set up and run *simulations*.

## Protocol
It is an interface where users of the library must implement the logic of the
protocol they want to code. It is a short term entity that is self sufficient,
i.e. it does not need external access to any other resources of the Cothority
framework. A protocol can be launched from another protocol or by a Service.
Look at the `protocols` folder in the repo to get an idea.

## Service
It is a long term entity that is launched when a Conode is created. It serves
different purposes:
* serving external client requests,
* create/attach protocols with the Overlay, and launch them,
* communicate informations to other Services on other Conodes.

## App
An application in the onet context is a cli-program that interacts with one of
more conodes through the use of the api defined by one or more services. It is
mostly written in go, but in the cothority-repository you also find libraries
for interaction in javascript.

## Simulation
The onet library allows for multiple levels of simulations:
- localhost:
  - up to 100 nodes
- mininet:
  - up to 300 nodes on a 48-core machine, multiplied by the number of machines
  available
  - define max. bandwidth and delay for your network
- deterlab:
  - up to 1000 nodes on a strong machine, multiplied by the number of machines
  available

# Terminology

## Cothority
A collective authority (cothority) is a set of conodes that work together to
handle a distributed, decentralized task.

## Conode
It is the main entity of a Cothority server. When starting a conode, you define
which services are available by including them in your `main.go`.

## Roster
It is a list of conodes denoted by their public key and address. A Roster is
identified by its ID which is unique for each list.

## Tree
A tree is comprised of TreeNodes each of them denoted by their public key and
address. It is constructed out of a Roster.

# Technical details

## Network stack

The network stack is comprised of the Router which handles all incoming and
outgoing messages from/to the network. A Router can use different underlying
type of connections: TCP which uses regular TCP connections, Local which uses
channels and is mainly for testing purposes, and TLS which is still in progress.
More should be put into the network stack section.

## Overlay
It provides an abstraction to communicate over different Trees that the
Protocols and Services need. It handles multiple tasks:
* the propagations of the Roster and the Trees between different Conodes.
* the creation of the Protocols
* the dispatching of incoming and outgoing messages to the right Protocol.

## TreeNodeInstance
It is created by the Overlay, one for each Protocol, being the central point of
communication for a Protocol. It offers the latter some common tree methods such
as `SendParent`,`SendChild`, `IsRoot` etc. More importantly, it transforms and
embeds the message given by the Protocol into its own struct and dispatch it to
the Overlay for the sending part; vice versa for the reception part.

## ServiceManager
It is the main interface between the Conode and the Service. It transforms  and
embed the message created by the Service into its own format and pass it to the
Conode for the sending part; vice versa for the reception part.
