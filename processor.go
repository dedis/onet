package onet

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"go.dedis.ch/onet/v3/log"
	"go.dedis.ch/onet/v3/network"
	"go.dedis.ch/protobuf"
	"golang.org/x/xerrors"
)

// ServiceProcessor allows for an easy integration of external messages
// into the Services. You have to embed it into your Service-struct as
// a pointer. It will process client requests that have been registered
// with RegisterMessage.
type ServiceProcessor struct {
	handlers map[string]serviceHandler
	*Context
}

// serviceHandler stores the handler and the message-type.
type serviceHandler struct {
	handler   interface{}
	msgType   reflect.Type
	streaming bool
}

// NewServiceProcessor initializes your ServiceProcessor.
func NewServiceProcessor(c *Context) *ServiceProcessor {
	return &ServiceProcessor{
		handlers: make(map[string]serviceHandler),
		Context:  c,
	}
}

var errType = reflect.TypeOf((*error)(nil)).Elem()

// RegisterHandler will store the given handler that will be used by the service.
// WebSocket will then forward requests to "ws://service_name/struct_name"
// to the given function f, which must be in the following form:
// func(msg interface{})(ret interface{}, err error)
//
//  * msg is a pointer to a structure to the message sent.
//  * ret is a pointer to a struct of the return-message.
//  * err is an error, it can be nil, or any type that implements error.
//
// struct_name is stripped of its package-name, so a structure like
// network.Body will be converted to Body.
func (p *ServiceProcessor) RegisterHandler(f interface{}) error {
	if err := handlerInputCheck(f); err != nil {
		return xerrors.Errorf("input check: %v", err)
	}

	pm, sh, err := createServiceHandler(f)
	if err != nil {
		return xerrors.Errorf("creating handler: %v", err)
	}
	p.handlers[pm] = sh

	return nil
}

// RegisterStreamingHandler stores a handler that is responsible for streaming
// messages to the client via a channel. Websocket will accept requests for
// this handler at "ws://service_name/struct_name", where struct_name is
// argument of f, which must be in the form:
// func(msg interface{})(retChan chan interface{}, closeChan chan bool, err error)
//
//  * msg is a pointer to a structure to the message sent.
//  * retChan is a channel of a pointer to a struct, everything sent into this
//    channel will be forwarded to the client, if there are no more messages,
//    the service should close retChan.
//  * closeChan is a boolean channel, upon receiving a message on this channel,
//    the handler must stop sending messages and close retChan.
//  * err is an error, it can be nil, or any type that implements error.
//
// struct_name is stripped of its package-name, so a structure like
// network.Body will be converted to Body.
func (p *ServiceProcessor) RegisterStreamingHandler(f interface{}) error {
	if err := handlerInputCheck(f); err != nil {
		return err
	}

	// check output
	ft := reflect.TypeOf(f)
	if ft.NumOut() != 3 {
		return xerrors.New("Need 3 return values: chan interface{}, chan bool and error")
	}
	// first output
	ret0 := ft.Out(0)
	if ret0.Kind() != reflect.Chan {
		return xerrors.New("1st return value must be a channel")
	}
	if ret0.Elem().Kind() != reflect.Interface {
		if ret0.Elem().Kind() != reflect.Ptr {
			return xerrors.New("1st return value must be a channel of a *pointer* to a struct")
		}
		if ret0.Elem().Elem().Kind() != reflect.Struct {
			return xerrors.New("1st return value must be a channel of a pointer to a *struct*")
		}
	}
	// second output
	ret1 := ft.Out(1)
	if ret1.Kind() != reflect.Chan {
		return xerrors.New("2nd return value must be a channel")
	}
	if ret1.Elem().Kind() != reflect.Bool {
		return xerrors.New("2nd return value must be a boolean channel")
	}
	// third output
	if !ft.Out(2).Implements(errType) {
		return xerrors.New("3rd return value has to implement error, but is: " +
			ft.Out(2).String())
	}

	cr := ft.In(0)
	log.Lvl4("Registering streaming handler", cr.String())
	pm := strings.Split(cr.Elem().String(), ".")[1]
	p.handlers[pm] = serviceHandler{f, cr.Elem(), true}

	return nil
}

