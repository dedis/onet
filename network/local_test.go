package network

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

func TestLocalRouter(t *testing.T) {
	addr := &ServerIdentity{Address: NewLocalAddress("127.0.0.1:2000")}
	wrongAddr1 := &ServerIdentity{Address: NewTCPAddress(addr.Address.NetworkAddress())}
	_, err := NewLocalRouter(wrongAddr1)
	if err == nil {
		t.Error("Should have returned something..")
	}
	_, err = NewLocalRouter(addr)
	if err != nil {
		t.Error("Should not have returned something")
	}

}

func TestLocalListener(t *testing.T) {
	addr := NewLocalAddress("127.0.0.1:2000")
	wrongAddr1 := NewTCPAddress(addr.NetworkAddress())
	listener, err := NewLocalListener(wrongAddr1)
	if err == nil {
		t.Error("Create listener with wrong address should fail")
	}
	defaultLocalManager.setListening(addr, func(c Conn) {})
	listener, err = NewLocalListener(addr)
	if err == nil {
		t.Error("Create listener with already binded address should fail")
	}
	LocalReset()

	listener, err = NewLocalListener(addr)
	if err != nil {
		t.Fatal(err)
	}

	var ready = make(chan bool)
	go func() {
		ready <- true
		err := listener.Listen(func(c Conn) {})
		if err != nil {
			t.Error("Should not have had error while listening")
		}
		ready <- true
	}()

	<-ready
	// give it some time
	time.Sleep(20 * time.Millisecond)
	if err := listener.Listen(func(c Conn) {}); err == nil {
		t.Error("listener should have returned an error when Listen twice")
	}
	assert.Nil(t, listener.Stop())
	if err := listener.Stop(); err != nil {
		t.Error("listener.Stop() twice should not returns an error")
	}
	<-ready
}

// Test whether a call to a conn.Close() will stop the remote Receive() call
func TestLocalConnCloseReceive(t *testing.T) {
	addr := NewLocalAddress("127.0.0.1:2000")
	listener, err := NewLocalListener(addr)
	if err != nil {
		t.Fatal("Could not listen", err)
	}

	var ready = make(chan bool)
	go func() {
		ready <- true
		listener.Listen(func(c Conn) {
			ready <- true
			assert.Nil(t, c.Close())
			ready <- true
		})
	}()
	<-ready

	outgoing, err := NewLocalConnWithManager(defaultLocalManager, addr, addr)
	if err != nil {
		t.Fatal("erro NewLocalConn:", err)
	}
	if outgoing.Type() != Local {
		t.Error("Wrong type for Conn?")
	}
	if outgoing.Local() != addr {
		t.Error("Wrong local addr for Conn!?")
	}
	<-ready
	<-ready
	_, err = outgoing.Receive()
	assert.True(t, xerrors.Is(err, ErrClosed))
	assert.True(t, xerrors.Is(outgoing.Close(), ErrClosed))
	assert.Nil(t, listener.Stop())
}

// Test if we can run two parallel local network using two different contexts
func TestLocalContext(t *testing.T) {
	ctx1 := NewLocalManager()
	ctx2 := NewLocalManager()

	addrListener := NewLocalAddress("127.0.0.1:2000")
	addrConn := NewLocalAddress("127.0.0.1:2001")
	siListener, err := NewTestServerIdentity(addrListener)
	require.NoError(t, err)
	siConn, err := NewTestServerIdentity(addrConn)
	require.NoError(t, err)

	done1 := make(chan error)
	done2 := make(chan error)

	go testConnListener(ctx1, done1, siListener, siConn, 1)
	go testConnListener(ctx2, done2, siListener, siConn, 2)

	var confirmed int
	for confirmed != 2 {
		var err error
		select {
		case err = <-done1:
		case err = <-done2:
		}

		if err != nil {
			t.Fatal(err)
		}
		confirmed++
	}
}

// launch a listener, then a Conn and communicate their own address + individual
// val
func testConnListener(ctx *LocalManager, done chan error, listenA, connA *ServerIdentity, secret int) {
	listener, err := NewLocalListenerWithManager(ctx, listenA.Address)
	if err != nil {
		done <- err
		return
	}

	var ok = make(chan error)

	// make the listener send and receive a struct that only they can know (this
	// listener + conn
	handshake := func(c Conn, sending, receiving Address) error {
		sentLen, err := c.Send(&AddressTest{sending, int64(secret)})
		if err != nil {
			return xerrors.Errorf("sending: %v", err)
		}
		if sentLen == 0 {
			return xerrors.Errorf("sentLen is zero")
		}

		p, err := c.Receive()
		if err != nil {
			return err
		}

		at := p.Msg.(*AddressTest)
		if at.Addr != receiving {
			return xerrors.New("Receiveid wrong address")
		}
		if at.Val != int64(secret) {
			return xerrors.New("Received wrong secret")
		}
		return nil
	}

	go func() {
		ok <- nil
		listener.Listen(func(c Conn) {
			ok <- nil
			err := handshake(c, listenA.Address, connA.Address)
			ok <- err
		})
		ok <- nil
	}()
	// wait go routine to start
	<-ok

	// trick to use host because it already tries multiple times to connect if
	// the listening routine is not up yet.
	h, err := NewLocalHostWithManager(ctx, connA.Address)
	if err != nil {
		done <- err
		return
	}
	c, err := h.Connect(listenA)
	if err != nil {
		done <- err
		return
	}

	// wait listening function to start for the incoming conn
	<-ok
	err = handshake(c, connA.Address, listenA.Address)
	if err != nil {
		done <- err
		return
	}
	// wait for any err of the handshake from the listening PoV
	err = <-ok
	if err != nil {
		done <- err
		return
	}
	if err := c.Close(); err != nil {
		done <- err
		return
	}
	listener.Stop()
	<-ok
	done <- nil
}

