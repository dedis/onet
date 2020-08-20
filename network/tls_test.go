package network

import (
	"log"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/kyber/v3/suites"
	"go.dedis.ch/kyber/v3/util/key"
)

func NewTestTLSHost(suite suites.Suite, port int) (*TCPHost, error) {
	addr := NewTLSAddress("127.0.0.1:" + strconv.Itoa(port))
	kp := key.NewKeyPair(suite)
	e := NewServerIdentity(kp.Public, addr)
	e.SetPrivate(kp.Private)
	return NewTCPHost(e, suite)
}

func NewTestRouterTLS(suite suites.Suite, port int) (*Router, error) {
	h, err := NewTestTLSHost(suite, port)
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
	testTLS(t, tSuite, false, false)
}

func TestTLS_bn256(t *testing.T) {
	s := suites.MustFind("bn256.g2")
	testTLS(t, s, false, false)
}

func TestTLS_noURIs(t *testing.T) {
	testTLS(t, tSuite, true, false)
	testTLS(t, tSuite, true, true)
	testTLS(t, tSuite, false, true)
}

func testTLS(t *testing.T, s suites.Suite, noURIs1, noURIs2 bool) {
	log.Print(noURIs1, noURIs2)
	// Clean up changes we might make in this test.
	defer func() {
		testNoURIs = false
	}()

	// R1 has URI-based handshakes unconditionally.
	testNoURIs = noURIs1
	r1, err := NewTestRouterTLS(s, 0)
	require.Nil(t, err, "new tcp router")

	// R2 might have no URIs, in order to simulate old handshake to new handshake
	// compatibility.
	testNoURIs = noURIs2
	r2, err := NewTestRouterTLS(s, 0)
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
	r1, err := NewTestRouterTCP(0)
	require.Nil(b, err, "new tcp router")
	r2, err := NewTestRouterTCP(0)
	require.Nil(b, err, "new tcp router 2")
	benchmarkMsg(b, r1, r2)
}

func BenchmarkMsgTLS(b *testing.B) {
	r1, err := NewTestRouterTLS(tSuite, 0)
	require.Nil(b, err, "new tls router")
	r2, err := NewTestRouterTLS(tSuite, 0)
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
	p1 := tSuite.Point().Pick(tSuite.RandomStream())

	// old-style
	cn := p1.String()

	p2, err := pubFromCN(tSuite, cn)
	require.NoError(t, err)
	require.True(t, p2.Equal(p1))

	// new-style
	cn = pubToCN(p1)

	p2, err = pubFromCN(tSuite, cn)
	require.NoError(t, err)
	require.True(t, p2.Equal(p1))
}
