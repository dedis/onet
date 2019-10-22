[![Build Status](https://travis-ci.org/dedis/onet.svg?branch=master)](https://travis-ci.org/dedis/onet)
[![Go Report Card](https://goreportcard.com/badge/github.com/dedis/onet)](https://goreportcard.com/report/github.com/dedis/onet)
[![Coverage Status](https://coveralls.io/repos/github/dedis/onet/badge.svg)](https://coveralls.io/github/dedis/onet)

Navigation: [DEDIS](https://github.com/dedis/doc/tree/master/README.md) ::
Onet

# The Cothority Network Library - ONet

The Overlay-network (ONet) is a library for simulation and deployment of
decentralized, distributed protocols. It offers an abstraction for tree-based
communications between thousands of nodes and is used both in research for
testing out new protocols and running simulations, as well as in production to
deploy those protocols as a service in a distributed manner.

ONet is developed by [DEDIS/EFPL](http://dedis.epfl.ch) as part of the
[Cothority](https://github.com/dedis/cothority) project that aims to deploy
a large number of conodes for distributed signing and related projects.

## Documentation

- To run and use a conode, have a look at
	[Cothority Node](https://github.com/dedis/cothority/tree/master/conode)
	with examples of protocols, services and apps
- To start a new project by developing and integrating a new protocol, have a look at
	the [Cothority Template](https://github.com/dedis/cothority_template)
- To participate as a core-developer, read on!

## Further Links

This library offers a framework for research, simulation and deployment of
crypto-related protocols with an emphasis of decentralized, distributed
protocols.

So you want it all, go down to the base of the code and make it faster / more
secure / better understandable. Or perhaps you see a bug and want to fix it
yourself. Here is a list of places that can help you:

* [Simulation](simul/README.md) How to run simulations
* [Library Overview](LIBRARY.md) High level description of the Cothority framework
* [Architecture](ARCHITECTURE.md) big overview of what ONet does
* [Database](Database-backup-and-recovery.md) gives indications how to handle
the database used by onet
* [GoDoc](https://godoc.org/github.com/dedis/onet) entry point to the go-documentation
* [App support](app/README.md) useful libraries if you want to create a CLI app
for the cothority

## Directories

- [app](app) - libraries to write applications that communicate with services
- [cfgpath](cfgpath) - single package to get the configuration-path
- [log](log) - everybody needs its own log-library - this one has log-levels,
- colors, time, ...
- [network](network) - different type of connections: channels, tcp, to come: tls
- [simul](simul) - allowing to run your protocols and services on different
- platforms with up to 50'000 nodes

## Version

We have a development and a stable version. The `master`-branch in
`github.com/dedis/onet` is the development version that works but can have
incompatible changes.

The version at `gopkg.in/dedis/onet.v2` is stable and has no incompatible
changes. It will get updates from onet/master about once a month, and there
should be no API breaking changes.

Also have a look at https://github.com/dedis/onet/blob/master/CHANGELOG.md for
any incompatible changes.

## Server builder

A server can be created by using the builder interface that provides the
different functions to set the parameters and also defines the default
values.

Two builders are available: `DefaultBuilder` and `LocalBuilder`. The first will
spawn a server over TCP/TLS and the second will use a local network manager to
send the messages over channels.

```go
builder := NewDefaultBuilder()

builder.SetSuite(cipherSuite) // mandatory step

builder.SetService(service1, cipherSuite, newService1) // uses its own key pair
builder.SetService(service2, nil, newService2) // uses the master key pair
// ....

builder.SetPort(2000)

srv := builder.Build()
srv.Start()
```

## Cipher Suite

Onet uses a cipher suite interface to perform the cryptographic operation it
needs to operate. At least one master cipher suite must be provided when creating
a server. It will generate the master key pair that is the one used by default
to sign messages.

Each service can have a different cipher suite associated with it. The advantage
is that it will generate a different key pair for this service so it won't share
the master key pair. That means the cipher suite can be of the same type as the
master one if you only want to generate a different key pair for the service.

Onet only defines the primitives that it needs to operate but you are free when
creating a service to extend the base interface and force the registration of the
service with this extended cipher suite:

```go
type ExtendedCipherSuite interface {
	ciphersuite.CipherSuite

	MyNewFunction()
}

type myService struct{}

func newMyService(c Context, suite ciphersuite.CipherSuite) (Service, error) {
	mySuite := suite.(*ExtendedCipherSuite)

	// Create the service...

	return service, nil
}

func RegisterMyService(builder Builder, suite ExtendedCipherSuite) {
	builder.SetService(MyServiceName, suite, newMyService)
}

```

## License

All repositories for the cothority-project
([ONet](https://github.com/dedis/onet),
[cothority](https://github.com/dedis/cothority),
[cothority_template](https://github.com/dedis/cothority_template))
are double-licensed under a
GNU/AGPL 3.0 and a commercial license. If you want to have more information,
contact us at dedis@epfl.ch.

## Contribution

If you want to contribute to Cothority-ONet, please have a look at
[CONTRIBUTION](https://github.com/dedis/onet/blob/master/CONTRIBUTION) for
licensing details. Once you are OK with those, you can have a look at our
coding-guidelines in
[Coding](https://github.com/dedis/Coding). In short, we use the github-issues
to communicate and pull-requests to do code-review. Travis makes sure that
everything goes smoothly. And we'd like to have good code-coverage.

You are very welcome to help us in further developing ONet. Here are two pointers
to start:

* [Open issues](https://github.com/dedis/onet/issues) what we know to fail and how to work around it
* [[Coding|https://github.com/dedis/Coding]] technical aspects of programming in Cothority

# Contact

You can contact us at https://groups.google.com/forum/#!forum/cothority or
privately at dedis@epfl.ch.

# Reporting security problems

This library is offered as-is, and without a guarantee. It will need an
independent security review before it should be considered ready for use in
security-critical applications. If you integrate Onet into your application it
is YOUR RESPONSIBILITY to arrange for that audit.

If you notice a possible security problem, please report it
to dedis-security@epfl.ch.
