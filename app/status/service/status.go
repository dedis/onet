package status

import (
	"time"

	"github.com/dedis/onet"
	"github.com/dedis/onet/log"
	"github.com/dedis/onet/network"
	"github.com/dedis/onet/simul/manage"
)

// This file contains all the code to run a Stat service. The Stat receives takes a
// request for the Status reports of the server, and sends back the status reports for each service
// in the server.

// ServiceName is the name to refer to the Status service.
const ServiceName = "Status"

func init() {
	onet.RegisterNewService(ServiceName, newStatService)
}

// Status treats external request to this service.
func (st *Stat) Status(req *Request) (network.Body, onet.ClientError) {
	log.Lvl3("Returning", st.Context.ReportStatus())
	ret := &Response{
		Msg:            make(map[string]*Status),
		ServerIdentity: st.ServerIdentity(),
	}
	for k, v := range st.Context.ReportStatus() {
		ret.Msg[k] = &Status{Field: make(map[string]string)}
		for fk, fv := range v {
			ret.Msg[k].Field[fk] = fv
		}
	}
	return ret, nil
}

// Count returns the number of conodes replying
func (st *Stat) Count(req *CountRequest) (network.Body, onet.ClientError) {
	log.Lvl3("Starting to count", req.Roster, req.Roster.List[0].Address,
		st.ServerIdentity().Address)
	index, _ := req.Roster.Search(st.ServerIdentity().ID)
	if index < 0 {
		return nil, onet.NewClientErrorCode(ErrorONet, "Didn't find ourself in roster")
	}
	req.Roster.List[0], req.Roster.List[index] =
		req.Roster.List[index], req.Roster.List[0]
	pi, err := st.CreateProtocolOnet("Count", req.Roster.GenerateBinaryTree())
	if err != nil {
		return nil, onet.NewClientErrorCode(ErrorONet,
			"Couldn't start protocol: "+err.Error())
	}
	protocol := pi.(*manage.ProtocolCount)
	protocol.SetTimeout(req.Timeout)
	go pi.Start()
	select {
	case children := <-protocol.Count:
		return &CountResponse{children}, nil
	case <-time.After(time.Duration(req.Timeout) * time.Millisecond):
		return nil, onet.NewClientErrorCode(ErrorTimeout,
			"Children didn't reply in time")
	}
}

// newStatService creates a new service that is built for Status
func newStatService(c *onet.Context, path string) onet.Service {
	s := &Stat{
		ServiceProcessor: onet.NewServiceProcessor(c),
		path:             path,
	}
	err := s.RegisterHandlers(s.Status, s.Count)
	if err != nil {
		log.ErrFatal(err, "Couldn't register message:")
	}

	return s
}
