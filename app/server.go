package app

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"go.dedis.ch/onet/v3"
	"go.dedis.ch/onet/v3/cfgpath"
	"go.dedis.ch/onet/v3/log"
	"go.dedis.ch/onet/v3/network"
	"golang.org/x/xerrors"
)

// DefaultServerConfig is the default server configuration file-name.
const DefaultServerConfig = "private.toml"

// DefaultGroupFile is the default group definition file-name.
const DefaultGroupFile = "public.toml"

// DefaultPort to listen and connect to. As of this writing, this port is not listed in
// /etc/services
const DefaultPort = 7770

// DefaultAddress where to be contacted by other servers.
const DefaultAddress = "127.0.0.1"

// Service used to get the public IP-address.
const portscan = "https://blog.dedis.ch/portscan.php"

// InteractiveConfig uses stdin to get the [address:]PORT of the server.
// If no address is given, portscan is used to find the public IP. In case
// no public IP can be configured, localhost will be used.
// If everything is OK, the configuration-files will be written.
// In case of an error this method Fatals.
func InteractiveConfig(builder *onet.DefaultBuilder, binaryName string) {
	log.Info("Setting up a cothority-server.")

	// Force the use of TLS for production servers.
	builder.UseTLS()

	str := Inputf(strconv.Itoa(DefaultPort), "Please enter the [address:]PORT for incoming to bind to and where other nodes will be able to contact you.")

	if !strings.Contains(str, ":") {
		str = ":" + str
	}
	host, port, err := net.SplitHostPort(str)
	log.ErrFatal(err, "Couldn't interpret", str)

	if port != "" {
		iport, err := strconv.Atoi(port)
		if err != nil {
			log.Error(err)
			return
		}
		builder.SetPort(iport)
	} else {
		builder.SetPort(DefaultPort)
	}

	if host != "" {
		builder.SetHost(host)
	} else {
		builder.SetHost("0.0.0.0")
	}

	log.Info()
	log.Info("We now need to get a reachable address for other Servers")
	log.Info("and clients to contact you. This address will be put in a group definition")
	log.Info("file that you can share and combine with others to form a Cothority roster.")

	// if IP was not provided then let's get the public IP address
	if host == "" {
		// TODO: https://stackoverflow.com/questions/23558425/how-do-i-get-the-local-ip-address-in-go
		// TODO: this depends on an online service.
		resp, err := http.Get(portscan)
		// cant get the public ip then ask the user for a reachable one
		if err != nil {
			log.Error("Could not get your public IP address")
		} else {
			buff, err := ioutil.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				log.Error("Could not parse your public IP address", err)
			} else {
				host = strings.TrimSpace(string(buff))
				builder.SetHost(host)
			}
		}
	}

	// create the keys
	si := builder.Identity()

	if si.Address.Public() {
		// TODO: is it really necessary ?
		if err := tryConnect(si.Address); err != nil {
			log.Error(err)
			return
		}
	}

	conf := &CothorityConfig{
		Public:   si.PublicKey,
		Private:  si.GetPrivate(),
		Address:  si.Address,
		Services: extractServiceIdentities(si),
		Description: Input("New cothority",
			"Give a description of the cothority"),
	}

	var configFolder string
	var defaultFolder = cfgpath.GetConfigPath(binaryName)
	var configFile string
	var groupFile string

	for {
		// get name of config file and write to config file
		configFolder = Input(defaultFolder, "Please enter a folder for the configuration files")
		configFile = path.Join(configFolder, DefaultServerConfig)
		groupFile = path.Join(configFolder, DefaultGroupFile)

		// check if the directory exists
		if _, err := os.Stat(configFolder); os.IsNotExist(err) {
			log.Info("Creating inexistant directory configuration", configFolder)
			if err = os.MkdirAll(configFolder, 0744); err != nil {
				log.Fatalf("Could not create directory configuration %s %v", configFolder, err)
			}
		}

		if checkOverwrite(configFile) && checkOverwrite(groupFile) {
			break
		}
	}

	server := NewServerToml(si.PublicKey, si.Address, conf)
	group := NewGroupToml(server)

	saveFiles(conf, configFile, group, groupFile)
	log.Info("All configurations saved, ready to serve signatures now.")
}

