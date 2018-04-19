package app

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/dedis/kyber"
	"github.com/dedis/kyber/suites"
	"github.com/dedis/kyber/util/encoding"
	"github.com/dedis/onet"
	"github.com/dedis/onet/log"
	"github.com/dedis/onet/network"
)

// CothorityConfig is the configuration structure of the cothority daemon.
// - Suite: The cryptographic suite
// - Public: The public key
// - Private: The Private key
// - Address: The external address of the conode, used by others to connect to this one
// - ListenAddress: The address this conode is listening on
// - Description: The description
// - WebSocketTLSCertificate: TLS certificate for the WebSocket
// - WebSocketTLSCertificateKey: TLS certificate key for the WebSocket
type CothorityConfig struct {
	Suite                      string
	Public                     string
	Private                    string
	Address                    network.Address
	ListenAddress              string
	Description                string
	WebSocketTLSCertificate    CertificateURL
	WebSocketTLSCertificateKey CertificateURL
}

// Save will save this CothorityConfig to the given file name. It
// will return an error if the file couldn't be created or if
// there is an error in the encoding.
func (hc *CothorityConfig) Save(file string) error {
	fd, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	fd.WriteString("# This file contains your private key.\n")
	fd.WriteString("# Do not give it away lightly!\n")
	err = toml.NewEncoder(fd).Encode(hc)
	if err != nil {
		return err
	}
	return nil
}

// ParseCothority parses the config file into a CothorityConfig.
// It returns the CothorityConfig, the Host so we can already use it, and an error if
// the file is inaccessible or has wrong values in it.
func ParseCothority(file string) (*CothorityConfig, *onet.Server, error) {
	hc := &CothorityConfig{}
	_, err := toml.DecodeFile(file, hc)
	if err != nil {
		return nil, nil, err
	}

	// Backwards compatibility with configs before we included the suite name
	if hc.Suite == "" {
		hc.Suite = "Ed25519"
	}
	suite, err := suites.Find(hc.Suite)
	if err != nil {
		return nil, nil, err
	}

	// Try to decode the Hex values
	private, err := encoding.StringHexToScalar(suite, hc.Private)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing private key: %v", err)
	}
	point, err := encoding.StringHexToPoint(suite, hc.Public)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing public key: %v", err)
	}
	si := network.NewServerIdentity(point, hc.Address)
	si.SetPrivate(private)
	si.Description = hc.Description
	// Same as `NewServerTCP` if `hc.ListenAddress` is empty
	server := onet.NewServerTCPWithListenAddr(si, suite, hc.ListenAddress)

	// Set Websocket TLS if possible
	if hc.WebSocketTLSCertificate != "" && hc.WebSocketTLSCertificateKey != "" {
		tlsCertificate, err := hc.WebSocketTLSCertificate.Content()
		if err != nil {
			return nil, nil, fmt.Errorf("getting WebSocketTLSCertificate content: %v", err)
		}
		tlsCertificateKey, err := hc.WebSocketTLSCertificateKey.Content()
		if err != nil {
			return nil, nil, fmt.Errorf("getting WebSocketTLSCertificateKey content: %v", err)
		}
		err = server.SetWebsocketTLS(tlsCertificate, tlsCertificateKey)
		if err != nil {
			return nil, nil, fmt.Errorf("setting TLS for websocket: %v", err)
		}
	}
	return hc, server, nil
}

// GroupToml holds the data of the group.toml file.
type GroupToml struct {
	Servers []*ServerToml `toml:"servers"`
}

// NewGroupToml creates a new GroupToml struct from the given ServerTomls.
// Currently used together with calling String() on the GroupToml to output
// a snippet which can be used to create a Cothority.
func NewGroupToml(servers ...*ServerToml) *GroupToml {
	return &GroupToml{
		Servers: servers,
	}
}

// ServerToml is one entry in the group.toml file describing one server to use for
// the cothority.
type ServerToml struct {
	Address     network.Address
	Suite       string
	Public      string
	Description string
}

// Group holds the Roster and the server-description.
type Group struct {
	Roster      *onet.Roster
	Description map[*network.ServerIdentity]string
}

// GetDescription returns the description of a ServerIdentity.
func (g *Group) GetDescription(e *network.ServerIdentity) string {
	return g.Description[e]
}

// ReadGroupDescToml reads a group.toml file and returns the list of ServerIdentities
// and descriptions in the file.
// If the file couldn't be decoded or doesn't hold valid ServerIdentities,
// an error is returned.
func ReadGroupDescToml(f io.Reader) (*Group, error) {
	group := &GroupToml{}
	_, err := toml.DecodeReader(f, group)
	if err != nil {
		return nil, err
	}
	// convert from ServerTomls to entities
	var entities = make([]*network.ServerIdentity, len(group.Servers))
	var descs = make(map[*network.ServerIdentity]string)
	for i, s := range group.Servers {
		// Backwards compatibility with old group files.
		if s.Suite == "" {
			s.Suite = "Ed25519"
		}
		en, err := s.toServerIdentity()
		if err != nil {
			return nil, err
		}
		entities[i] = en
		descs[en] = s.Description
	}
	el := onet.NewRoster(entities)
	return &Group{el, descs}, nil
}

