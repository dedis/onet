package platform

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"sync"

	"strings"

	"github.com/dedis/onet"
	"github.com/dedis/onet/log"
	"github.com/dedis/onet/simul"
)

// Localhost is responsible for launching the app with the specified number of nodes
// directly on your machine, for local testing.

// Localhost is the platform for launching thee apps locally
type Localhost struct {

	// Address of the logger (can be local or not)
	logger string

	// The simulation to run
	Simulation string

	// Where is the Localhost package located
	localDir string
	// Where to build the executables +
	// where to read the config file
	// it will be assembled like LocalDir/RunDir
	runDir string

	// Debug level 1 - 5
	debug int

	// The number of servers
	servers int
	// All addresses - we use 'localhost1'..'localhostn' to
	// identify the different cothorities, but when opening the
	// ports they will be converted to normal 'localhost'
	addresses []string

	// Whether we started a simulation
	running bool
	// WaitGroup for running processes
	wgRun sync.WaitGroup

	// errors go here:
	errChan chan error

	// Listening monitor port
	monitorPort int

	// SimulationConfig holds all things necessary for the run
	sc *onet.SimulationConfig
}

// Configure various internal variables
func (d *Localhost) Configure(pc *Config) {
	pwd, _ := os.Getwd()
	d.runDir = pwd + "/build"
	os.RemoveAll(d.runDir)
	log.ErrFatal(os.Mkdir(d.runDir, 0770))
	d.localDir = pwd
	d.debug = pc.Debug
	d.running = false
	d.monitorPort = pc.MonitorPort
	if d.Simulation == "" {
		log.Fatal("No simulation defined in simulation")
	}
	log.Lvl3(fmt.Sprintf("Localhost dirs: RunDir %s", d.runDir))
	log.Lvl3("Localhost configured ...")
}

// As we're using our own binary, no need to build
func (d *Localhost) Build(build string, arg ...string) error {
	return nil
}

// Cleanup kills all running cothority-binaryes
func (d *Localhost) Cleanup() error {
	log.Lvl1("Nothing to clean up")
	return nil
}

// Deploy copies all files to the run-directory
func (d *Localhost) Deploy(rc *RunConfig) error {
	if runtime.GOOS == "darwin" {
		files, err := exec.Command("ulimit", "-n").Output()
		log.ErrFatal(err)
		filesNbr, err := strconv.Atoi(strings.TrimSpace(string(files)))
		log.ErrFatal(err)
		hosts, _ := strconv.Atoi(rc.Get("hosts"))
		if filesNbr < hosts*2 {
			maxfiles := 10000 + hosts*2
			log.Fatalf("Maximum open files is too small. Please run the following command:\n"+
				"sudo sysctl -w kern.maxfiles=%d\n"+
				"sudo sysctl -w kern.maxfilesperproc=%d\n"+
				"ulimit -n %d\n"+
				"sudo sysctl -w kern.ipc.somaxconn=2048\n",
				maxfiles, maxfiles, maxfiles)
		}
	}

	d.servers, _ = strconv.Atoi(rc.Get("servers"))
	log.Lvl2("Localhost: Deploying and writing config-files for", d.servers, "servers")
	sim, err := onet.NewSimulation(d.Simulation, string(rc.Toml()))
	if err != nil {
		return err
	}
	d.addresses = make([]string, d.servers)
	for i := range d.addresses {
		d.addresses[i] = "127.0.0." + strconv.Itoa(i)
	}
	d.sc, err = sim.Setup(d.runDir, d.addresses)
	if err != nil {
		return err
	}
	d.sc.Config = string(rc.Toml())
	if err := d.sc.Save(d.runDir); err != nil {
		return err
	}
	log.Lvl2("Localhost: Done deploying")
	return nil

}

// Start will execute one cothority-binary for each server
// configured
func (d *Localhost) Start(args ...string) error {
	if err := os.Chdir(d.runDir); err != nil {
		return err
	}
	log.Lvl4("Localhost: chdir into", d.runDir)
	ex := d.runDir + "/" + d.Simulation
	d.running = true
	log.Lvl1("Starting", d.servers, "applications of", ex)
	mon := "localhost:" + strconv.Itoa(d.monitorPort)
	d.wgRun.Add(d.servers)
	// add one to the channel length to indicate it's done
	d.errChan = make(chan error, d.servers+1)
	for index := 0; index < d.servers; index++ {
		log.Lvl3("Starting", index)
		host := "127.0.0." + strconv.Itoa(index)
		go func(i int, h string) {
			log.Lvl3("Localhost: will start host", i, h)
			err := simul.Simulate(host, d.Simulation, mon)
			if err != nil {
				log.Error("Error running localhost", h, ":", err)
				d.errChan <- err
			}
			d.wgRun.Done()
			log.Lvl3("host (index", i, ")", h, "done")
		}(index, host)
	}
	return nil
}

// Wait for all processes to finish
func (d *Localhost) Wait() error {
	log.Lvl3("Waiting for processes to finish")

	var err error
	go func() {
		d.wgRun.Wait()
		log.Lvl3("WaitGroup is 0")
		// write to error channel when done:
		d.errChan <- nil
	}()

	// if one of the hosts fails, stop waiting and return the error:
	select {
	case e := <-d.errChan:
		log.Lvl3("Finished waiting for hosts:", e)
		if e != nil {
			if err := d.Cleanup(); err != nil {
				log.Error("Couldn't cleanup running instances",
					err)
			}
			err = e
		}
	}

	log.Lvl2("Processes finished")
	return err
}