// GenerateServiceKeyPairs generates a map of the service with their
// key pairs. It can be used to generate server configuration.
func extractServiceIdentities(si *network.ServerIdentity) map[string]ServiceConfig {
	services := make(map[string]ServiceConfig)
	for _, srvid := range si.ServiceIdentities {
		services[srvid.Name] = ServiceConfig{
			Public:  srvid.PublicKey,
			Private: srvid.GetPrivate(),
		}
	}

	return services
}

// Returns true if file exists and user confirms overwriting, or if file doesn't exist.
// Returns false if file exists and user doesn't confirm overwriting.
func checkOverwrite(file string) bool {
	// check if the file exists and ask for override
	if _, err := os.Stat(file); err == nil {
		return InputYN(true, "Configuration file "+file+" already exists. Override?")
	}
	return true
}

// saveFiles takes a CothorityConfig and its filename, and a GroupToml and its filename,
// and saves the data to these files.
// In case of a failure it Fatals.
func saveFiles(conf *CothorityConfig, fileConf string, group *GroupToml, fileGroup string) {
	if err := conf.Save(fileConf); err != nil {
		log.Fatal("Unable to write the config to file:", err)
	}
	log.Info("Success! You can now use the conode with the config file", fileConf)
	// group definition part
	if err := group.Save(fileGroup); err != nil {
		log.Fatal("Could not write your group file snippet:", err)
	}

	log.Info("Saved a group definition snippet for your server at", fileGroup)
	log.Info(group.String())
}

// tryConnect binds to the given IP address and ask an internet service to
// connect to it. binding is the address where we must listen (needed because
// the reachable address might not be the same as the binding address => NAT, ip
// rules etc).
// In case anything goes wrong, an error is returned.
func tryConnect(ip network.Address) error {
	stopCh := make(chan bool, 1)
	listening := make(chan bool)
	// let's bind
	go func() {
		ln, err := net.Listen("tcp", ip.NetworkAddress())
		if err != nil {
			log.Error("Trouble with binding to the address:", err)
			return
		}
		listening <- true
		con, err := ln.Accept()
		if err != nil {
			log.Error("Error while accepting connections: ", err.Error())
			return
		}
		<-stopCh
		con.Close()
	}()
	defer func() { stopCh <- true }()
	select {
	case <-listening:
	case <-time.After(2 * time.Second):
		return xerrors.New("timeout while listening on " + ip.NetworkAddress())
	}
	conn, err := net.Dial("tcp", ip.NetworkAddress())
	log.ErrFatal(err, "Could not connect itself to public address.\n"+
		"This is most probably an error in your system-setup.\n"+
		"Please make sure this conode can connect to ", ip.NetworkAddress())

	log.Info("Successfully connected to our own port")
	conn.Close()

	_, portStr, err := net.SplitHostPort(ip.NetworkAddress())
	if err != nil {
		return xerrors.Errorf("invalid address: %v", err)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return xerrors.Errorf("invalid port: %v", err)
	}

	// Ask the check. Since the public adress may not be available at this time
	// we set up a timeout of 10 seconds.
	url := fmt.Sprintf("%s?port=%d", portscan, port)
	log.Infof("Trying for 10 sec to get the public IP (%s)...", url)
	timeout := time.Duration(10 * time.Second)
	client := http.Client{
		Timeout: timeout,
	}
	resp, err := client.Get(url)
	// can't get the public ip then ask the user for a reachable one
	if err != nil {
		return xerrors.New("...could not get your public IP address")
	}

	buff, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return xerrors.Errorf("reading body: %v", err)
	}

	res := string(buff)
	if res != "Open" {
		return xerrors.Errorf("Portscan returned: %s", res)
	}
	return nil
}

// MakeServer creates a conode with the given config file name. It can
// be used by different apps (like CoSi, for example)
func MakeServer(builder onet.Builder, configFilename string) *onet.Server {
	if _, err := os.Stat(configFilename); os.IsNotExist(err) {
		log.Fatalf("[-] Configuration file does not exist. %s", configFilename)
	}
	// Let's read the config
	_, server, err := ParseCothority(builder, configFilename)
	if err != nil {
		log.Fatal("Couldn't parse config:", err)
	}
	return server
}
