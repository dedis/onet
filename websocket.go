package onet

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.dedis.ch/onet/v3/log"
	"go.dedis.ch/onet/v3/network"
	"golang.org/x/xerrors"
)

const certificateReloaderLeeway = 1 * time.Hour

// CertificateReloader takes care of reloading a TLS certificate when
// requested.
type CertificateReloader struct {
	sync.RWMutex
	cert     *tls.Certificate
	certPath string
	keyPath  string
}

// NewCertificateReloader takes two file paths as parameter that contain
// the certificate and the key data to create an automatic reloader. It will
// try to read again the files when the certificate is almost expired.
func NewCertificateReloader(certPath, keyPath string) (*CertificateReloader, error) {
	loader := &CertificateReloader{
		certPath: certPath,
		keyPath:  keyPath,
	}

	err := loader.reload()
	if err != nil {
		return nil, xerrors.Errorf("reloading certificate: %v", err)
	}

	return loader, nil
}

func (cr *CertificateReloader) reload() error {
	newCert, err := tls.LoadX509KeyPair(cr.certPath, cr.keyPath)
	if err != nil {
		return xerrors.Errorf("load x509: %v", err)
	}

	cr.Lock()
	cr.cert = &newCert
	// Successful parse means at least one certificate.
	cr.cert.Leaf, err = x509.ParseCertificate(newCert.Certificate[0])
	cr.Unlock()

	if err != nil {
		return xerrors.Errorf("parse x509: %v", err)
	}
	return nil
}

// GetCertificateFunc makes a function that can be passed to the TLSConfig
// so that it resolves the most up-to-date one.
func (cr *CertificateReloader) GetCertificateFunc() func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return func(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		cr.RLock()

		exp := time.Now().Add(certificateReloaderLeeway)

		// Here we know the leaf has been parsed successfully as an error
		// would have been thrown otherwise.
		if cr.cert == nil || exp.After(cr.cert.Leaf.NotAfter) {
			// Certificate has expired so we try to load the new one.

			// Free the read lock to be able to reload.
			cr.RUnlock()
			err := cr.reload()
			if err != nil {
				return nil, xerrors.Errorf("reload certificate: %v", err)
			}

			cr.RLock()
		}

		defer cr.RUnlock()
		return cr.cert, nil
	}
}

// WebSocket handles incoming client-requests using the websocket
// protocol. When making a new WebSocket, it will listen one port above the
// ServerIdentity-port-#.
// The websocket protocol has been chosen as smallest common denominator
// for languages including JavaScript.
type WebSocket struct {
	services  map[string]Service
	server    *http.Server
	mux       *http.ServeMux
	startstop chan bool
	started   bool
	TLSConfig *tls.Config // can only be modified before Start is called
	sync.Mutex
}

// NewWebSocket opens a webservice-listener at the given si.URL.
func NewWebSocket(si *network.ServerIdentity) *WebSocket {
	w := &WebSocket{
		services:  make(map[string]Service),
		startstop: make(chan bool),
	}
	webHost, err := getWSHostPort(si, true)
	log.ErrFatal(err)
	w.mux = http.NewServeMux()
	w.mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
		log.Lvl4("ok?", r.RemoteAddr)
		ok := []byte("ok\n")
		w.Write(ok)
	})

	if allowPprof() {
		log.Warn("HTTP pprof profiling is enabled")
		initPprof(w.mux)
	}

	// Add a catch-all handler (longest paths take precedence, so "/" takes
	// all non-registered paths) and correctly upgrade to a websocket and
	// throw an error.
	w.mux.HandleFunc("/", func(wr http.ResponseWriter, re *http.Request) {
		log.Error("request from ", re.RemoteAddr, "for invalid path ", re.URL.Path)

		u := websocket.Upgrader{
			// The mobile app on iOS doesn't support compression well...
			EnableCompression: false,
			// As the website will not be served from ourselves, we
			// need to accept _all_ origins. Cross-site scripting is
			// required.
			CheckOrigin: func(*http.Request) bool {
				return true
			},
		}
		ws, err := u.Upgrade(wr, re, http.Header{})
		if err != nil {
			log.Error(err)
			return
		}

		ws.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(4001, "This service doesn't exist"),
			time.Now().Add(time.Millisecond*500))
		ws.Close()
	})
	w.server = &http.Server{
		Addr:    webHost,
		Handler: w.mux,
	}
	return w
}

// Listening returns true if the server has been started and is
// listening on the ports for incoming connections.
func (w *WebSocket) Listening() bool {
	w.Lock()
	defer w.Unlock()
	return w.started
}

// start listening on the port.
func (w *WebSocket) start() {
	w.Lock()
	w.started = true
	w.server.TLSConfig = w.TLSConfig
	log.Lvl2("Starting to listen on", w.server.Addr)
	started := make(chan bool)
	go func() {
		// Check if server is configured for TLS
		started <- true
		if w.server.TLSConfig != nil && (w.server.TLSConfig.GetCertificate != nil || len(w.server.TLSConfig.Certificates) >= 1) {
			w.server.ListenAndServeTLS("", "")
		} else {
			w.server.ListenAndServe()
		}
	}()
	<-started
	w.Unlock()
	w.startstop <- true
}

