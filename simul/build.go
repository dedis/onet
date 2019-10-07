package simul

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"math"
	"time"

	"go.dedis.ch/onet/v3/log"
	"go.dedis.ch/onet/v3/simul/monitor"
	"go.dedis.ch/onet/v3/simul/platform"
	"golang.org/x/xerrors"
)

// Configuration-variables
var platformDst = "localhost"
var nobuild = false
var clean = true
var build = ""
var machines = 3
var monitorPort = monitor.DefaultSinkPort
var simRange = ""
var race = false
var runWait = 180 * time.Second
var experimentWait = 0 * time.Second

func init() {
	flag.StringVar(&platformDst, "platform", platformDst, "platform to deploy to [localhost,mininet,deterlab]")
	flag.BoolVar(&nobuild, "nobuild", false, "Don't rebuild all helpers")
	flag.BoolVar(&clean, "clean", false, "Only clean platform")
	flag.StringVar(&build, "build", "", "List of packages to build")
	flag.BoolVar(&race, "race", false, "Build with go's race detection enabled (doesn't work on all platforms)")
	flag.IntVar(&machines, "machines", machines, "Number of machines on Deterlab")
	flag.IntVar(&monitorPort, "mport", monitorPort, "Port-number for monitor")
	flag.StringVar(&simRange, "range", simRange, "Range of simulations to run. 0: or 3:4 or :4")
	flag.DurationVar(&runWait, "runwait", runWait, "How long to wait for each simulation to finish - overwrites .toml-value")
	flag.DurationVar(&experimentWait, "experimentwait", experimentWait, "How long to wait for the whole experiment to finish")
	log.RegisterFlags()
}

// Reads in the platform that we want to use and prepares for the tests
func startBuild() {
	flag.Parse()
	deployP := platform.NewPlatform(platformDst)
	if deployP == nil {
		log.Fatal("Platform not recognized.", platformDst)
	}
	log.Lvl1("Deploying to", platformDst)

	simulations := flag.Args()
	if len(simulations) == 0 {
		log.Fatal("Please give a simulation to run")
	}

	for _, simulation := range simulations {
		runconfigs := platform.ReadRunFile(deployP, simulation)

		if len(runconfigs) == 0 {
			log.Fatal("No tests found in", simulation)
		}
		deployP.Configure(&platform.Config{
			MonitorPort: monitorPort,
			Debug:       log.DebugVisible(),
			Suite:       runconfigs[0].Get("Suite"),
		})

		if clean {
			err := deployP.Deploy(runconfigs[0])
			if err != nil {
				log.Fatal("Couldn't deploy:", err)
			}
			if err := deployP.Cleanup(); err != nil {
				log.Error("Couldn't cleanup correctly:", err)
			}
		} else {
			logname := strings.Replace(filepath.Base(simulation), ".toml", "", 1)
			testsDone := make(chan bool)
			timeout, err := getExperimentWait(runconfigs)
			if err != nil {
				log.Fatal("ExperimentWait:", err)
			}
			go func() {
				RunTests(deployP, logname, runconfigs)
				testsDone <- true
			}()
			select {
			case <-testsDone:
				log.Lvl3("Done with test", simulation)
			case <-time.After(timeout):
				log.Fatal("Test failed to finish in", timeout, "seconds")
			}
		}
	}
}