// Save writes the GroupToml definition into the file given by its name.
// It will return an error if the file couldn't be created or if writing
// to it failed.
func (gt *GroupToml) Save(fname string) error {
	file, err := os.Create(fname)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(gt.String())
	return err
}

// String returns the TOML representation of this GroupToml.
func (gt *GroupToml) String() string {
	var buff bytes.Buffer
	for _, s := range gt.Servers {
		if s.Description == "" {
			s.Description = "Description of your server"
		}
	}
	enc := toml.NewEncoder(&buff)
	if err := enc.Encode(gt); err != nil {
		return "Error encoding grouptoml" + err.Error()
	}
	return buff.String()
}

// toServerIdentity converts this ServerToml struct to a ServerIdentity.
func (s *ServerToml) toServerIdentity() (*network.ServerIdentity, error) {
	suite, err := suites.Find(s.Suite)
	if err != nil {
		return nil, err
	}

	pubR := strings.NewReader(s.Public)
	public, err := encoding.ReadHexPoint(suite, pubR)
	if err != nil {
		return nil, err
	}
	return network.NewServerIdentity(public, s.Address), nil
}

// NewServerToml takes a public key and an address and returns
// the corresponding ServerToml.
// If an error occurs, it will be printed to StdErr and nil
// is returned.
func NewServerToml(suite network.Suite, public kyber.Point, addr network.Address,
	desc string) *ServerToml {
	var buff bytes.Buffer
	if err := encoding.WriteHexPoint(suite, &buff, public); err != nil {
		log.Error("Error writing public key")
		return nil
	}
	return &ServerToml{
		Address:     addr,
		Suite:       suite.String(),
		Public:      buff.String(),
		Description: desc,
	}
}

// String returns the TOML representation of the ServerToml.
func (s *ServerToml) String() string {
	var buff bytes.Buffer
	if s.Description == "" {
		s.Description = "## Put your description here for convenience ##"
	}
	enc := toml.NewEncoder(&buff)
	if err := enc.Encode(s); err != nil {
		return "## Error encoding server informations ##" + err.Error()
	}
	return buff.String()
}

// CertificateURLType represents the type of a CertificateURL.
// The supported types are defined as constants of type CertificateURLType.
type CertificateURLType string

// CertificateURL contains the CertificateURLType and the actual URL
// certificate, which can be a path leading to a file containing a certificate
// or a string directly being a certificate.
type CertificateURL string

const (
	// String is a CertificateURL type containing a certificate.
	String CertificateURLType = "string"
	// File is a CertificateURL type that contains the path to a file
	// containing a certificate.
	File = "file"
	// InvalidCertificateURLType is an invalid CertificateURL type.
	InvalidCertificateURLType = "wrong"
)

// typeCertificateURLSep is the separator between the type of the URL
// certificate and the string that identifies the certificate (e.g.
// filepath, content).
const typeCertificateURLSep = "://"

// certificateURLType converts a string to a CertificateURLType. In case of
// failure, it returns InvalidCertificateURLType.
func certificateURLType(t string) CertificateURLType {
	cuType := CertificateURLType(t)
	types := []CertificateURLType{String, File}
	for _, t := range types {
		if t == cuType {
			return cuType
		}
	}
	return InvalidCertificateURLType
}

// String returns the CertificateURL as a string.
func (cu CertificateURL) String() string {
	return string(cu)
}

// CertificateURLType returns the CertificateURL type from the CertificateURL.
// It returns InvalidCertificateURLType if the CertificateURL is not valid or
// if the CertificateURL type is not known.
func (cu CertificateURL) CertificateURLType() CertificateURLType {
	if !cu.Valid() {
		return InvalidCertificateURLType
	}
	vals := strings.Split(string(cu), typeCertificateURLSep)
	return certificateURLType(vals[0])
}

// Valid returns true if the CertificateURL is well formed or false otherwise.
func (cu CertificateURL) Valid() bool {
	vals := strings.Split(string(cu), typeCertificateURLSep)
	if len(vals) != 2 {
		return false
	}
	cuType := certificateURLType(vals[0])
	if cuType == InvalidCertificateURLType {
		return false
	}

	return true
}

// Content returns the bytes representing the certificate.
func (cu CertificateURL) Content() ([]byte, error) {
	vals := strings.Split(string(cu), typeCertificateURLSep)
	cuType := cu.CertificateURLType()
	if cuType == String {
		return []byte(vals[1]), nil
	}
	if cuType == File {
		dat, err := ioutil.ReadFile(vals[1])
		if err != nil {
			return nil, err
		}
		return dat, nil
	}
	return nil, fmt.Errorf("Unknown CertificateURL type (%s), cannot get its content", cuType)
}
