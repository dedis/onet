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