// RunTests the given tests and puts the output into the
// given file name. It outputs RunStats in a CSV format.
func RunTests(deployP platform.Platform, name string, runconfigs []*platform.RunConfig) {

	if nobuild == false {
		if race {
			if err := deployP.Build(build, "-race"); err != nil {
				log.Error("Couln't finish build without errors:",
					err)
			}
		} else {
			if err := deployP.Build(build); err != nil {
				log.Error("Couln't finish build without errors:",
					err)
			}
		}
	}

	mkTestDir()
	args := os.O_CREATE | os.O_RDWR | os.O_TRUNC
	// If a range is given, we only append
	if simRange != "" {
		args = os.O_CREATE | os.O_RDWR | os.O_APPEND
	}
	files := []*os.File{}
	defer func() {
		for _, f := range files {
			if err := f.Close(); err != nil {
				log.Error("Couln't close", f.Name())
			}
		}
	}()

	start, stop := getStartStop(len(runconfigs))
	for i, rc := range runconfigs {
		// Implement a simple range-argument that will skip checks not in range
		if i < start || i > stop {
			log.Lvl2("Skipping", rc, "because of range")
			continue
		}

		// run test t nTimes times
		// take the average of all successful runs
		log.Lvl1("Running test with config:", rc)
		stats, err := RunTest(deployP, rc)
		if err != nil {
			log.Error("Error running test:", err)
			continue
		}
		log.Lvl1("Test results:", stats[0])

		for j, bucketStat := range stats {
			if j >= len(files) {
				f, err := os.OpenFile(generateResultFileName(name, j), args, 0660)
				if err != nil {
					log.Fatal("error opening test file:", err)
				}
				err = f.Sync()
				if err != nil {
					log.Fatal("error syncing test file:", err)
				}

				files = append(files, f)
			}
			f := files[j]

			if i == 0 {
				bucketStat.WriteHeader(f)
			}
			if rc.Get("IndividualStats") != "" {
				err := bucketStat.WriteIndividualStats(f)
				log.ErrFatal(err)
			} else {
				bucketStat.WriteValues(f)
			}
			err = f.Sync()
			if err != nil {
				log.Fatal("error syncing data to test file:", err)
			}
		}
	}
}

// RunTest a single test - takes a test-file as a string that will be copied
// to the deterlab-server
func RunTest(deployP platform.Platform, rc *platform.RunConfig) ([]*monitor.Stats, error) {
	CheckHosts(rc)
	rc.Delete("simulation")
	stats := []*monitor.Stats{
		// this is the global bucket
		monitor.NewStats(rc.Map(), "hosts", "bf"),
	}

	if err := deployP.Cleanup(); err != nil {
		log.Error(err)
		return nil, err
	}

	if err := deployP.Deploy(rc); err != nil {
		log.Error(err)
		return nil, err
	}

	m := monitor.NewMonitor(stats[0])
	m.SinkPort = uint16(monitorPort)
	defer m.Stop()

	// create the buckets that will split the statistics of the hosts
	// according to the configuration file
	buckets, err := rc.GetBuckets()
	if err != nil {
		if err != platform.ErrorFieldNotPresent {
			return nil, err
		}

		// Do nothing, there won't be any bucket.
	} else {
		for i, rules := range buckets {
			bs := monitor.NewStats(rc.Map(), "hosts", "bf")
			stats = append(stats, bs)
			m.InsertBucket(i, rules, bs)
		}
	}

	done := make(chan error)
	go func() {
		if err := m.Listen(); err != nil {
			log.Error("error while closing monitor: " + err.Error())
		}
	}()

	go func() {
		// Start monitor before so ssh tunnel can connect to the monitor
		// in case of deterlab.
		err := deployP.Start()
		if err != nil {
			done <- err
			return
		}

		if err = deployP.Wait(); err != nil {
			log.Error("Test failed:", err)
			if err := deployP.Cleanup(); err != nil {
				log.Lvl3("Couldn't cleanup platform:", err)
			}
			done <- err
			return
		}
		done <- nil
	}()

	timeout, err := getRunWait(rc)
	if err != nil {
		log.Fatal("RunWait:", err)
	}

	// can timeout the command if it takes too long
	select {
	case err := <-done:
		if err != nil {
			return nil, err
		}
		return stats, nil
	case <-time.After(timeout):
		return nil, xerrors.New("simulation timeout")
	}
}

