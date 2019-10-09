package platform

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"sync"

	"strings"

	"time"

	"go.dedis.ch/onet/v3"
	"go.dedis.ch/onet/v3/app"
	"go.dedis.ch/onet/v3/log"
	"go.dedis.ch/onet/v3/simul/monitor"
	"golang.org/x/xerrors"
)

// Localhost is responsible for launching the app with the specified number of nodes
// directly on your machine, for local testing.

// Localhost is the platform for launching thee apps locally
type Localhost struct {
	// Need mutex because build.go has a global variable that
	// is used for multiple experiments
	sync.Mutex

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

	// Suite used for the simulation
	Suite string

	// SimulationConfig holds all things necessary for the run
	sc *onet.SimulationConfig

	// PreScript is run before the simulation is started
	PreScript string

	// RunWait for long simulations
	RunWait string
}

// Configure various internal variables
func (d *Localhost) Configure(pc *Config) {
	d.Lock()
	defer d.Unlock()
	pwd, _ := os.Getwd()
	d.runDir = pwd + "/build"
	os.RemoveAll(d.runDir)
	log.ErrFatal(os.Mkdir(d.runDir, 0770))
	d.Suite = pc.Suite
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

// Build does nothing, as we're using our own binary, no need to build
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
	d.Lock()
	defer d.Unlock()
	if runtime.GOOS == "darwin" {
		files, err := exec.Command("ulimit", "-n").Output()
		if err != nil {
			return xerrors.Errorf("ulimit: %+v", err)
		}
		filesNbr, err := strconv.Atoi(strings.TrimSpace(string(files)))
		if err != nil {
			return xerrors.Errorf("atoi: %+v", err)
		}
		hosts, _ := strconv.Atoi(rc.Get("hosts"))
		if filesNbr < hosts*2 {
			maxfiles := 10000 + hosts*2
			return xerrors.Errorf("Maximum open files is too small. Please run the following command:\n"+
				"sudo sysctl -w kern.maxfiles=%d\n"+
				"sudo sysctl -w kern.maxfilesperproc=%d\n"+
				"ulimit -n %d\n"+
				"sudo sysctl -w kern.ipc.somaxconn=2048\n",
				maxfiles, maxfiles, maxfiles)
		}
	}

	// Check for PreScript and copy it to the deploy-dir
	d.PreScript = rc.Get("PreScript")
	if d.PreScript != "" {
		_, err := os.Stat(d.PreScript)
		if !os.IsNotExist(err) {
			if err := app.Copy(d.runDir, d.PreScript); err != nil {
				return xerrors.Errorf("copying: %+v", err)
			}
		}
	}

	d.servers, _ = strconv.Atoi(rc.Get("servers"))
	log.Lvl2("Localhost: Deploying and writing config-files for", d.servers, "servers")
	sim, err := onet.NewSimulation(d.Simulation, string(rc.Toml()))
	if err != nil {
		return xerrors.Errorf("simulation error: %+v", err)
	}
	d.addresses = make([]string, d.servers)
	for i := range d.addresses {
		d.addresses[i] = "127.0.0." + strconv.Itoa(i+1)
	}
	d.sc, err = sim.Setup(d.runDir, d.addresses)
	if err != nil {
		return xerrors.Errorf("simulation setup: %+v", err)
	}
	d.sc.Config = string(rc.Toml())
	if err := d.sc.Save(d.runDir); err != nil {
		return xerrors.Errorf("saving folder: %+v", err)
	}
	log.Lvl2("Localhost: Done deploying")
	d.wgRun.Add(d.servers)
	// add one to the channel length to indicate it's done
	d.errChan = make(chan error, d.servers+1)
	return nil
}

// Start will execute one cothority-binary for each server
// configured
func (d *Localhost) Start(args ...string) error {
	d.Lock()
	defer d.Unlock()
	if err := os.Chdir(d.runDir); err != nil {
		return err
	}
	log.Lvl4("Localhost: chdir into", d.runDir)
	ex := d.runDir + "/" + d.Simulation
	d.running = true
	log.Lvl1("Starting", d.servers, "applications of", ex)
	time.Sleep(100 * time.Millisecond)

	// If PreScript is defined, run the appropriate script _before_ the simulation.
	if d.PreScript != "" {
		out, err := exec.Command("sh", "-c", "./"+d.PreScript+" localhost").CombinedOutput()
		outStr := strings.TrimRight(string(out), "\n")
		if err != nil {
			return xerrors.Errorf("error deploying PreScript: " + err.Error() + " " + outStr)
		}
		log.Lvl1(outStr)
	}

	err := monitor.ConnectSink("localhost:" + strconv.Itoa(d.monitorPort))
	if err != nil {
		return xerrors.Errorf("monitor: %+v", err)
	}

	for index := 0; index < d.servers; index++ {
		log.Lvl3("Starting", index)
		host := "127.0.0." + strconv.Itoa(index+1)
		go func(i int, h string) {
			log.Lvl3("Localhost: will start host", i, h)
			err := Simulate(d.Suite, host, d.Simulation, "")
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
	d.Lock()
	defer d.Unlock()
	log.Lvl3("Waiting for processes to finish")

	wait, err := time.ParseDuration(d.RunWait)
	if err != nil || wait == 0 {
		wait = 600 * time.Second
		err = nil
	}

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
				log.Errorf("Couldn't cleanup running instances: %+v", err)
			}
			err = xerrors.Errorf("localhost error: %+v", err)
		}
	case <-time.After(wait):
		log.Lvl1("Quitting after waiting", wait)
	}

	errCleanup := os.Chdir(d.localDir)
	if errCleanup != nil {
		log.Error("Fail to restore the cwd: " + errCleanup.Error())
	}

	monitor.EndAndCleanup()
	log.Lvl2("Processes finished")
	return err
}
