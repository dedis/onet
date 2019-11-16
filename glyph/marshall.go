package glyph

func (pk *PublicKey) Marshall() ([]byte, error) {
	t := pk.GetT()
	return t.MarshalBinary()
}

func (pk *PrivateKey) Marshall() ([]byte, error) {
	z1 := pk.GetS()
	z2 := pk.GetE()
	d1, e1 := z1.MarshalBinary()
	if e1 != nil {
		return nil, e1
	}
	d2, e2 := z2.MarshalBinary()
	if e2 != nil {
		return nil, e2
	}
	l1, l2 := len(d1), len(d2)
	data := make([]byte, l1+l2)
	copy(data[:l1], d1)
	copy(data[l1:l1+l2], d2)
	return data, nil
}

func (sig *Signature) Marshall() ([]byte, error) {
	z1 := sig.z1
	z2 := sig.z2
	c := sig.c
	d1, e1 := z1.MarshalBinary()
	if e1 != nil {
		return nil, e1
	}
	d2, e2 := z2.MarshalBinary()
	if e2 != nil {
		return nil, e2
	}
	d3, e3 := c.MarshalBinary()
	if e3 != nil {
		return nil, e3
	}
	l1, l2, l3 := len(d1), len(d2), len(d3)
	data := make([]byte, l1+l2+l3)
	copy(data[0:l1], d1)
	copy(data[l1:l1+l2], d2)
	copy(data[l1+l2:l1+l2+l3], d3)
	return data, nil
}