// getRouter returns the gorilla mutiplexing router. If we need to support
// arbitrary registration of REST API, we could make this method public.
func (p *ServiceProcessor) getRouter() *http.ServeMux {
	return p.server.WebSocket.mux
}

type kindGET int

const (
	invalidGET kindGET = iota
	emptyGET
	intGET
	sliceGET
)

// prepareHandlerGET check whether the first argument of f has any fields; if
// it does then make sure the number of fields is either 0 or 1; if there is 1
// field then it has to be an int or a slice of bytes.
func prepareHandlerGET(f interface{}) (kindGET, string, error) {
	in0 := reflect.TypeOf(f).In(0).Elem()
	if in0.Kind() != reflect.Struct {
		return invalidGET, "", xerrors.New("input argument must be a struct")
	}
	if in0.NumField() == 0 {
		return emptyGET, "", nil
	} else if in0.NumField() == 1 {
		// we support int and byte slices only
		if in0.Field(0).Type.Kind() == reflect.Slice && in0.Field(0).Type.Elem().Kind() == reflect.Uint8 {
			return sliceGET, in0.Field(0).Name, nil
		} else if in0.Field(0).Type.Kind() == reflect.Int {
			return intGET, in0.Field(0).Name, nil
		}
		return invalidGET, "", xerrors.New("only byte slices and int are supported")
	}
	return invalidGET, "", xerrors.New("number of fields must be 0 or 1")
}

