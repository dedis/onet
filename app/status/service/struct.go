package status

import (
	"github.com/dedis/onet"
	"github.com/dedis/onet/network"
)

const (
	// ErrorTimeout indicates a missed timeout while counting
	ErrorTimeout = 4100 + iota
	// ErrorONet indicates an error in ONet-calls
	ErrorONet
)

func init() {
	network.RegisterPacketType(&Request{})
	network.RegisterPacketType(&Response{})
}

// Stat is the service that returns the status reports of all services running on a server.
type Stat struct {
	*onet.ServiceProcessor
	path string
}

// Status holds all fields for one status.
type Status struct {
	Field map[string]string
}

// Request is what the Status service is expected to receive from clients.
type Request struct{}

// Response is what the Status service will reply to clients.
type Response struct {
	Msg            map[string]*Status
	ServerIdentity *network.ServerIdentity
}

// CountRequest starts a count-protocol over the Roster and waiting for
// a total of Timeout milliseconds.
type CountRequest struct {
	Roster  *onet.Roster
	Timeout int
}

// CountResponse returns the number of children that replied successfully
type CountResponse struct {
	Children int
}