// registerService stores a service to the given path. All requests to that
// path and it's sub-endpoints will be forwarded to ProcessClientRequest.
func (w *WebSocket) registerService(service string, s Service) error {
	if service == "ok" {
		return xerrors.New("service name \"ok\" is not allowed")
	}

	w.services[service] = s
	h := &wsHandler{
		service:     s,
		serviceName: service,
	}
	w.mux.Handle(fmt.Sprintf("/%s/", service), h)
	return nil
}

// stop the websocket and free the port.
func (w *WebSocket) stop() {
	w.Lock()
	defer w.Unlock()
	if !w.started {
		return
	}
	log.Lvl3("Stopping", w.server.Addr)

	d := time.Now().Add(100 * time.Millisecond)
	ctx, cancel := context.WithDeadline(context.Background(), d)
	w.server.Shutdown(ctx)
	cancel()

	<-w.startstop
	w.started = false
}

// Pass the request to the websocket.
type wsHandler struct {
	serviceName string
	service     Service
}

// Wrapper-function so that http.Requests get 'upgraded' to websockets
// and handled correctly.
func (t wsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rx := 0
	tx := 0
	n := 0

	defer func() {
		log.Lvl2("ws close", r.RemoteAddr, "n", n, "rx", rx, "tx", tx)
	}()

	u := websocket.Upgrader{
		// The mobile app on iOS doesn't support compression well...
		EnableCompression: false,
		// As the website will not be served from ourselves, we
		// need to accept _all_ origins. Cross-site scripting is
		// required.
		CheckOrigin: func(*http.Request) bool {
			return true
		},
	}
	ws, err := u.Upgrade(w, r, http.Header{})
	if err != nil {
		log.Error(err)
		return
	}
	defer ws.Close()

	// Loop for each message
outerReadLoop:
	for err == nil {
		mt, buf, rerr := ws.ReadMessage()
		if rerr != nil {
			err = rerr
			break
		}
		rx += len(buf)
		n++

		s := t.service
		var reply []byte
		var outChan chan []byte
		path := strings.TrimPrefix(r.URL.Path, "/"+t.serviceName+"/")
		log.Lvlf2("ws request from %s: %s/%s", r.RemoteAddr, t.serviceName, path)

		isStreaming := false
		bidirectionalStreamer, ok := s.(BidirectionalStreamer)
		if ok {
			isStreaming, err = bidirectionalStreamer.IsStreaming(path)
			if err != nil {
				log.Errorf("failed to check if it is a streaming "+
					"request %s/%s: %+v", t.serviceName, path, err)
				continue
			}
		}

		if !isStreaming {
			reply, _, err = s.ProcessClientRequest(r, path, buf)
			if err != nil {
				log.Errorf("Got an error while executing %s/%s: %+v",
					t.serviceName, path, err)
				continue
			}

			tx += len(reply)
			err = ws.SetWriteDeadline(time.Now().Add(5 * time.Minute))
			if err != nil {
				log.Error(xerrors.Errorf("failed to set the write deadline "+
					"with request request %s/%s: %v", t.serviceName, path, err))
				break
			}

			err = ws.WriteMessage(mt, reply)
			if err != nil {
				log.Error(xerrors.Errorf("failed to write message with "+
					"request %s/%s: %v", t.serviceName, path, err))
				break
			}

			continue
		}

		clientInputs := make(chan []byte, 10)
		clientInputs <- buf
		outChan, err = bidirectionalStreamer.ProcessClientStreamRequest(r,
			path, clientInputs)
		if err != nil {
			log.Errorf("got an error while processing streaming "+
				"request %s/%s: %+v", t.serviceName, path, err)
			continue
		}

		closing := make(chan bool)
		go func() {
			for {
				// Listen for incoming messages to know if the client wants to
				// close the stream. If this is an error, we assume the client
				// wants to close the stream, otherwise we forward the message
				// to the service.
				_, buf, err := ws.ReadMessage()
				if err != nil {
					close(closing)
					return
				}
				clientInputs <- buf
			}
		}()

		for {
			select {
			case <-closing:
				close(clientInputs)
				break outerReadLoop
			case reply, ok := <-outChan:
				if !ok {
					ws.WriteControl(websocket.CloseMessage,
						websocket.FormatCloseMessage(websocket.CloseNormalClosure, "service finished streaming"),
						time.Now().Add(time.Millisecond*500))
					close(clientInputs)
					return
				}
				tx += len(reply)

				err = ws.SetWriteDeadline(time.Now().Add(5 * time.Minute))
				if err != nil {
					log.Error(xerrors.Errorf("failed to set the write "+
						"deadline in the streaming loop: %v", err))
					close(clientInputs)
					break outerReadLoop
				}

				err = ws.WriteMessage(mt, reply)
				if err != nil {
					log.Error(xerrors.Errorf("failed to write next message "+
						"in the streaming loop: %v", err))
					close(clientInputs)
					break outerReadLoop
				}
			}
		}

	}

	errMessage := "unexpected error: "
	if err != nil {
		errMessage += err.Error()
	}

	ws.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseProtocolError, errMessage),
		time.Now().Add(time.Millisecond*500))
	return
}

type destination struct {
	si   *network.ServerIdentity
	path string
}