// RegisterRESTHandler takes a callback of type
// func(msg interface{})(ret interface{}, err error),
// where msg and ret must be pointers to structs.
// For POST and PUT, the callback is registered on the URL
// /v$version/$namespace/$msgStructName. The client should serialize the request
// using JSON and set the conent type to application/json to use the service.
// The response is also JSON encoded.
//
// For GET requests, the callback is registered on the same URL. But clients
// can also query individual resources such as
// /v$version/$namespace/$msgStructName/$id. For this to work, msg in the
// callback must be a singleton struct with either an integer or a byte slice.
// For integers, the client can directly query the integer resource, for byte
// slices, the clients must query the hex encoded representation. Using an
// empty struct for msg is also supported.
//
// The min/maxVersion argument represents the range of versions where the API
// is present. If breaking changes must be made then they must use a new
// version.
//
// This method is experimental.
func (p *ServiceProcessor) RegisterRESTHandler(f interface{}, namespace, method string, minVersion, maxVersion int) error {
	// TODO support more methods
	if method != "GET" && method != "POST" && method != "PUT" {
		return xerrors.New("invalid REST method")
	}
	if minVersion > maxVersion {
		return xerrors.New("min version is greater than max version")
	}
	if minVersion < 3 {
		return xerrors.New("earliest supported API level must be greater or equal to 3")
	}
	if err := handlerInputCheck(f); err != nil {
		return xerrors.Errorf("input check: %v", err)
	}
	resource, sh, err := createServiceHandler(f)
	if err != nil {
		return xerrors.Errorf("creating handler: %v", err)
	}
	var k kindGET
	if method == "GET" {
		k, _, err = prepareHandlerGET(f)
		if err != nil {
			return xerrors.Errorf("preparing get handler: %v", err)
		}
	}

	intRegex, err := regexp.Compile(fmt.Sprintf(`^/v\d/%s/%s/\d+$`, namespace, resource))
	if err != nil {
		return xerrors.Errorf("regex: %v", err)
	}
	sliceRegex, err := regexp.Compile(fmt.Sprintf(`^/v\d/%s/%s/[0-9a-f]+$`, namespace, resource))
	if err != nil {
		return xerrors.Errorf("regex: %v", err)
	}
	val0 := reflect.New(sh.msgType)

	h := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			http.Error(w, wrapJSONMsg("unsupported method: "+r.Method), http.StatusMethodNotAllowed)
			return
		}
		var msgBuf []byte
		switch r.Method {
		case "GET":
			switch k {
			case emptyGET:
				msgBuf = []byte("{}")
			case intGET:
				if ok := intRegex.MatchString(r.URL.EscapedPath()); !ok {
					http.Error(w, wrapJSONMsg("invalid path"), http.StatusNotFound)
					return
				}
				_, num := path.Split(r.URL.EscapedPath())
				numI64, err := strconv.Atoi(num)
				if err != nil {
					http.Error(w, wrapJSONMsg("not a number"), http.StatusBadRequest)
					return
				}
				val0.Elem().Field(0).SetInt(int64(numI64))
			case sliceGET:
				if ok := sliceRegex.MatchString(r.URL.EscapedPath()); !ok {
					http.Error(w, wrapJSONMsg("invalid path"), http.StatusNotFound)
					return
				}
				_, hexStr := path.Split(r.URL.EscapedPath())
				byteBuf, err := hex.DecodeString(hexStr)
				if err != nil {
					http.Error(w, wrapJSONMsg(err.Error()), http.StatusBadRequest)
					return
				}
				val0.Elem().Field(0).SetBytes(byteBuf)
			default:
				http.Error(w, wrapJSONMsg("invalid GET"), http.StatusBadRequest)
				return
			}
		case "POST", "PUT":
			if r.Header.Get("Content-Type") != "application/json" {
				http.Error(w, wrapJSONMsg("content type needs to be application/json"), http.StatusBadRequest)
				return
			}
			var err error
			msgBuf, err = ioutil.ReadAll(r.Body)
			if err != nil {
				http.Error(w, wrapJSONMsg(err.Error()), http.StatusBadRequest)
				return
			}
			if err := json.Unmarshal(msgBuf, val0.Interface()); err != nil {
				http.Error(w, wrapJSONMsg("decoding error "+err.Error()), http.StatusBadRequest)
				return
			}
		default:
			http.Error(w, wrapJSONMsg("unsupported method: "+r.Method), http.StatusMethodNotAllowed)
			return
		}

		out, tun, err := callInterfaceFunc(f, val0.Interface(), false)
		if err != nil {
			http.Error(w, wrapJSONMsg("processing error "+err.Error()), http.StatusBadRequest)
			return
		}
		if tun != nil {
			http.Error(w, wrapJSONMsg("streaming requests are not supported"), http.StatusBadRequest)
			return
		}
		reply, err := json.Marshal(out)
		if err != nil {
			http.Error(w, wrapJSONMsg(err.Error()), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(reply)
	}
	finalSlash := ""
	if k == intGET || k == sliceGET {
		finalSlash = "/"
	}
	for v := minVersion; v <= maxVersion; v++ {
		p.getRouter().HandleFunc(fmt.Sprintf("/v%d/%s/%s", v, namespace, resource)+finalSlash, h)
	}
	return nil
}

func wrapJSONMsg(s string) string {
	return fmt.Sprintf(`{"message": "%s"}`, s)
}

func createServiceHandler(f interface{}) (string, serviceHandler, error) {
	// check output
	ft := reflect.TypeOf(f)
	if ft.NumOut() != 2 {
		return "", serviceHandler{}, xerrors.New("Need 2 return values: network.Body and error")
	}
	// first output
	ret := ft.Out(0)
	if ret.Kind() != reflect.Interface {
		if ret.Kind() != reflect.Ptr {
			return "", serviceHandler{},
				xerrors.New("1st return value must be a *pointer* to a struct or an interface")
		}
		if ret.Elem().Kind() != reflect.Struct {
			return "", serviceHandler{},
				xerrors.New("1st return value must be a pointer to a *struct* or an interface")
		}
	}
	// second output
	if !ft.Out(1).Implements(errType) {
		return "", serviceHandler{},
			xerrors.New("2nd return value has to implement error, but is: " + ft.Out(1).String())
	}

	cr := ft.In(0)
	log.Lvl4("Registering handler", cr.String())
	pm := strings.Split(cr.Elem().String(), ".")[1]

	return pm, serviceHandler{f, cr.Elem(), false}, nil
}

