[![Build Status](https://travis-ci.org/dedis/onet.svg?branch=master)](https://travis-ci.org/dedis/onet)
[![Go Report Card](https://goreportcard.com/badge/github.com/dedis/onet)](https://goreportcard.com/report/github.com/dedis/onet)
[![Coverage Status](https://coveralls.io/repos/github/dedis/onet/badge.svg)](https://coveralls.io/github/dedis/onet)
[![Codacy Badge](https://api.codacy.com/project/badge/Grade/11ec79aa77fe41748edfdfcd55e92fab)](https://www.codacy.com/manual/nkcr/onet?utm_source=github.com&utm_medium=referral&utm_content=dedis/onet&utm_campaign=Badge_Grade)

# The Cothority Overlay Network Library - Onet

The Overlay-network (Onet) is a library for simulation and deployment of
decentralized, distributed protocols. This library offers a framework for
research, simulation, and deployment of crypto-related protocols with an emphasis
on decentralized, distributed protocols. It offers an abstraction for tree-based
communications between thousands of nodes and it is used both in research for
testing out new protocols and running simulations, as well as in production to
deploy those protocols as a service in a distributed manner.

**Onet** is developed by [DEDIS/EFPL](http://dedis.epfl.ch) as part of the
[Cothority](https://github.com/dedis/cothority) project that aims to deploy a
large number of nodes for distributed signing and related projects. In
cothority, nodes are commonly named "conodes". A collective authority
(cothority) is a set of conodes that work together to handle a distributed,
decentralized task.

Onet allows you to set up the following three elements:

-   _protocols_: a short-lived set of messages being passed back and forth between
    one or more conodes

-   _services_: define an API usable by client programs and instantiate protocols

-   _apps_: communicate with the service-API of one or more conodes

We also provide a set of methods to set up and run _simulations_.

* * *

<!-- START doctoc.sh generated TOC please keep comment here to allow auto update -->

<!-- DO NOT EDIT THIS SECTION, INSTEAD RE-RUN doctoc.sh TO UPDATE -->
**:book: Table of Contents**

-   [The Cothority Overlay Network Library - Onet](#the-cothority-overlay-network-library---onet)
-   [General information](#general-information)
    -   [Directories](#directories)
    -   [Version](#version)
    -   [License](#license)
    -   [Contribution](#contribution)
    -   [Contact](#contact)
    -   [Reporting security problems](#reporting-security-problems)
-   [Components](#components)
    -   [Router](#router)
    -   [Conode](#conode)
    -   [Roster](#roster)
    -   [Protocol](#protocol)
    -   [Service](#service)
    -   [ServiceManager](#servicemanager)
    -   [Tree](#tree)
    -   [Overlay](#overlay)
    -   [TreeNodeInstance](#treenodeinstance)
    -   [App](#app)
-   [Database Backup and Recovery](#database-backup-and-recovery)
    -   [Backup](#backup)
    -   [Recovery](#recovery)
    -   [Interacting with the database](#interacting-with-the-database)
-   [Simulation](#simulation)

<!-- END doctoc.sh generated TOC please keep comment here to allow auto update -->

# General information

## Directories

-   [app](app) - useful libraries if you want to create a CLI app for the
    cothority

-   [cfgpath](cfgpath) - single package to get the configuration-path

-   [log](log) - everybody needs its own log-library - this one has log-levels,
    colors, time, ...

-   [network](network) - different type of connections: channels, tcp, tls

-   [simul](simul) - allowing to run your protocols and services on different
    platforms with up to 50'000 nodes

## Version

The Onet library follows the same development cycle as the one described in
the [dedis/cothority](https://github.com/dedis/cothority/tree/master/) project.

## License

All repositories for the cothority-project
([Onet](https://github.com/dedis/onet),
[cothority](https://github.com/dedis/cothority),
[cothority_template](https://github.com/dedis/cothority_template))
are double-licensed under a
GNU/AGPL 3.0 and a commercial license. If you want to have more information,
contact us at dedis@epfl.ch.

## Contribution

If you want to contribute, please have a look at
[CONTRIBUTION](https://github.com/dedis/onet/blob/master/CONTRIBUTION) for
licensing details and feel free to open a pull request.

## Contact

You can contact us at <https://groups.google.com/forum/#!forum/cothority> or
privately at dedis@epfl.ch.

## Reporting security problems

This library is offered as-is without any guarantees. It would need an
independent security review before it should be considered ready for use in
security-critical applications. If you integrate Onet into your application it
is YOUR RESPONSIBILITY to arrange for that audit.

If you notice a possible security problem, please report it
to dedis-security@epfl.ch.

# Components

In Onet, you define _Services_ that use _Protocols_ which can send and receive
messages. Each _Protocol_ is instantiated when needed as a _ProtocolInstance_.
As multiple _Protocols_ can be run at the same time, there can be more than one
_ProtocolInstance_ of the same _Protocol_. Onet makes sure all messages get
routed to the appropriate _ProtocolInstance_.

Foreign applications can communicate with Onet over the service-API, which is
implemented using protobuf over WebSockets for JavaScript compatibility.

This chapter provides a high-level description of the cothority framework. Let's
start with a picture and then dive into each main components of the
library.

![system overview](http://www.plantuml.com/plantuml/proxy?cache=no&src=https://raw.githubusercontent.com/dedis/onet/master/docs/diagrams/components.iuml)

As you can see there's a bunch of different entities involved. Let's get down
the rabbit hole to explain the most important ones!

## Router

The Router handles all incoming and outgoing messages from and to the network. A
Router can use different underlying types of connection: 

-   _TCP_ which uses regular TCP connections, 
-   _Local_ which uses channels and is mainly for testing purposes, and 
-   _TLS_ which is still in progress.

## Conode

A conode is the main entity of a Cothority server. It holds the Router, the
Overlay, and the different Services. Generally, for developing an application
using the framework, you would create your Router first, then the Conode, and
then finally call `conode.Start()`.

## Roster

A Roster is simply a list of Conodes denoted by their public key and address. A
Roster is identified by its ID, which is unique for each list.

## Protocol

A Protocol is an interface where users of the library must implement the logic
of the protocol they want to code. It is supposed to be a short term entity that
is self-sufficient, i.e. it does not need external access to any other resources
of the Cothority framework. A protocol can be launched from SDA itself or by a
Service.

## Service

A Service is a long term entity that is created when a Conode is created. It
serves different purposes:

-   serving external client requests,
-   creating and attaching protocols with the Overlay (and launching them),
-   communicating information to other Services on other Conodes.

## ServiceManager

A ServiceManager is the main interface between the Conode and the Service. It
transforms  and embed the messages created by the Service to its own format and
pass it to the Conode for the sending part; vice versa for the reception part.

## Tree

A Tree is a standard tree data structure where each node - called
_TreeNode_ - is denoted by its public key and address. The Tree is constructed
out of a Roster.

## Overlay

The Overlay provides an abstraction to communicate over different Trees that the
Protocols and Services need. It handles the following tasks:

-   Propagations of the Roster and the Trees between different Conodes
-   Creation of the Protocol
-   Dispatching of incoming and outgoing messages to the right Protocol

## TreeNodeInstance

A TreeNodeInstance is created by the Overlay. There is one TreeNodeInstance for
each Protocol and it acts as the central point of communication for that
Protocol. The TreeNodeInstance offers to its Protocol some common tree methods
such as `SendParent`,`SendChild`, `IsRoot` etc. More importantly, it transforms
and embeds the message given by the Protocol into its own struct and dispatch it
to the Overlay for the sending part; vice versa for the reception part.

## App

An application in the context of Onet is a CLI-program that interacts with one
or more conodes through the use of the API defined by one or more services. It
is mostly written in go, but in the cothority-repository you can also find
libraries to interact in javascript and java.

# Database Backup and Recovery

Users of Onet have the option to make use of its built-in database.

We use [bbolt](https://github.com/etcd-io/bbolt), which supports "fully
serializable ACID transactions" to ensure data integrity for Onet users. Users
should be able to do the following:

-   Backup data while Onet is running
-   Recovery from a backup in case of data corruption

## Backup

Users are recommended to perform frequent backups such that data can be
recovered if Onet nodes fail. Onet stores all of its data in the context folder,
specified by `$CONODE_SERVICE_PATH`. If unset, it defaults to

-   `~/Library/Application Support/conode/data` on macOS,
-   `$HOME\AppData\Local\Conode` on Windows, or
-   `~/.local/share/conode` on other Unix/Linux.

Hence, to backup, it is recommended to use a standard backup tool, such as
rsync, and copy the folder to a different physical location periodically.
The database keeps a transaction log.

Performing backups in the middle of a transaction should not be a problem.
However, it is still recommended to check the data integrity of the backed-up
file using the bbolt CLI, i.e. `bolt check database_name.db`.

To install the bbolt CLI, see [Bolt Installation](https://github.com/etcd-io/bbolt#installing).

## Recovery

Data corruption is easy to detect as Onet nodes crash when reading from a
corrupted database, at startup or during operation. Concretely, the bbolt
library would panic
([source](https://github.com/etcd-io/bbolt/blob/386b851495d42c4e02908838373a06d0a533e170/freelist.go#L237)).
This behavior is produced by writing a few blocks of random data using `dd` to
the database.

In case of data corruption, the database must be restored from a backup by
simply copying the backup copy to the context directory, and then starting the
conode again. It is the user's responsibility to make sure that the data is up
to date, e.g. by reading the latest data from running Onet nodes.

## Interacting with the database

The primary and recommended methods to interact with the database are
[`Load`](https://godoc.org/github.com/dedis/onet#Context.Load) and
[`Save`](https://godoc.org/github.com/dedis/onet#Context.Save). If more control
on the database is needed, then we can ask the context to return a database
handler and bucket name using the function
[`GetAdditionalBucket`](https://godoc.org/github.com/dedis/onet#Context.GetAdditionalBucket).

All the [bbolt functions](https://godoc.org/github.com/etcd-io/bbolt) can be
used with the database handler. However, the user should avoid creating new
buckets using the bbolt functions and only use `GetAdditionalBucket` to avoid
bucket name conflicts.

# Simulation

Have a look at the `simul/README.md` for explanations about simulations.
