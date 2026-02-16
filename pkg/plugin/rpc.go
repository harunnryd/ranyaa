package plugin

import (
	"context"
	"net/rpc"

	"github.com/hashicorp/go-plugin"
)

// Handshake is used to verify that the plugin and host are compatible
var Handshake = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "RANYA_PLUGIN",
	MagicCookieValue: "ranya-plugin-system-v1",
}

// PluginMap is the map of plugins we can dispense
var PluginMap = map[string]plugin.Plugin{
	"plugin": &PluginRPCPlugin{},
}

// PluginRPCPlugin is the implementation of plugin.Plugin for RPC
type PluginRPCPlugin struct {
	Impl Plugin
}

func (p *PluginRPCPlugin) Server(*plugin.MuxBroker) (interface{}, error) {
	return &PluginRPCServer{Impl: p.Impl}, nil
}

func (p *PluginRPCPlugin) Client(b *plugin.MuxBroker, c *rpc.Client) (interface{}, error) {
	return &PluginRPCClient{client: c}, nil
}

// PluginRPCServer is the RPC server that PluginRPCClient talks to
type PluginRPCServer struct {
	Impl Plugin
}

// ActivateArgs are the arguments for Activate RPC call
type ActivateArgs struct {
	API    PluginAPI
	Config map[string]any
}

func (s *PluginRPCServer) Activate(args *ActivateArgs, resp *error) error {
	*resp = s.Impl.Activate(context.Background(), args.API, args.Config)
	return nil
}

func (s *PluginRPCServer) Deactivate(args interface{}, resp *error) error {
	*resp = s.Impl.Deactivate(context.Background())
	return nil
}

// ExecuteToolArgs are the arguments for ExecuteTool RPC call
type ExecuteToolArgs struct {
	Name   string
	Params map[string]any
}

// ExecuteToolResp is the response for ExecuteTool RPC call
type ExecuteToolResp struct {
	Result map[string]any
	Error  error
}

func (s *PluginRPCServer) ExecuteTool(args *ExecuteToolArgs, resp *ExecuteToolResp) error {
	result, err := s.Impl.ExecuteTool(context.Background(), args.Name, args.Params)
	resp.Result = result
	resp.Error = err
	return nil
}

// ExecuteHookArgs are the arguments for ExecuteHook RPC call
type ExecuteHookArgs struct {
	Event HookEvent
}

func (s *PluginRPCServer) ExecuteHook(args *ExecuteHookArgs, resp *error) error {
	*resp = s.Impl.ExecuteHook(context.Background(), args.Event)
	return nil
}

// ExecuteGatewayMethodArgs are the arguments for ExecuteGatewayMethod RPC call
type ExecuteGatewayMethodArgs struct {
	Name   string
	Params map[string]any
}

// ExecuteGatewayMethodResp is the response for ExecuteGatewayMethod RPC call
type ExecuteGatewayMethodResp struct {
	Result map[string]any
	Error  error
}

func (s *PluginRPCServer) ExecuteGatewayMethod(args *ExecuteGatewayMethodArgs, resp *ExecuteGatewayMethodResp) error {
	result, err := s.Impl.ExecuteGatewayMethod(context.Background(), args.Name, args.Params)
	resp.Result = result
	resp.Error = err
	return nil
}

// PluginRPCClient is the RPC client that talks to PluginRPCServer
type PluginRPCClient struct {
	client *rpc.Client
}

func (c *PluginRPCClient) Activate(ctx context.Context, api PluginAPI, config map[string]any) error {
	var resp error
	err := c.client.Call("Plugin.Activate", &ActivateArgs{API: api, Config: config}, &resp)
	if err != nil {
		return err
	}
	return resp
}

func (c *PluginRPCClient) Deactivate(ctx context.Context) error {
	var resp error
	err := c.client.Call("Plugin.Deactivate", new(interface{}), &resp)
	if err != nil {
		return err
	}
	return resp
}

func (c *PluginRPCClient) ExecuteTool(ctx context.Context, name string, params map[string]any) (map[string]any, error) {
	var resp ExecuteToolResp
	err := c.client.Call("Plugin.ExecuteTool", &ExecuteToolArgs{Name: name, Params: params}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, resp.Error
	}
	return resp.Result, nil
}

func (c *PluginRPCClient) ExecuteHook(ctx context.Context, event HookEvent) error {
	var resp error
	err := c.client.Call("Plugin.ExecuteHook", &ExecuteHookArgs{Event: event}, &resp)
	if err != nil {
		return err
	}
	return resp
}

func (c *PluginRPCClient) ExecuteGatewayMethod(ctx context.Context, name string, params map[string]any) (map[string]any, error) {
	var resp ExecuteGatewayMethodResp
	err := c.client.Call("Plugin.ExecuteGatewayMethod", &ExecuteGatewayMethodArgs{Name: name, Params: params}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, resp.Error
	}
	return resp.Result, nil
}