func handlerInputCheck(f interface{}) error {
	ft := reflect.TypeOf(f)
	if ft.Kind() != reflect.Func {
		return xerrors.New("Input is not a function")
	}
	if ft.NumIn() != 1 {
		return xerrors.New("Need one argument: *struct")
	}
	cr := ft.In(0)
	if cr.Kind() != reflect.Ptr {
		return xerrors.New("Argument must be a *pointer* to a struct")
	}
	if cr.Elem().Kind() != reflect.Struct {
		return xerrors.New("Argument must be a pointer to *struct*")
	}
	return nil
}

// RegisterHandlers takes a vararg of messages to register and returns
// the first error encountered or nil if everything was OK.
func (p *ServiceProcessor) RegisterHandlers(procs ...interface{}) error {
	for _, pr := range procs {
		if err := p.RegisterHandler(pr); err != nil {
			return xerrors.Errorf("registering handler: %v", err)
		}
	}
	return nil
}

// RegisterStreamingHandlers takes a vararg of messages to register and returns
// the first error encountered or nil if everything was OK.
func (p *ServiceProcessor) RegisterStreamingHandlers(procs ...interface{}) error {
	for _, pr := range procs {
		if err := p.RegisterStreamingHandler(pr); err != nil {
			return xerrors.Errorf("registering handler: %v", err)
		}
	}
	return nil
}

// Process implements the Processor interface and dispatches ClientRequest
// messages.
func (p *ServiceProcessor) Process(env *network.Envelope) {
	log.Panic("Cannot handle message.")
}

// NewProtocol is a stub for services that don't want to intervene in the
// protocol-handling.
func (p *ServiceProcessor) NewProtocol(tn *TreeNodeInstance, conf *GenericConfig) (ProtocolInstance, error) {
	return nil, nil
}

// StreamingTunnel is used as a tunnel between service processor and its
// caller, usually the websocket read-loop. When the tunnel is returned to the
// websocket loop, it should read from the out channel and forward the content
// to the client. If the client is disconnected, then the close channel should
// be closed. The signal exists to notify the service to stop streaming.
type StreamingTunnel struct {
	out   chan []byte
	close chan bool
}

func callInterfaceFunc(handler, input interface{}, streaming bool) (intf interface{}, ch chan bool, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = xerrors.Errorf("panic with %v", r)
		}
	}()

	to := reflect.TypeOf(handler).In(0)
	f := reflect.ValueOf(handler)

	arg := reflect.New(to.Elem())
	arg.Elem().Set(reflect.ValueOf(input).Elem())
	ret := f.Call([]reflect.Value{arg})

	if streaming {
		ierr := ret[2].Interface()
		if ierr != nil {
			err = xerrors.Errorf("processing error: %v", ierr)
			return
		}

		intf = ret[0].Interface()
		ch = ret[1].Interface().(chan bool)
		return
	}
	ierr := ret[1].Interface()
	if ierr != nil {
		err = xerrors.Errorf("processing error: %v", ierr)
		return
	}

	intf = ret[0].Interface()
	return
}

