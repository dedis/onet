Navigation: [DEDIS](https://github.com/dedis/doc/tree/master/README.md) ::
[Onet](../README.md) ::
Simulation

# Simulation

The simulation can be used to run the protocol or service in
different settings:

- localhost - for up to 100 nodes
- [Mininet](platform/MININET.md) - for up to 3000 nodes
- [Deterlab](platform/DETERLAB.md) - for up to 50'000 nodes

Refer to the simulation-examples in simul/manage/simulation and
https://github.com/dedis/cothority_template

## Runfile for simulations

Each simulation can have one or more .toml-files that describe a number of experiments
to be run on localhost or deterlab.

The .toml-files are split in two parts, separated by an empty line. The first
part consists of one or more 'global' variables that describe all experiments.

The second part starts with a line of variables that have to be defined for each
experiment, where each experiment makes up one line.

### Necessary variables

- `Simulation` - what simulation to run
- `Hosts` - how many hosts to instantiate
- `Servers` - how many servers to use

### onet.SimulationBFTree

If you use the `onet.SimulationBFTree`, the following variables are also available:

- `BF` - branching factor: how many children each node has
- `Depth` - the depth of the tree in levels below the root-node
- `Rounds` - for how many rounds the simulation should run

### Simulations with long setup-times and multiple measurements

Per default, all rounds of an individual simulation-run will be averaged and
written to the csv-file. If you set `IndividualStats` to a non-`""`-value,
every round will create a new line. This is useful if you have a simulation
with a long setup-time and you want to do multiple measurements for the same
setup.

### Timeouts

Timeouts are parsed according to Go's time.Duration: A duration string
is a possibly signed sequence of decimal numbers, each with optional
fraction and a unit suffix, such as "300ms", "-1.5h" or "2h45m". Valid
time units are "ns", "us" (or "µs"), "ms", "s", "m", "h".

Two timeout variables are available:

- `RunWait` - how many seconds to wait for a run (one line of .toml-file) to finish
    (default: 180s)
- `ExperimentWait` - how many seconds to wait for the while experiment to finish
    (default: RunWait * #Runs)

### PreScript

If you need to run a script before the simulation is started (like installing
a missing library), you can define

- `PreScript` - a shell-script that is run _before_ the simulation is started
  on each machine.
  It receives a single argument: the platform this simulation runs:
  [localhost,mininet,deterlab]

### MiniNet specific

Mininet has support for setting up delays and bandwidth for each simulation.
You can use the following two variables:
- `Delay`[ms] - the delay between two hosts - the round-trip delay will be
the double of this
- `Bandwidth`[MBps] - the bandwidth in both sending and receiving direction
for each host

You can put these variables either globally at the top of the .toml file or
set them up for each line in the experiment (see the exapmles below).

### Experimental

- `SingleHost` - which will reduce the tree to use only one host per server, and
thus speeding up connections again
- `Tags` - build-tags that will be called when building the binaries for the
simulation

### Example

    Simulation = "ExampleHandlers"
    Servers = 16
    BF = 2
    Rounds = 10
    #SingleHost = true

    Hosts
    3
    7
    15
    31
This will run the `ExampleHandlers`-simulation on 16 servers with a branching
factor of 2 and 10 rounds. The `SingleHost`-argument is commented out, so it
will use as many hosts as described.

In the second part, 4 experiments are defined, each only changing the number
of `Hosts`. First 3, then 7, 15, and finally 31 hosts are run on the 16
servers. For each experiment 10 rounds are started.

Assuming the simulation runs on MiniNet, the network delay can be set globally
as follows:

    Simulation = "ExampleHandlers"
	Delay = 100
    Servers = 16
    BF = 2
    Rounds = 10
    #SingleHost = true

    Hosts
    3
    7
    15
    31

Alternatively, it can be set for each individual experiment:

    Simulation = "ExampleHandlers"
    Servers = 16
    BF = 2
    Rounds = 10
    #SingleHost = true

    Hosts,Delay
    3,50
    7,100
    15,200
    31,400

