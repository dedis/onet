package status

import (
	"github.com/dedis/onet"
	"github.com/dedis/onet/network"
)

// Client is a structure to communicate with status service
type Client struct {
	*onet.Client
}

// NewClient makes a new Client
func NewClient() *Client {
	return &Client{Client: onet.NewClient(ServiceName)}
}

// Request sends requests to all other members of network and creates client.
func (c *Client) Request(dst *network.ServerIdentity) (*Response, onet.ClientError) {
	resp := &Response{}
	cerr := c.SendProtobuf(dst, &Request{}, resp)
	if cerr != nil {
		return nil, cerr
	}
	return resp, nil
}

// Count starts the count-protocol and returns how many nodes replied.
// It takes a roster and a timout in seconds
func (c *Client) Count(r *onet.Roster, to int) (int, onet.ClientError) {
	resp := &CountResponse{}
	req := &CountRequest{r, to}
	cerr := c.SendProtobuf(r.RandomServerIdentity(), req, resp)
	if cerr != nil {
		return -1, cerr
	}
	return resp.Children, nil
}