// ProcessClientStreamRequest allows clients to push multiple messages
// asynchronously to the same service with the same connection. Unlike in
// ProcessClientRequest, we take a channel of inputs that can be filled and
// will subsequently call the service with any new messages received in the
// channel. The caller is responsible for closing the client input channel when
// it is done.
func (p *ServiceProcessor) ProcessClientStreamRequest(req *http.Request, path string,
	clientInputs chan []byte) ([]byte, chan []byte, error) {

	outChan := make(chan []byte, 100)
	buf := <-clientInputs
	mh, ok := p.handlers[path]

	if !ok {
		err := xerrors.New("the requested message hasn't been " +
			"registered: " + path)
		log.Error(err)
		return nil, nil, err
	}

	msg := reflect.New(mh.msgType).Interface()
	err := protobuf.DecodeWithConstructors(buf, msg,
		network.DefaultConstructors(p.Context.server.Suite()))
	if err != nil {
		return nil, nil, xerrors.Errorf("failed to decode message: %v", err)
	}

	reply, stopServiceChan, err := callInterfaceFunc(mh.handler, msg, mh.streaming)
	if err != nil {
		return nil, nil, xerrors.Errorf("failed to get function: %v", err)
	}

	// This goroutine is responsible for listening on the service channel,
	// decoding the messages and then forwarding them to the streaming tunnel,
	// which should then forward the message to the client.
	go func() {
		inChan := reflect.ValueOf(reply)
		cases := []reflect.SelectCase{
			reflect.SelectCase{Dir: reflect.SelectRecv, Chan: inChan},
		}

		for {
			chosen, v, ok := reflect.Select(cases)
			if !ok {
				log.Lvlf4("publisher is closed for %s, closing "+
					"outgoing channel", path)
				close(outChan)
				return
			}
			if chosen == 0 {
				// Send information down to the client.
				buf, err = protobuf.Encode(v.Interface())
				if err != nil {
					log.Error(err)
					close(outChan)
					return
				}
				outChan <- buf
			} else {
				panic("no such channel index")
			}
			// We don't add a way to explicitly stop the go-routine, otherwise
			// the service will block. The service should close the channel when
			// it has nothing else to say because it is the producer. Then this
			// go-routine will be stopped as well.
		}
	}()

	// This goroutine listens on any new messages from the client and executes
	// the request. Executing the request should fill the service's channel, as
	// the service will use the same chanel for further requests.
	go func() {
		for {
			select {
			case buf, ok := <-clientInputs:
				if !ok {
					close(stopServiceChan)
					return
				}

				_, _, err := func() (interface{}, chan bool, error) {
					if !ok {
						err := xerrors.New("The requested message hasn't " +
							"been registered: " + path)
						log.Error(err)
						return nil, nil, err
					}
					msg := reflect.New(mh.msgType).Interface()

					err := protobuf.DecodeWithConstructors(buf, msg,
						network.DefaultConstructors(p.Context.server.Suite()))
					if err != nil {
						return nil, nil, xerrors.Errorf("decoding: %v", err)
					}

					return callInterfaceFunc(mh.handler, msg, mh.streaming)
				}()
				if err != nil {
					log.Error(err)
				}
			}
		}
	}()

	return nil, outChan, nil
}

// IsStreaming tell if the service registered at the given path is a streaming
// service or not. Return an error if the service is not registered.
func (p *ServiceProcessor) IsStreaming(path string) (bool, error) {
	mh, ok := p.handlers[path]
	if !ok {
		err := xerrors.New("The requested message hasn't been registered: " + path)
		log.Error(err)
		return false, err
	}
	return mh.streaming, nil
}

// ProcessClientRequest implements the Service interface, see the interface
// documentation.
func (p *ServiceProcessor) ProcessClientRequest(req *http.Request, path string, buf []byte) ([]byte, *StreamingTunnel, error) {
	mh, ok := p.handlers[path]

	if mh.streaming {
		return nil, nil, xerrors.Errorf("using a streaming request with " +
			"ProcessClientRequest: Please use instead ProcessClientStreamRequest")
	}

	reply, _, err := func() (interface{}, chan bool, error) {
		if !ok {
			err := xerrors.New("The requested message hasn't been registered: " + path)
			log.Error(err)
			return nil, nil, err
		}
		msg := reflect.New(mh.msgType).Interface()
		if err := protobuf.DecodeWithConstructors(buf, msg,
			network.DefaultConstructors(p.Context.server.Suite())); err != nil {
			return nil, nil, xerrors.Errorf("decoding: %v", err)
		}
		return callInterfaceFunc(mh.handler, msg, mh.streaming)
	}()
	if err != nil {
		return nil, nil, err
	}

	buf, err = protobuf.Encode(reply)
	if err != nil {
		log.Error(err)
		return nil, nil, xerrors.Errorf("encoding: %v", err)
	}
	return buf, nil, nil
}
