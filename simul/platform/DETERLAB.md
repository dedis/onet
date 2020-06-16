Navigation: [DEDIS](https://github.com/dedis/doc/tree/master/README.md) ::
[Onet](../../README.md) ::
[Simulation](../README.md) ::
Deterlab

# Deterlab Simulation

The first implementation of a simulation (even before localhost) was a deterlab
simulation that allowed to run CoSi on https://deterlab.net. Deterlab is a
_state-of-the-art scientific computing facility for cyber-security researchers
engaged in research, development, discovery, experimentation, and testing of
innovative cyber-security technology_. It allows to reserve computers and define
the network between those. Using onet, it is very simple to pass from a localhost
simulation to a simulation using Deterlab.

Before a successful Deterlab simulation, you need to

1. be signed up at Deterlab. If you're working with DEDIS, ask your
responsible for the _Project Name_ and the _Group Name_.
2. create a simulation defined by an NS-file. You can find a simple
NS-file here: [cothority.ns](./deterlab_users/cothority.ns)
3. swap the simulation in

For point 3. it is important of course that Deterlab has enough machines
available at the moment of your experiment. If machines are missing, you might
change the NS-file to reference machines that are available. Attention: different
machines have different capabilities and can run more or less nodes, depending
on the power of the machine.

Supposing you have the points 1., 2., 3., solved, you can go on to the next step.

## Preparing automatic login

For easier simulation, you should prepare your ssh client to allow automatic
login to the remote server. If you login to the https://deterlab.net website,
go to _My DeterLab_ (on top), then _Profile_ (one of the tabs in the middle),
and finally chose _Edit SSH keys_ from the left. Now you can add your public
ssh-key to the list of allowed keys. Verify everything is running by checking
that you're not asked for your password when doing

```bash
ssh username@users.deterlab.net
```

## Running a Deterlab Simulation

If you have a successful [localhost](Localhost.md) simulation, it is very easy
to run it on Deterlab. Make sure the experiment is swapped in, then you can
start it with:

```go
go build && ./simul -platform deterlab simul.toml
```

Of course you'll need to adjust the `simul` and `simul.toml` to fit your
experiment. When running for the first time, the simulation will ask you for
your user-name, project and experiment. For the hostname of deterlab and the
monitor address, you can accept the default values. It will store all this in the
`deter.toml` file, where you can also change it. Or you delete the file, if you
want to enter the information again.

## Monitoring Port

During the experiment, your computer will play the role as the organizer and
start all the experiments on the remote deterlab network. This means, that
your computer needs to be online during the whole experiment.

To communicate with Deterlab, the simulation starts a ssh-tunnel to deterlab.net
where all information from the simulation are received. As there might
be more than one person using deterlab, and all users need to go through
users.deterlab.net, you might want to change the port-number you're using. It's
as easy as:

```go
./simul -platform deterlab -mport 10002 simult.toml
```

This will use port 10002 to communicate with deterlab. Be sure to increment by
2, so only use even numbers.

## Oversubscription

If you have more nodes than available servers, the simulation will put multiple
cothority-nodes on the same server. When creating the `Tree` in
`Simulation.Setup`, it will make sure that any parent-children connection will
go through the network, so that every communication between a parent and a
child will go through the bandwidth and timing restrictions.

Unfortunately this means that not all nodes will have these restrictions, and
in the case of a broadcast with oversubscription, some nodes might communicate
directly with the other nodes. If you need a fully restricted network, you
need to use [Mininet](MININET.md)
