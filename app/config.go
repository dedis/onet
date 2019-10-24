package app

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"go.dedis.ch/onet/v4"
	"go.dedis.ch/onet/v4/ciphersuite"
	"go.dedis.ch/onet/v4/network"
	"golang.org/x/xerrors"
)

// CothorityConfig is the configuration structure of the cothority daemon.
// - Suite: The cryptographic suite
// - Public: The public key
// - Private: The Private key
// - Address: The external address of the conode, used by others to connect to this one
// - ListenAddress: The address this conode is listening on
// - Description: The description
// - URL: The URL where this server can be contacted externally.
// - WebSocketTLSCertificate: TLS certificate for the WebSocket
// - WebSocketTLSCertificateKey: TLS certificate key for the WebSocket
type CothorityConfig struct {
	Public                     *ciphersuite.RawPublicKey
	Private                    *ciphersuite.RawSecretKey
	Services                   map[string]ServiceConfig
	Address                    network.Address
	Description                string
	URL                        string
	WebSocketTLSCertificate    CertificateURL
	WebSocketTLSCertificateKey CertificateURL
}

// ServiceConfig is the configuration of a specific service to override
// default parameters as the key pair
type ServiceConfig struct {
	Public  *ciphersuite.RawPublicKey
	Private *ciphersuite.RawSecretKey
}

// Save will save this CothorityConfig to the given file name. It
// will return an error if the file couldn't be created or if
// there is an error in the encoding.
func (hc *CothorityConfig) Save(file string) error {
	fd, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return xerrors.Errorf("opening config file: %v", err)
	}
	fd.WriteString("# This file contains your private key.\n")
	fd.WriteString("# Do not give it away lightly!\n")
	err = toml.NewEncoder(fd).Encode(hc)
	if err != nil {
		return xerrors.Errorf("toml encoding: %v", err)
	}
	return nil
}

// LoadCothority loads a conode config from the given file.
func LoadCothority(file string) (*CothorityConfig, error) {
	hc := &CothorityConfig{}
	_, err := toml.DecodeFile(file, hc)
	if err != nil {
		return nil, xerrors.Errorf("toml decoding: %v", err)
	}
	return hc, nil
}

// GetServerIdentity will convert a CothorityConfig into a *network.ServerIdentity.
// It can give an error if there is a problem parsing the strings from the CothorityConfig.
func (hc *CothorityConfig) GetServerIdentity() (*network.ServerIdentity, error) {
	si := network.NewServerIdentity(hc.Public, hc.Address)
	si.SetPrivate(hc.Private)
	si.Description = hc.Description
	si.ServiceIdentities = parseServiceConfig(hc.Services)
	if hc.WebSocketTLSCertificateKey != "" {
		if hc.URL != "" {
			si.URL = strings.Replace(hc.URL, "http://", "https://", 0)
		} else {
			p, err := strconv.Atoi(si.Address.Port())
			if err != nil {
				return nil, xerrors.Errorf("port conversion: %v")
			}
			si.URL = fmt.Sprintf("https://%s:%d", si.Address.Host(), p+1)
		}
	} else {
		si.URL = hc.URL
	}

	return si, nil
}

// ParseCothority parses the config file into a CothorityConfig.
// It returns the CothorityConfig, the Host so we can already use it, and an error if
// the file is inaccessible or has wrong values in it.
func ParseCothority(builder onet.Builder, file string) (*CothorityConfig, *onet.Server, error) {
	hc, err := LoadCothority(file)
	if err != nil {
		return nil, nil, xerrors.Errorf("reading config: %v", err)
	}

	si, err := hc.GetServerIdentity()
	if err != nil {
		return nil, nil, xerrors.Errorf("parse server identity: %v", err)
	}

	builder.SetIdentity(si)

	// Set Websocket TLS if possible
	if hc.WebSocketTLSCertificate != "" && hc.WebSocketTLSCertificateKey != "" {
		if hc.WebSocketTLSCertificate.CertificateURLType() == File &&
			hc.WebSocketTLSCertificateKey.CertificateURLType() == File {
			// Use the reloader only when both are files as it doesn't
			// make sense for string embedded certificates.

			cert := []byte(hc.WebSocketTLSCertificate.blobPart())
			key := []byte(hc.WebSocketTLSCertificateKey.blobPart())
			builder.SetSSLCertificate(cert, key, true)
		} else {
			cert, err := hc.WebSocketTLSCertificate.Content()
			if err != nil {
				return nil, nil, xerrors.Errorf("getting WebSocketTLSCertificate content: %v", err)
			}
			key, err := hc.WebSocketTLSCertificateKey.Content()
			if err != nil {
				return nil, nil, xerrors.Errorf("getting WebSocketTLSCertificateKey content: %v", err)
			}

			builder.SetSSLCertificate(cert, key, false)
		}
	}

	server := builder.Build()
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
	Public      *ciphersuite.RawPublicKey
	Description string
	Services    map[string]ServerServiceConfig
	URL         string `toml:"URL,omitempty"`
}