func TestLocalConnDiffAddress(t *testing.T) {
	testLocalConn(t, NewLocalAddress("127.0.0.1:2000"), NewLocalAddress("127.0.0.1:2001"))
}

func TestLocalConnSameAddress(t *testing.T) {
	testLocalConn(t, NewLocalAddress("127.0.0.1:2000"), NewLocalAddress("127.0.0.1:2000"))
}

func testLocalConn(t *testing.T, a1, a2 Address) {
	addr1 := a1
	addr2 := a2

	listener, err := NewLocalListener(addr1)
	if err != nil {
		t.Fatal("Could not listen", err)
	}

	var ready = make(chan bool)
	var incomingConn = make(chan bool)
	var outgoingConn = make(chan bool)
	go func() {
		ready <- true
		listener.Listen(func(c Conn) {
			incomingConn <- true
			nm, err := c.Receive()
			assert.Nil(t, err)
			assert.Equal(t, int64(3), nm.Msg.(*SimpleMessage).I)
			// acknoledge the message
			incomingConn <- true
			sentLen, err := c.Send(&SimpleMessage{3})
			assert.Nil(t, err)
			assert.NotZero(t, sentLen)
			//wait ack
			<-outgoingConn
			assert.Equal(t, 2, listener.manager.count())
			// close connection
			assert.Nil(t, c.Close())
			incomingConn <- true

		})
		ready <- true
	}()
	<-ready

	outgoing, err := NewLocalConnWithManager(defaultLocalManager, addr2, addr1)
	if err != nil {
		t.Fatal("erro NewLocalConn:", err)
	}

	// check if connection is opened on the listener
	<-incomingConn
	// send stg and wait for ack
	sentLen, err := outgoing.Send(&SimpleMessage{3})
	assert.Nil(t, err)
	assert.NotZero(t, sentLen)
	<-incomingConn

	// receive stg and send ack
	nm, err := outgoing.Receive()
	assert.Nil(t, err)
	assert.Equal(t, int64(3), nm.Msg.(*SimpleMessage).I)
	outgoingConn <- true

	<-incomingConn
	// close the incoming conn, so Receive here should return an error
	nm, err = outgoing.Receive()
	if !xerrors.Is(err, ErrClosed) {
		t.Error("Receive should have returned an error")
	}
	assert.True(t, xerrors.Is(outgoing.Close(), ErrClosed))
	// close the listener
	assert.Nil(t, listener.Stop())
	<-ready
}

func TestLocalManyConn(t *testing.T) {
	nbrConn := 3
	addr := NewLocalAddress("127.0.0.1:2000")
	listener, err := NewLocalListener(addr)
	if err != nil {
		t.Fatal("Could not setup listener:", err)
	}
	var wg sync.WaitGroup
	go func() {
		listener.Listen(func(c Conn) {
			_, err := c.Receive()
			assert.Nil(t, err)
			sentLen, err := c.Send(&SimpleMessage{3})
			assert.Nil(t, err)
			assert.NotZero(t, sentLen)
		})
	}()

	if !waitListeningUp(addr) {
		t.Fatal("Can't get listener up")
	}
	wg.Add(nbrConn)
	for i := 1; i <= nbrConn; i++ {
		go func(j int) {
			a := NewLocalAddress("127.0.0.1:" + strconv.Itoa(2000+j))
			c, err := NewLocalConnWithManager(defaultLocalManager, a, addr)
			if err != nil {
				t.Fatal(err)
			}
			sentLen, err := c.Send(&SimpleMessage{3})
			assert.Nil(t, err)
			assert.NotZero(t, sentLen)
			nm, err := c.Receive()
			assert.Nil(t, err)
			assert.Equal(t, int64(3), nm.Msg.(*SimpleMessage).I)
			assert.Nil(t, c.Close())
			wg.Done()
		}(i)
	}

	wg.Wait()
	listener.Stop()
}

func TestDefaultLocalManager(t *testing.T) {
	defaultLocalManager.setListening(NewLocalAddress("127.0.0.1"), func(c Conn) {})
	assert.Equal(t, 1, len(defaultLocalManager.listening))
	LocalReset()
	assert.Equal(t, 0, len(defaultLocalManager.listening))
}

func waitListeningUp(addr Address) bool {
	for i := 0; i < 5; i++ {
		if defaultLocalManager.isListening(addr) {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

func NewTestLocalHost(port int) (*LocalHost, error) {
	addr := NewLocalAddress("127.0.0.1:" + strconv.Itoa(port))
	return NewLocalHost(addr)
}

type AddressTest struct {
	Addr Address
	Val  int64
}

var AddressTestType = RegisterMessage(&AddressTest{})
