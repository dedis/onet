// Package Dummy's only purpose is to create a DB that can be tested by the
// dbadmin binary.
// It sets up some services and stores data,
// only to be able to store the db-file.
//
// It uses a fixed private/public key,
// as the db-name is chosen using the hash of the key.
// Taking a fixed value (the base) makes testing easier.
package main

import (
	"flag"
	"fmt"
	"os"

	"go.dedis.ch/onet/v3/network"

	"go.dedis.ch/kyber/v3/group/edwards25519"

	"go.dedis.ch/onet/v3"
	"go.dedis.ch/onet/v3/log"
	"go.etcd.io/bbolt"
	"golang.org/x/xerrors"
)

var load = flag.Bool("load", false, "loading data from service")
var save = flag.Bool("save", false, "saving data from service")

func main() {
	flag.Parse()
	var a action
	switch {
	case *load:
		a = actionLoad
	case *save:
		a = actionSave
	default:
		log.Fatal("Please give either --load or --save")
	}

	_, err := onet.RegisterNewService("Foo", newService("foo", a))
	log.ErrFatal(err)
	_, err = onet.RegisterNewService("Bar", newService("bar", a))
	log.ErrFatal(err)

	log.ErrFatal(os.Setenv("CONODE_SERVICE_PATH", "."))
	suite := &edwards25519.SuiteEd25519{}
	_, si := onet.NewPrivIdentity(suite, 2000)
	si.Address = network.NewTCPAddress(si.Address.NetworkAddress())
	si.Public = suite.Point().Null()
	s := onet.NewServerTCP(si, suite)
	log.ErrFatal(s.Close())
}

type action int

const (
	actionLoad = iota
	actionSave
)

func newService(name string, a action) onet.NewServiceFunc {
	network.RegisterMessage(&Data{})
	name1 := []byte(name + "1")
	nameDB := []byte(name + "DB")
	return func(c *onet.Context) (onet.Service, error) {
		s := onet.NewServiceProcessor(c)
		switch a {
		case actionSave:
			err := s.Save(name1, &Data{name, 22})
			if err != nil {
				return nil, xerrors.Errorf("couldn't save data: %+v", err)
			}

			db, bName := c.GetAdditionalBucket(nameDB)
			err = db.Update(func(tx *bbolt.Tx) error {
				for i := 0; i < 10; i++ {
					err := tx.Bucket([]byte(bName)).Put(
						[]byte(fmt.Sprintf("key_%s_%d", name, i)),
						[]byte(fmt.Sprintf("data_%s_%d", name, i)))
					if err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				return nil, xerrors.Errorf("couldn't update db: %+v", err)
			}
		case actionLoad:
			d, err := s.Load(name1)
			if err != nil {
				return nil, err
			}
			if data, ok := d.(*Data); !ok {
				return nil, xerrors.Errorf("couldn't convert to data: %+v", err)
			} else {
				log.Infof("KV: Data_%s is: %+v", name, data)
			}

			db, bName := c.GetAdditionalBucket(nameDB)
			err = db.View(func(tx *bbolt.Tx) error {
				c := tx.Bucket([]byte(bName)).Cursor()
				i := 0
				for k, v := c.First(); k != nil; k, v = c.Next() {
					log.Infof("DB: Data_%s[%x]: %s / %s", name, i, string(k),
						string(v))
				}
				return nil
			})
		}
		return s, nil
	}
}

type ServiceBar struct {
	onet.Context
}

type Data struct {
	One string
	Two int
}
