package onet

import "golang.org/x/xerrors"

func (c *Server) CreateProtocol(name string, t *Tree) (ProtocolInstance, error) {
	pi, err := c.overlay.CreateProtocol(name, t, NilServiceID)
	if err != nil {
		return nil, xerrors.Errorf("creating protocol: %v", err)
	}
	return pi, nil
}

func (c *Server) StartProtocol(name string, t *Tree) (ProtocolInstance, error) {
	pi, err := c.overlay.StartProtocol(name, t, NilServiceID)
	if err != nil {
		return nil, xerrors.Errorf("starting protocol: %v", err)
	}
	return pi, nil
}

func (c *Server) Roster(id RosterID) (*Roster, bool) {
	ro := c.overlay.treeStorage.GetRoster(id)
	return ro, ro != nil
}

func (c *Server) GetTree(id TreeID) (*Tree, bool) {
	t := c.overlay.treeStorage.Get(id)
	return t, t != nil
}

func (c *Server) Overlay() *Overlay {
	return c.overlay
}

func (o *Overlay) TokenToNode(tok *Token) (*TreeNodeInstance, bool) {
	tni, ok := o.instances[tok.ID()]
	return tni, ok
}

// AddTree registers the given Tree struct in the underlying overlay.
// Useful for unit-testing only.
func (c *Server) AddTree(t *Tree) {
	c.overlay.RegisterTree(t)
}
