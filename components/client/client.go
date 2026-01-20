package client

import (
	"bytes"
	"context"
	"fmt"
	"github.com/fullstorydev/grpcurl"
	"github.com/goccy/go-json"
	"github.com/golang/protobuf/jsonpb"
	"github.com/jhump/protoreflect/v2/grpcdynamic"
	"github.com/jhump/protoreflect/v2/grpcreflect"
	"github.com/swaggest/jsonschema-go"
	"github.com/tiny-systems/module/api/v1alpha1"
	"github.com/tiny-systems/module/module"
	"github.com/tiny-systems/module/registry"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
	"sort"
)

const (
	ComponentName = "grpc_call"
	RequestPort   = "request"
	ResponsePort  = "response"
	ErrorPort     = "error"
)

type Context any

type Settings struct {
	Address         string      `json:"address" title:"gRPC server address" required:"true" tab:"Connect"`
	Insecure        bool        `json:"insecure" title:"Insecure mode" default:"false" tab:"Connect"`
	KeepAlive       bool        `json:"keepAlive" title:"Keep Alive" default:"false" tab:"Connect"`
	Service         ServiceName `json:"service" title:"Service" description:"Name of the service" tab:"Request"`
	Method          MethodName  `json:"method" title:"Method" description:"Name of the gRPC method" tab:"Request"`
	EnableErrorPort bool        `json:"enableErrorPort" required:"true" title:"Enable Error Port" tab:"General" description:"If error happen, error port will emit an error message"`
}

// MethodName special type which can carry its value and possible options for enum values
type MethodName struct {
	Enum
}

// ServiceName special type which can carry its value and possible options for enum values
type ServiceName struct {
	Enum
}

type RequestMsg struct {
	MessageDescriptor
}

type ResponseMsg struct {
	MessageDescriptor
}

type Error struct {
	Context Context `json:"context"`
	Error   string  `json:"error"`
}

type Request struct {
	Context Context    `json:"context" configurable:"true" title:"Context" description:"Arbitrary message to be send alongside with encoded message"`
	Request RequestMsg `json:"request" required:"true" title:"Request message" description:""`
}

type Response struct {
	Context  Context     `json:"context"`
	Response ResponseMsg `json:"response"`
}

type Component struct {
	settings Settings
	//
	servicesAvailable []string
	methodsAvailable  []string
	//
	currentService string
	currentMethod  string
	//
	currentMethodDesc protoreflect.MethodDescriptor
	//
	clientConn *grpc.ClientConn
}

func (h *Component) GetInfo() module.ComponentInfo {
	return module.ComponentInfo{
		Name:        ComponentName,
		Description: "gRPC request",
		Info:        "Sends grpc request",
		Tags:        []string{"grpc", "client"},
	}
}

func (h *Component) Handle(ctx context.Context, handler module.Handler, port string, msg interface{}) any {

	switch port {
	case v1alpha1.SettingsPort:
		in, ok := msg.(Settings)
		if !ok {
			return fmt.Errorf("invalid settings")
		}
		err := h.connectAndDiscover(ctx, &in)
		h.settings = in
		return err

	case RequestPort:

		in, ok := msg.(Request)
		if !ok {
			return fmt.Errorf("invalid input")
		}

		data, err := h.invoke(ctx, in.Request)
		if err != nil {
			if !h.settings.EnableErrorPort {
				return err
			}
			return handler(ctx, ErrorPort, Error{
				Context: in.Context,
				Error:   err.Error(),
			})
		}
		return handler(ctx, ResponsePort, Response{
			Response: ResponseMsg{
				MessageDescriptor{
					Output: data,
				},
			},
			Context: in.Context,
		})

	default:
		return fmt.Errorf("port %s is not supoprted", port)
	}
}

func (h *Component) invoke(ctx context.Context, msg any) ([]byte, error) {
	if h.currentMethodDesc == nil {
		return nil, fmt.Errorf("no method descriptor configured")
	}
	//
	input := h.currentMethodDesc.Input()
	inputMsg := dynamicpb.NewMessage(input)

	data, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	if err := jsonpb.Unmarshal(bytes.NewReader(data), inputMsg); err != nil {
		return nil, fmt.Errorf("proto unmarshal: %w", err)
	}
	//
	resp, err := grpcdynamic.NewStub(h.clientConn).InvokeRpc(ctx, h.currentMethodDesc, inputMsg)
	if err != nil {
		return nil, err
	}

	respData, err := protojson.Marshal(resp)
	if err != nil {
		return nil, err
	}

	return respData, nil
}

