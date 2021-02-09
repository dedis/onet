package platform

// Used for shell-commands

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"go.dedis.ch/onet/v3/log"
	"golang.org/x/xerrors"
)

// Scp copies the given files to the remote host
func Scp(username, host, file, dest string) error {
	addr := host + ":" + dest
	if username != "" {
		addr = username + "@" + addr
	}
	cmd := exec.Command("scp", "-r", file, addr)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return xerrors.Errorf("cmd: %v", err)
	}
	return nil
}

// Rsync copies files or directories to the remote host. If the DebugVisible
// is > 1, the rsync-operation is displayed on screen.
func Rsync(username, host, file, dest string) error {
	h, p, err := net.SplitHostPort(host)
	if err != nil {
		if !strings.Contains(err.Error(), "missing port in address") {
			return err
		}
		p = "22"
	}
	addr := h + ":" + dest
	if username != "" {
		addr = username + "@" + addr
	}
	cmd := exec.Command("rsync", "-Pauz", "-e", fmt.Sprintf("ssh -T -o Compression=no -x -p %s", p), file, addr)
	cmd.Stderr = os.Stderr
	if log.DebugVisible() > 1 {
		cmd.Stdout = os.Stdout
	}
	err = cmd.Run()
	if err != nil {
		return xerrors.Errorf("cmd: %v", err)
	}
	return nil
}

// SSHRun runs a command on the remote host
func SSHRun(username, host, command string) ([]byte, error) {
	addr := host
	if username != "" {
		addr = username + "@" + addr
	}

	log.Lvl2("Going to ssh to", addr, command)
	cmd := exec.Command("ssh", "-o", "StrictHostKeyChecking=no", addr,
		"eval '"+command+"'")
	buf, err := cmd.Output()
	if err != nil {
		return nil, xerrors.Errorf("cmd: %v", err)
	}
	return buf, nil
}

// SSHRunStdout runs a command on the remote host but redirects stdout and
// stderr of the Ssh-command to the os.Stderr and os.Stdout
func SSHRunStdout(username, host, command string) error {
	h, p, err := net.SplitHostPort(host)
	if err != nil {
	        if !strings.Contains(err.Error(), "missing port in address") {
	                return err
	        }
		p = "22"
	}
	addr := h
	if username != "" {
		addr = username + "@" + h
	}

	log.Lvl4("Going to ssh to", addr, command)
	cmd := exec.Command("ssh", "-o", "StrictHostKeyChecking=no", "-p", p, addr,
		"eval '"+command+"'")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	err = cmd.Run()
	if err != nil {
		return xerrors.Errorf("cmd: %v", err)
	}
	return nil
}

// Build builds the the golang packages in `path` and stores the result in `out`. Besides specifying the environment
// variables GOOS and GOARCH you can pass any additional argument using the buildArgs
// argument. The command which will be executed is of the following form:
// $ go build -v buildArgs... -o out path
func Build(path, out, goarch, goos string, buildArgs ...string) (string, error) {
	// When cross-compiling:
	// Run "go install" for the stdlib, to speed up future builds.
	// The first time we run this it builds and installs. Afterwards,
	// this finishes quickly and the later "go build" is faster.
	if goarch != runtime.GOARCH || goos != runtime.GOOS {
		cmd := exec.Command("go", []string{"env", "GOROOT"}...)
		gosrcB, err := cmd.Output()
		if err == nil {
			gosrcB := bytes.TrimRight(gosrcB, "\n\r")
			gosrc := filepath.Join(string(gosrcB), "src")
			cmd = exec.Command("go", []string{"install", "./..."}...)
			log.Lvl4("Installing cross-compilation stdlib in", gosrc)
			cmd.Env = append([]string{"GOOS=" + goos, "GOARCH=" + goarch}, os.Environ()...)
			cmd.Dir = gosrc
			log.Lvl4("Command:", cmd.Args, "in directory", gosrc)
			// Ignore errors from here; perhaps we didn't have rights to write.
			cmd.Run()
		}
	}

	var cmd *exec.Cmd
	var b bytes.Buffer
	buildBuffer := bufio.NewWriter(&b)
	wd, _ := os.Getwd()
	log.Lvl4("In directory", wd)
	var args []string
	args = append(args, "build", "-v")
	args = append(args, buildArgs...)
	args = append(args, "-o", out)
	cmd = exec.Command("go", args...)
	// we have to change the working directory to do the build when using
	// go modules, not sure about the exact reason for this behaviour yet
	cmd.Dir = path
	log.Lvl4("Building", cmd.Args, "in", path)
	cmd.Stdout = buildBuffer
	cmd.Stderr = buildBuffer
	cmd.Env = append([]string{"GOOS=" + goos, "GOARCH=" + goarch}, os.Environ()...)
	wd, err := os.Getwd()
	log.Lvl4(wd)
	log.Lvl4("Command:", cmd.Args)
	err = cmd.Run()
	if err != nil {
		err = xerrors.Errorf("cmd: %v", err)
	}
	log.Lvl4(b.String())
	return b.String(), err
}

// KillGo kills all go-instances
func KillGo() {
	cmd := exec.Command("killall", "go")
	if err := cmd.Run(); err != nil {
		log.Lvl3("Couldn't kill all go instances:", err)
	}
}