// ServerServiceConfig is a public configuration for a server (i.e. private key
// is missing)
type ServerServiceConfig struct {
	Public *ciphersuite.RawPublicKey
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

// Toml returns the GroupToml instance of this Group
func (g *Group) Toml() (*GroupToml, error) {
	servers := make([]*ServerToml, len(g.Roster.List))
	for i, si := range g.Roster.List {
		services := make(map[string]ServerServiceConfig)
		for _, sid := range si.ServiceIdentities {
			services[sid.Name] = ServerServiceConfig{Public: sid.PublicKey.Clone()}
		}

		servers[i] = &ServerToml{
			Address:     si.Address,
			Public:      si.PublicKey.Clone(),
			Description: si.Description,
			Services:    services,
			URL:         si.URL,
		}
	}

	return &GroupToml{Servers: servers}, nil
}

// Save converts the group into a toml structure and save it to the file
func (g *Group) Save(filename string) error {
	gt, err := g.Toml()
	if err != nil {
		return xerrors.Errorf("toml encoding: %v", err)
	}

	return gt.Save(filename)
}

// ReadGroupDescToml reads a group.toml file and returns the list of ServerIdentities
// and descriptions in the file.
// If the file couldn't be decoded or doesn't hold valid ServerIdentities,
// an error is returned.
func ReadGroupDescToml(f io.Reader) (*Group, error) {
	group := &GroupToml{}
	_, err := toml.DecodeReader(f, group)
	if err != nil {
		return nil, xerrors.Errorf("toml decoding: %v", err)
	}
	// convert from ServerTomls to entities
	var entities = make([]*network.ServerIdentity, len(group.Servers))
	var descs = make(map[*network.ServerIdentity]string)
	for i, s := range group.Servers {
		en := s.ToServerIdentity()
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
		return xerrors.Errorf("creating file: %v", err)
	}
	defer file.Close()
	_, err = file.WriteString(gt.String())
	if err != nil {
		return xerrors.Errorf("writing file: %v", err)
	}

	return nil
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

// ToServerIdentity converts this ServerToml struct to a ServerIdentity.
func (s *ServerToml) ToServerIdentity() *network.ServerIdentity {
	si := network.NewServerIdentity(s.Public, s.Address)
	si.URL = s.URL
	si.Description = s.Description
	si.ServiceIdentities = parseServerServiceConfig(s.Services)

	return si
}

// NewServerToml takes a public key and an address and returns
// the corresponding ServerToml.
// If an error occurs, it will be printed to StdErr and nil
// is returned.
func NewServerToml(public *ciphersuite.RawPublicKey, addr network.Address, conf *CothorityConfig) *ServerToml {

	// Keep only the public key
	publics := make(map[string]ServerServiceConfig)
	for name, conf := range conf.Services {
		publics[name] = ServerServiceConfig{Public: conf.Public}
	}

	return &ServerToml{
		Address:     addr,
		Public:      public,
		Description: conf.Description,
		Services:    publics,
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
	// DefaultCertificateURLType is the default type when no type is specified
	DefaultCertificateURLType = File
)

// typeCertificateURLSep is the separator between the type of the URL
// certificate and the string that identifies the certificate (e.g.
// filepath, content).
const typeCertificateURLSep = "://"

// certificateURLType converts a string to a CertificateURLType. In case of
// failure, it returns InvalidCertificateURLType.
func certificateURLType(t string) CertificateURLType {
	if t == "" {
		return DefaultCertificateURLType
	}
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
	return certificateURLType(cu.typePart())
}

// Valid returns true if the CertificateURL is well formed or false otherwise.
func (cu CertificateURL) Valid() bool {
	vals := strings.Split(string(cu), typeCertificateURLSep)
	if len(vals) > 2 {
		return false
	}
	cuType := certificateURLType(cu.typePart())
	if cuType == InvalidCertificateURLType {
		return false
	}

	return true
}

// Content returns the bytes representing the certificate.
func (cu CertificateURL) Content() ([]byte, error) {
	cuType := cu.CertificateURLType()
	if cuType == String {
		return []byte(cu.blobPart()), nil
	}
	if cuType == File {
		dat, err := ioutil.ReadFile(cu.blobPart())
		if err != nil {
			return nil, xerrors.Errorf("reading file: %v", err)
		}
		return dat, nil
	}
	return nil, xerrors.Errorf("Unknown CertificateURL type (%s), cannot get its content", cuType)
}

// typePart returns only the string representing the type of a CertificateURL
// (empty string for no type specified)
func (cu CertificateURL) typePart() string {
	vals := strings.Split(string(cu), typeCertificateURLSep)
	if len(vals) == 1 {
		return ""
	}
	return vals[0]
}

// blobPart returns only the string representing the blob of a CertificateURL
// (the content of the certificate, a file path, ...)
func (cu CertificateURL) blobPart() string {
	vals := strings.Split(string(cu), typeCertificateURLSep)
	if len(vals) == 1 {
		return vals[0]
	}
	return vals[1]
}

// parseServiceConfig takes the map and creates service identities
func parseServiceConfig(configs map[string]ServiceConfig) []network.ServiceIdentity {
	sis := []network.ServiceIdentity{}

	for name, sc := range configs {
		sid := network.NewServiceIdentity(name, sc.Public, sc.Private)
		sis = append(sis, sid)
	}

	return sis
}

// parseServerServiceConfig takes the map and creates service identities with only the public key
func parseServerServiceConfig(configs map[string]ServerServiceConfig) []network.ServiceIdentity {
	sis := []network.ServiceIdentity{}

	for name, sc := range configs {
		sid := network.NewServiceIdentity(name, sc.Public, nil)
		sis = append(sis, sid)
	}

	return sis
}
