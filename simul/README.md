# Simulation

The onet library allows for multiple levels of simulations:

-   [Localhost](./platform/LOCALHOST.md):
    -   up to 100 nodes
-   [Mininet](./platform/MININET.md):
    -   up to 300 nodes on a 48-core machine, multiplied by the number of machines
        available
    -   define max. bandwidth and delay for your network
-   [Deterlab](./platform/DETERLAB.md):
    -   up to 1000 nodes on a strong machine, multiplied by the number of machines
        available

Refer to the simulation-examples in one of the following places:
- [./manage/simulation](./manage/simulation)
- [./test_simul](./test_simul)
- https://github.com/dedis/cothority_template

## Runfile for simulations

Each simulation can have one or more .toml-files that describe a number of experiments
to be run on localhost or deterlab.

The .toml-files are split in two parts, separated by an empty line. The first
part consists of one or more 'global' variables that describe all experiments.

The second part starts with a line of variables that have to be defined for each
experiment, where each experiment makes up one line.

### Necessary variables

-   `Simulation` - what simulation to run
-   `Hosts` - how many hosts to instantiate - this corresponds to the nodes
 that will be running and available in the main `Roster`
-   `Servers` - how many servers to use maximum - if less than this number of
 servers are available, a warning will be printed, but the simulation will
  still be run 

The `Servers` will mostly influence how the simulation will be run.
Depending on the platform, this will be handled differently:
- `localhost` - `Servers` is ignored here
- `Deterlab` - the system will distribute the `Hosts` nodes over the
 available servers, but not over more than `Servers`.
 This allows for running simulations that are smaller than your whole DETERLab experiment without having to modify and restart the
  experiment.
- `Mininet` - as in `Deterlab`, the `Hosts` nodes will be distributed over
 a maximum of `Servers`.

### onet.SimulationBFTree

The standard simulation (and the only one implemented) is the
 `SimulationBFTree`, which will prepare the `Roster` and the `Tree` for the
 simulation.
Even if you use the `SimulationBFTree`, you're not restricted to use only the
 prepared `Tree`.
However, there will not be more nodes available than the ones in the prepared
 `Roster`.
Some restrictions apply when you're using the `Deterlab` simulation: 
- all nodes on one server (`Hosts` / min(available servers, `Servers`)) are
 run in one binary, which means
  - bandwidth measurements cover all the nodes
  - time measurements need to make sure no other calculations are taking place  
- the bandwidth- and delay-restrictions only apply between two physical servers, so
  - the simulation makes sure that all connected nodes in the `Tree` are always
    on different servers. If you use another communication than the one in the
    `Tree`, this will mean that the system cannot guarantee that the
    communication is restricted
  - the bandwidth restrictions apply to the sum of all communications between
   two servers, so to a number of hosts
If you want to have a bandwidth restriction that is between all nodes, and
 `Hosts > Servers`, you have to use the `Mininet` platform, which doesn't
  have this restriction.  

The following variables define how the original `Tree` is calculated - only
 one of the two should be given:

-   `BF` - branching factor: how many children each node has
-   `Depth` - the depth of the tree in levels below the root-node

If there are 13 `Hosts` with a `BF` of 3, the system will create a complete
 tree with the root-node having 3 children, and each of the children having 3
 more children.
The same setup can be achieved with 13 `Hosts` and a `Depth` of 3. 

If the tree to be created is not complete, it will be filled breath-first and
 the children of the last row will be distributed as evenly as possible. 

In addition, `Rounds` defines how many rounds the simulation will run.

### Statistics for subset of hosts

Buckets of statistics can be defined using the following variable:

-   `Buckets` - indices range of the buckets

The parameter is a string where the buckets are separated with spaces and the ranges
by a dash (e.g. `Buckets = "0:5 5:10-15:20"` that will create a bucket with hosts 0
to 4 and another one with hosts 5 to 9 and 15 to 19). Range indices can be compared
to Go slices so that the lower index is inclusive and the higher is exclusive.

A file will be written per bucket and the global one containing the statistics of all
the conodes will always be present independently from the parameter. Each file will have
the bucket number as suffix.

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

-   `RunWait` - how many seconds to wait for a run (one line of .toml-file) to finish
      (default: 180s)
-   `ExperimentWait` - how many seconds to wait for the while experiment to finish
      (default: RunWait \* #Runs)

### PreScript

If you need to run a script before the simulation is started (like installing
a missing library), you can define

-   `PreScript` - a shell-script that is run _before_ the simulation is started
    on each machine.
    It receives a single argument: the platform this simulation runs:
    [localhost,mininet,deterlab]

### MiniNet specific

Mininet has support for setting up delays and bandwidth for each simulation.
You can use the following two variables:

-   `Delay`[ms] - the delay between two hosts - the round-trip delay will be
    the double of this
-   `Bandwidth`[Mbps] - the bandwidth in both sending and receiving direction
    for each host, measured in mega bits per second

You can put these variables either globally at the top of the .toml file or
set them up for each line in the experiment (see the exapmles below).

### Experimental

-   `SingleHost` - which will reduce the tree to use only one host per server, and
    thus speeding up connections again
-   `Tags` - build-tags that will be called when building the binaries for the
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

## test_data format

Every simulation will be written to the `test_data` directory with the name
 of the simulation file as base and a `.csv` applied.
The configuration of the simulation file is written to the tables in the
 following columns, which are copied as-is from the simulation file:
 
- hosts, bf, delay, depth, other, prescript, ratio, rounds, servers, suite

For all the other measurements, the following statistics are available:

- `_avg` - the average
- `_std` - standard-deviation
- `_min` - minimum
- `_max` - maximum
- `_sum` - sum of all calls

### measure.NewTimeMeasure

The following measurements will be taken for `measure.NewTimeMeasure`:
- `_user` - user-space time, crypto and other calculations
- `_system` - system-space time - disk i/o network i/o
- `_wall` - wall-clock, as described above

The measurements are given in seconds.
There is an important difference in the `_wall` and the `_user`/`_system` 
measurements: the `_wall` measurements indicate how much time an external
 observer would have measured.
So if the system waits for a reply of the network, this waiting time is
 included in the measurement.
Contrary to this, the `_user`/`_system` measures how much work has been done
 by the CPU during the measurement.
When measuring parallel execution of code, it is possible that the 
`_user`/`_system` measurements are bigger than the `_wall` measurements
, because more than one CPU participated in the calculation.
The difference in `_user`/`_system` is explained for example here: 
https://stackoverflow.com/questions/556405/what-do-real-user-and-sys-mean-in-the-output-of-time1
The `_wall` corresponds to the `real` in this comment.

There are some standard time measurements done by the simulation:
- `ChildrenWait` - how long the system had to wait for all children to be
 available - might show problems in setting up the servers
- `SimulSyncWait` - how long the system had to wait at the end of the
 simulation - might indicate problems in the wrap-up of the simulation
 
### measure.NewCounterIOMeasure

If you want to measure bandwidth, you can use `measure.NewCounterIOMeasure`.
But you have to be careful to make sure that the system will not include
 traffic that is outside of your scope by putting the `.Record()` as close as
  possible to the `NewCounterIOMeasure`.
Every `CounterIOMeasure` has the following statistics:

- `_tx` - transmission-bytes
- `_rx` - bytes received
- `_msg_tx` - packets transmitted
- `_msg_rx` - packets received

Plus the standard modifiers (`_avg`, `_std`, ...).

There are two standard measurements done by every simulation:
- `bandwidth` (empty) - all node bandwidth
- `bandwidth_root` - bandwidth of the first node of the roster