// CheckHosts verifies that at least two out of the three parameters: hosts, BF
// and depth are set in RunConfig. If one is missing, it tries to fix it. When
// more than one is missing, it stops the program.
func CheckHosts(rc *platform.RunConfig) {
	hosts, _ := rc.GetInt("hosts")
	bf, _ := rc.GetInt("bf")
	depth, _ := rc.GetInt("depth")
	if hosts == 0 {
		if depth == 0 || bf == 0 {
			log.Fatal("When hosts is not set, depth and BF must be set.")
		}
		hosts = calcHosts(bf, depth)
		rc.Put("hosts", strconv.Itoa(hosts))
	} else if bf == 0 {
		if depth == 0 || hosts == 0 {
			log.Fatal("When BF is not set, depth and hosts must be set.")
		}
		bf = 1
		for calcHosts(bf, depth) < hosts {
			bf++
		}
		rc.Put("bf", strconv.Itoa(bf))
	} else if depth == 0 {
		if hosts == 0 || bf == 0 {
			log.Fatal("When depth is not set, hsots and BF must be set.")
		}
		depth = 1
		for calcHosts(bf, depth) < hosts {
			depth++
		}
		rc.Put("depth", strconv.Itoa(depth))
	}
	// don't do anything if all three parameters are set
}

// Geometric sum to count the total number of nodes:
// Root-node: 1
// 1st level: bf (branching-factor)*/
// 2nd level: bf^2 (each child has bf children)
// 3rd level: bf^3
// So total: sum(level=0..depth)(bf^level)
func calcHosts(bf, depth int) int {
	if bf <= 0 {
		log.Fatal("illegal branching-factor")
	} else if depth <= 0 {
		log.Fatal("illegal depth")
	} else if bf == 1 {
		return depth + 1
	}
	return int((1 - math.Pow(float64(bf), float64(depth+1))) /
		float64(1-bf))
}

type runFile struct {
	Machines int
	Args     string
	Runs     string
}

func mkTestDir() {
	err := os.MkdirAll("test_data/", 0777)
	if err != nil {
		log.Fatal("failed to make test directory")
	}
}

func generateResultFileName(name string, index int) string {
	if index == 0 {
		// don't add the bucket index if it is the global one
		return fmt.Sprintf("test_data/%s.csv", name)
	}

	return fmt.Sprintf("test_data/%s_%d.csv", name, index)
}

// returns a tuple of start and stop configurations to run
func getStartStop(rcs int) (int, int) {
	ssStr := strings.Split(simRange, ":")
	start, err := strconv.Atoi(ssStr[0])
	stop := rcs - 1
	if err == nil {
		stop = start
		if len(ssStr) > 1 {
			stop, err = strconv.Atoi(ssStr[1])
			if err != nil {
				stop = rcs
			}
		}
	}
	log.Lvl2("Range is", start, ":", stop)
	return start, stop
}

// getRunWait returns either the command-line value or the value from the runconfig
// file
func getRunWait(rc *platform.RunConfig) (time.Duration, error) {
	rcWait, err := rc.GetDuration("runwait")
	if err == platform.ErrorFieldNotPresent {
		return runWait, nil
	}
	if err == nil {
		return rcWait, nil
	}
	return 0, err
}

// getExperimentWait returns, in the following order of precedence:
// 1. the command-line value
// 2. the value from runconfig
// 3. #runconfigs * runWait
func getExperimentWait(rcs []*platform.RunConfig) (time.Duration, error) {
	if experimentWait > 0 {
		return experimentWait, nil
	}
	rcExp, err := rcs[0].GetDuration("experimentwait")
	if err == nil {
		return rcExp, nil
	}
	// Probably a parse error parsing the duration.
	if err != platform.ErrorFieldNotPresent {
		return 0, err
	}

	// Otherwise calculate a useful default.
	wait := 0 * time.Second
	for _, rc := range rcs {
		w, err := getRunWait(rc)
		if err != nil {
			return 0, err
		}
		wait += w
	}
	return wait, nil
}