func (h *Component) Ports() []module.Port {

	h.settings.Service = ServiceName{
		Enum{
			Value:   h.currentService,
			Options: h.servicesAvailable,
		},
	}
	h.settings.Method = MethodName{
		Enum{
			Value:   h.currentMethod,
			Options: h.methodsAvailable,
		},
	}
	//
	response := ResponseMsg{}
	request := RequestMsg{}

	if h.currentMethodDesc != nil {
		request.Descriptor = h.currentMethodDesc.Input()
		response.Descriptor = h.currentMethodDesc.Output()
	}

	ports := []module.Port{
		{
			Name:     RequestPort,
			Label:    "Request",
			Position: module.Left,
			Configuration: Request{
				Request: request,
			},
		},
		{
			Name:     ResponsePort,
			Position: module.Right,
			Label:    "Response",
			Source:   true,
			Configuration: Response{
				Response: response,
			},
		},
		{
			Name:          v1alpha1.SettingsPort,
			Label:         "Settings",
			Configuration: h.settings,
		},
	}
	if !h.settings.EnableErrorPort {
		return ports
	}

	return append(ports, module.Port{
		Position:      module.Bottom,
		Name:          ErrorPort,
		Label:         "Error",
		Source:        true,
		Configuration: Error{},
	})
}

func (h *Component) connectAndDiscover(ctx context.Context, settings *Settings) error {
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))

	var addr = settings.Address

	if addr == "" {
		return fmt.Errorf("server address is empty")
	}

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	//
	h.clientConn = conn

	md := grpcurl.MetadataFromHeaders([]string{})
	refCtx := metadata.NewOutgoingContext(ctx, md)

	refClient := grpcreflect.NewClientAuto(refCtx, conn)

	defer refClient.Reset()

	allServices, err := refClient.ListServices()
	if err != nil {
		return err
	}

	var (
		serviceNames []string
	)
	for _, svc := range allServices {
		if svc == "grpc.reflection.v1alpha.ServerReflection" || svc == "grpc.reflection.v1.ServerReflection" {
			continue
		}
		serviceNames = append(serviceNames, string(svc))
	}

	h.currentService = settings.Service.Value

	if len(serviceNames) == 0 {
		return fmt.Errorf("no services discovered")
	}

	sort.Strings(serviceNames)
	h.servicesAvailable = serviceNames

	if h.currentService == "" {
		return fmt.Errorf("select a service")
	}

	//
	h.currentMethod = settings.Method.Value
	h.currentMethodDesc = nil

	for _, service := range serviceNames {

		if service != h.currentService {
			continue
		}

		serviceSymbol, err := refClient.FileContainingSymbol(protoreflect.FullName(service))
		if err != nil {
			return err
		}

		serviceDesc := serviceSymbol.Services()
		if serviceDesc == nil {
			continue
		}

		svcDesc := serviceDesc.ByName(protoreflect.FullName(service).Name())
		if svcDesc == nil {
			continue
		}

		//
		methodsDescs := svcDesc.Methods()
		if methodsDescs == nil {
			continue
		}

		var allMethods []string

		for i := 0; i < methodsDescs.Len(); i++ {

			methodDescriptor := methodsDescs.Get(i)
			if methodDescriptor == nil {
				continue
			}
			methodName := string(methodDescriptor.Name())
			allMethods = append(allMethods, methodName)

			if methodName == h.currentMethod {
				h.currentMethodDesc = methodDescriptor
			}
		}

		h.methodsAvailable = allMethods

		//
		if h.currentMethod == "" {
			return fmt.Errorf("select method")
		}

		if len(allMethods) == 0 {
			return nil
		}

		if h.currentMethodDesc == nil {
			return fmt.Errorf("selected method description not found")
		}
		return nil
	}

	return fmt.Errorf("selected service %s not found", h.currentService)
}

func (h *Component) Instance() module.Component {
	return &Component{}
}

var _ jsonschema.Exposer = (*ServiceName)(nil)
var _ jsonschema.Exposer = (*MethodName)(nil)

var _ json.Marshaler = (*ServiceName)(nil)
var _ json.Unmarshaler = (*ServiceName)(nil)

var _ jsonschema.Exposer = (*RequestMsg)(nil)
var _ jsonschema.Exposer = (*ResponseMsg)(nil)

var _ json.Marshaler = (*ResponseMsg)(nil)
var _ json.Unmarshaler = (*ResponseMsg)(nil)
var _ json.Marshaler = (*RequestMsg)(nil)
var _ json.Unmarshaler = (*RequestMsg)(nil)

var _ module.Component = (*Component)(nil)

func init() {
	registry.Register(&Component{})
}
