// Package core runs the operations of the xhandler WebAssembly module.
//
// The module exports one JSON entry point (see xhandler-rust/xhandler-wasm):
// the request is {"op", "input"} and the response {"ok": ...} or
// {"error": {"kind", "message"}}.
package core

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

//go:embed xhandler.wasm
var wasm []byte

//go:embed VERSION
var version string

// Version is the version of the embedded xhandler core.
var Version = strings.TrimSpace(version)

// Invoke runs op with input (marshaled to JSON) and unmarshals the result
// into output. Errors of the operation are returned as *Error.
func Invoke(ctx context.Context, op string, input, output any) error {
	request, err := json.Marshal(struct {
		Op    string `json:"op"`
		Input any    `json:"input"`
	}{op, input})
	if err != nil {
		return fmt.Errorf("xhandler %s: %w", op, err)
	}

	raw, err := InvokeRaw(ctx, request)
	if err != nil {
		return err
	}

	var response struct {
		Ok    json.RawMessage `json:"ok"`
		Error *Error          `json:"error"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return fmt.Errorf("xhandler %s: invalid response: %w", op, err)
	}
	if response.Error != nil {
		return response.Error
	}
	if err := json.Unmarshal(response.Ok, output); err != nil {
		return fmt.Errorf("xhandler %s: invalid result: %w", op, err)
	}
	return nil
}

// InvokeRaw sends a JSON request to the module and returns its JSON response.
func InvokeRaw(ctx context.Context, request []byte) ([]byte, error) {
	m, err := modules.get(ctx)
	if err != nil {
		return nil, err
	}
	response, err := m.invoke(ctx, request)
	if err != nil {
		// A trap (e.g. a panic in the core) leaves the instance unusable.
		_ = m.Close(ctx)
		return nil, fmt.Errorf("xhandler core: %w", err)
	}
	modules.put(ctx, m)
	return response, nil
}

// pool keeps idle module instances: an instance runs one call at a time and
// parses the XML templates on its first use, so it is worth reusing.
type pool struct {
	init     sync.Once
	initErr  error
	runtime  wazero.Runtime
	compiled wazero.CompiledModule
	idle     chan *module
}

var modules = &pool{idle: make(chan *module, runtime.GOMAXPROCS(0))}

func (p *pool) get(ctx context.Context) (*module, error) {
	p.init.Do(func() { p.initErr = p.compile() })
	if p.initErr != nil {
		return nil, p.initErr
	}
	select {
	case m := <-p.idle:
		return m, nil
	default:
		return p.instantiate(ctx)
	}
}

func (p *pool) put(ctx context.Context, m *module) {
	select {
	case p.idle <- m:
	default:
		_ = m.Close(ctx)
	}
}

// compile prepares the module once; instances are cheap after that.
func (p *pool) compile() error {
	ctx := context.Background()
	p.runtime = wazero.NewRuntime(ctx)
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, p.runtime); err != nil {
		return fmt.Errorf("xhandler core: WASI: %w", err)
	}
	compiled, err := p.runtime.CompileModule(ctx, wasm)
	if err != nil {
		return fmt.Errorf("xhandler core: compile: %w", err)
	}
	p.compiled = compiled
	return nil
}

func (p *pool) instantiate(ctx context.Context) (*module, error) {
	config := wazero.NewModuleConfig().
		WithName(""). // anonymous: several instances can coexist
		WithStartFunctions("_initialize").
		WithRandSource(rand.Reader).
		WithSysWalltime(). // the core uses today's date by default
		WithSysNanotime()
	instance, err := p.runtime.InstantiateModule(ctx, p.compiled, config)
	if err != nil {
		return nil, fmt.Errorf("xhandler core: instantiate: %w", err)
	}
	return &module{
		Module: instance,
		alloc:  instance.ExportedFunction("xh_alloc"),
		free:   instance.ExportedFunction("xh_free"),
		call:   instance.ExportedFunction("xh_invoke"),
	}, nil
}

type module struct {
	api.Module
	alloc, free, call api.Function
}

// invoke runs one request. After an error (a trap) the instance must be
// discarded, so nothing is freed then.
func (m *module) invoke(ctx context.Context, request []byte) ([]byte, error) {
	size := uint64(len(request))
	result, err := m.alloc.Call(ctx, size)
	if err != nil {
		return nil, err
	}
	ptr := result[0]
	if !m.Memory().Write(uint32(ptr), request) {
		return nil, fmt.Errorf("request out of memory bounds")
	}
	result, err = m.call.Call(ctx, ptr, size)
	if err != nil {
		return nil, err
	}
	responsePtr, responseSize := uint32(result[0]>>32), uint32(result[0])
	view, ok := m.Memory().Read(responsePtr, responseSize)
	if !ok {
		return nil, fmt.Errorf("response out of memory bounds")
	}
	// The view is the module memory: copy it before freeing.
	response := append([]byte(nil), view...)
	if _, err := m.free.Call(ctx, ptr, size); err != nil {
		return nil, err
	}
	if _, err := m.free.Call(ctx, uint64(responsePtr), uint64(responseSize)); err != nil {
		return nil, err
	}
	return response, nil
}
