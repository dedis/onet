package onet

import (
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/kyber/v3/util/key"
	"go.dedis.ch/onet/v3/log"
	"go.dedis.ch/onet/v3/network"
	bbolt "go.etcd.io/bbolt"
	"golang.org/x/xerrors"
)

type ContextData struct {
	I int64
	S string
}

func TestContextSaveLoad(t *testing.T) {
	tmp, err := ioutil.TempDir("", "conode")
	defer os.RemoveAll(tmp)
	os.Setenv("CONODE_SERVICE_PATH", tmp)
	p := dbPathFromEnv()
	require.Equal(t, p, tmp)

	nbr := 10
	c := make([]*Context, nbr)
	for i := range c {
		c[i] = createContext(t, p)
	}

	testSaveFailure(t, c[0])

	var wg sync.WaitGroup
	wg.Add(nbr)
	for i := range c {
		go func(i int) {
			// defer insures the call even on a panic
			defer wg.Done()
			testLoadSave(t, c[i])
		}(i)
	}
	wg.Wait()
	files, err := ioutil.ReadDir(tmp)
	log.ErrFatal(err)
	require.False(t, files[0].IsDir())
	require.True(t, files[0].Mode().IsRegular())
	require.True(t, strings.HasSuffix(files[0].Name(), ".db"))

	v, err := c[0].LoadVersion()
	require.Nil(t, err)
	require.Equal(t, 0, v)
	err = c[0].SaveVersion(1)
	require.Nil(t, err)
	v, err = c[0].LoadVersion()
	require.Nil(t, err)
	require.Equal(t, 1, v)
}

func testLoadSave(t *testing.T, c *Context) {
	key := []byte("test")
	cd := &ContextData{42, "meaning of life"}
	network.RegisterMessage(ContextData{})
	require.Nil(t, c.Save(key, cd))

	msg, err := c.Load(append(key, byte('_')))
	if err != nil || msg != nil {
		log.Fatal("this should not exist")
	}
	cdInt, err := c.Load(key)
	require.Nil(t, err)
	cd2, ok := cdInt.(*ContextData)
	if !ok {
		log.Fatal("contextData should exist")
	}

	cdBuf, err := c.LoadRaw(key)
	require.Nil(t, err)
	cdBuf2, err := network.Marshal(cd)
	require.EqualValues(t, cdBuf, cdBuf2)

	if cd.I != cd2.I || cd.S != cd2.S {
		log.Fatal("stored and loaded data should be equal", cd, cd2)
	}
}

func testSaveFailure(t *testing.T, c *Context) {
	key := []byte("test")
	cd := &ContextData{42, "meaning of life"}
	// should fail because ContextData is not registered
	if c.Save(key, cd) == nil {
		log.Fatal("Save should fail")
	}
}

func TestContext_GetAdditionalBucket(t *testing.T) {
	tmp, err := ioutil.TempDir("", "conode")
	log.ErrFatal(err)
	defer os.RemoveAll(tmp)

	c := createContext(t, tmp)
	db, name := c.GetAdditionalBucket([]byte("new"))
	require.NotNil(t, db)
	require.Equal(t, "testService_new", string(name))
	// Need to accept a second run with an existing bucket
	db, name = c.GetAdditionalBucket([]byte("new"))
	require.NotNil(t, db)
	require.Equal(t, "testService_new", string(name))
}

func TestContext_Path(t *testing.T) {
	tmp, err := ioutil.TempDir("", "conode")
	log.ErrFatal(err)
	defer os.RemoveAll(tmp)

	c := createContext(t, tmp)
	pub, _ := c.ServerIdentity().Public.MarshalBinary()
	h := sha256.New()
	h.Write(pub)
	dbPath := path.Join(tmp, fmt.Sprintf("%x.db", h.Sum(nil)))
	_, err = os.Stat(dbPath)
	if err != nil {
		t.Error(err)
	}
	os.Remove(dbPath)

	tmp, err = ioutil.TempDir("", "conode")
	log.ErrFatal(err)
	defer os.RemoveAll(tmp)

	c = createContext(t, tmp)

	_, err = os.Stat(tmp)
	log.ErrFatal(err)
	pub, _ = c.ServerIdentity().Public.MarshalBinary()
	h = sha256.New()
	h.Write(pub)
	_, err = os.Stat(path.Join(tmp, fmt.Sprintf("%x.db", h.Sum(nil))))
	log.ErrFatal(err)
}

// createContext creates the minimum number of things required for the test
func createContext(t *testing.T, dbPath string) *Context {
	kp := key.NewKeyPair(tSuite)
	si := network.NewServerIdentity(kp.Public,
		network.NewAddress(network.Local, "localhost:0"))
	cn := &Server{
		Router: &network.Router{
			ServerIdentity: si,
		},
	}

	name := "testService"
	RegisterNewService(name, func(c *Context) (Service, error) {
		return nil, nil
	})

	sm := &serviceManager{
		server: cn,
		dbPath: dbPath,
	}

	db, err := openDb(sm.dbFileName())
	require.Nil(t, err)

	err = db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucket([]byte(name))
		if err != nil {
			return xerrors.Errorf("creating bucket: %v", err)
		}
		return nil
	})
	require.Nil(t, err)
	sm.db = db

	return newContext(cn, nil, ServiceFactory.ServiceID(name), sm)
}
