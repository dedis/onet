package network

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/onet/v4/ciphersuite"
	"go.dedis.ch/protobuf"
)

func NewTestTLSHost(cr *ciphersuite.Registry, port int) (*TCPHost, error) {
	addr := NewTLSAddress("127.0.0.1:" + strconv.Itoa(port))
	pk, sk, err := testSuite.GenerateKeyPair(nil)
	if err != nil {
		return nil, err
	}
	e := NewServerIdentity(pk.Raw(), addr)
	e.SetPrivate(sk.Raw())
	return NewTCPHost(cr, e)
}

func NewTestRouterTLS(cr *ciphersuite.Registry, port int) (*Router, error) {
	h, err := NewTestTLSHost(cr, port)
	if err != nil {
		return nil, err
	}
	h.sid.Address = h.TCPListener.Address()
	r := NewRouter(h.sid, h)
	return r, nil
}

type hello struct {
	Hello string
	From  ServerIdentity
}

func TestTLS(t *testing.T) {
	testTLS(t, testRegistry)
}

func testTLS(t *testing.T, cr *ciphersuite.Registry) {
	r1, err := NewTestRouterTLS(cr, 0)
	require.Nil(t, err, "new tcp router")
	r2, err := NewTestRouterTLS(cr, 0)
	require.Nil(t, err, "new tcp router 2")

	ready := make(chan bool)
	stop := make(chan bool)
	rcv := make(chan bool, 1)

	mt := RegisterMessage(&hello{})
	r1.Dispatcher.RegisterProcessorFunc(mt, func(*Envelope) error {
		rcv <- true
		return nil
	})

	go func() {
		ready <- true
		r1.Start()
		stop <- true
	}()
	go func() {
		ready <- true
		r2.Start()
		stop <- true
	}()

	<-ready
	<-ready

	// We want these cleanups to happen if we leave by the require failing
	// or by the end of the function.
	defer func() {
		r1.Stop()
		r2.Stop()

		for i := 0; i < 2; i++ {
			select {
			case <-stop:
			case <-time.After(100 * time.Millisecond):
				t.Fatal("Could not stop router", i)
			}
		}
	}()

	// now send a message from r2 to r1
	sentLen, err := r2.Send(r1.ServerIdentity, &hello{
		Hello: "Howdy.",
		From:  *r2.ServerIdentity,
	})
	require.Nil(t, err, "Could not router.Send")
	require.NotZero(t, sentLen)

	<-rcv
}

func BenchmarkMsgTCP(b *testing.B) {
	r1, err := NewTestRouterTCP(testRegistry, 0)
	require.Nil(b, err, "new tcp router")
	r2, err := NewTestRouterTCP(testRegistry, 0)
	require.Nil(b, err, "new tcp router 2")
	benchmarkMsg(b, r1, r2)
}

func BenchmarkMsgTLS(b *testing.B) {
	r1, err := NewTestRouterTLS(testRegistry, 0)
	require.Nil(b, err, "new tls router")
	r2, err := NewTestRouterTLS(testRegistry, 0)
	require.Nil(b, err, "new tls router 2")
	benchmarkMsg(b, r1, r2)
}

func benchmarkMsg(b *testing.B, r1, r2 *Router) {
	mt := RegisterMessage(&hello{})
	r1.Dispatcher.RegisterProcessorFunc(mt, func(*Envelope) error {
		// Don't do anything. We are not interested in
		// benchmarking this work.
		return nil
	})

	ready := make(chan bool)
	stop := make(chan bool)

	go func() {
		ready <- true
		r1.Start()
		stop <- true
	}()
	go func() {
		ready <- true
		r2.Start()
		stop <- true
	}()

	<-ready
	<-ready

	// Setup is complete.
	b.ReportAllocs()
	b.ResetTimer()

	// Send one message from r2 to r1.
	for i := 0; i < b.N; i++ {
		_, err := r2.Send(r1.ServerIdentity, &hello{
			Hello: "Howdy.",
			From:  *r2.ServerIdentity,
		})
		if err != nil {
			b.Log("Could not router.Send")
		}
	}

	r1.Stop()
	r2.Stop()

	for i := 0; i < 2; i++ {
		select {
		case <-stop:
		case <-time.After(100 * time.Millisecond):
			b.Fatal("Could not stop router", i)
		}
	}
}

func Test_pubFromCN(t *testing.T) {
	pk, _, err := testSuite.GenerateKeyPair(nil)
	require.NoError(t, err)

	pkbuf, err := protobuf.Encode(pk.Raw())
	require.NoError(t, err)

	// old-style
	cn := string(pkbuf)

	p2, err := pubFromCN(cn)
	require.NoError(t, err)
	require.True(t, p2.Equal(pk.Raw()))

	// new-style
	cn, err = pubToCN(pk.Raw())
	require.NoError(t, err)

	p2, err = pubFromCN(cn)
	require.NoError(t, err)
	require.True(t, p2.Equal(pk.Raw()))
}
