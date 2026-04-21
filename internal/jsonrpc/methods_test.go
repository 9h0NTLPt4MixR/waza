package jsonrpc

import (
	"context"
	"encoding/json"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMethodRegistry_NewIsEmpty(t *testing.T) {
	reg := NewMethodRegistry()
	assert.Empty(t, reg.Methods())
	assert.Nil(t, reg.Lookup("anything"))
}

func TestMethodRegistry_RegisterAndLookup(t *testing.T) {
	reg := NewMethodRegistry()
	handler := func(_ context.Context, _ json.RawMessage) (any, *Error) {
		return "ok", nil
	}
	reg.Register("test.method", handler)

	found := reg.Lookup("test.method")
	require.NotNil(t, found)

	// Verify handler works
	result, err := found(context.Background(), nil)
	assert.Nil(t, err)
	assert.Equal(t, "ok", result)
}

func TestMethodRegistry_LookupMissing(t *testing.T) {
	reg := NewMethodRegistry()
	reg.Register("exists", func(_ context.Context, _ json.RawMessage) (any, *Error) {
		return nil, nil
	})
	assert.Nil(t, reg.Lookup("does.not.exist"))
}

func TestMethodRegistry_Methods_ReturnsList(t *testing.T) {
	reg := NewMethodRegistry()
	reg.Register("alpha", func(_ context.Context, _ json.RawMessage) (any, *Error) {
		return nil, nil
	})
	reg.Register("beta", func(_ context.Context, _ json.RawMessage) (any, *Error) {
		return nil, nil
	})
	reg.Register("gamma", func(_ context.Context, _ json.RawMessage) (any, *Error) {
		return nil, nil
	})

	methods := reg.Methods()
	sort.Strings(methods)
	assert.Equal(t, []string{"alpha", "beta", "gamma"}, methods)
}

func TestMethodRegistry_OverwriteHandler(t *testing.T) {
	reg := NewMethodRegistry()
	reg.Register("method", func(_ context.Context, _ json.RawMessage) (any, *Error) {
		return "first", nil
	})
	reg.Register("method", func(_ context.Context, _ json.RawMessage) (any, *Error) {
		return "second", nil
	})

	handler := reg.Lookup("method")
	require.NotNil(t, handler)
	result, _ := handler(context.Background(), nil)
	assert.Equal(t, "second", result, "later registration should overwrite")
}

func TestMethodRegistry_EmptyMethodName(t *testing.T) {
	reg := NewMethodRegistry()
	reg.Register("", func(_ context.Context, _ json.RawMessage) (any, *Error) {
		return "empty", nil
	})

	handler := reg.Lookup("")
	require.NotNil(t, handler)
	result, _ := handler(context.Background(), nil)
	assert.Equal(t, "empty", result)
}

func TestMethodRegistry_HandlerReturnsError(t *testing.T) {
	reg := NewMethodRegistry()
	reg.Register("fail", func(_ context.Context, _ json.RawMessage) (any, *Error) {
		return nil, ErrInternalError("broke")
	})

	handler := reg.Lookup("fail")
	require.NotNil(t, handler)
	result, rpcErr := handler(context.Background(), nil)
	assert.Nil(t, result)
	require.NotNil(t, rpcErr)
	assert.Equal(t, CodeInternalError, rpcErr.Code)
}

func TestMethodRegistry_HandlerReceivesParams(t *testing.T) {
	reg := NewMethodRegistry()
	reg.Register("echo", func(_ context.Context, params json.RawMessage) (any, *Error) {
		var p map[string]string
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, ErrInvalidParams(err.Error())
		}
		return p, nil
	})

	handler := reg.Lookup("echo")
	require.NotNil(t, handler)

	params, _ := json.Marshal(map[string]string{"key": "value"})
	result, rpcErr := handler(context.Background(), params)
	assert.Nil(t, rpcErr)
	m, ok := result.(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "value", m["key"])
}

func TestMethodRegistry_HandlerReceivesMalformedParams(t *testing.T) {
	reg := NewMethodRegistry()
	reg.Register("strict", func(_ context.Context, params json.RawMessage) (any, *Error) {
		var p struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, ErrInvalidParams(err.Error())
		}
		return p.Name, nil
	})

	handler := reg.Lookup("strict")
	require.NotNil(t, handler)

	result, rpcErr := handler(context.Background(), json.RawMessage(`not json`))
	assert.Nil(t, result)
	require.NotNil(t, rpcErr)
	assert.Equal(t, CodeInvalidParams, rpcErr.Code)
}

func TestMethodRegistry_MultipleRegistrations(t *testing.T) {
	reg := NewMethodRegistry()

	// Register many methods to verify map scaling
	for i := 0; i < 50; i++ {
		name := "method." + string(rune('a'+i%26)) + string(rune('0'+i/26))
		reg.Register(name, func(_ context.Context, _ json.RawMessage) (any, *Error) {
			return nil, nil
		})
	}

	methods := reg.Methods()
	assert.Len(t, methods, 50)
}

func TestRegisterHandlers_AllMethodsPresent(t *testing.T) {
	reg := NewMethodRegistry()
	hctx := NewHandlerContext()
	RegisterHandlers(reg, hctx)

	expected := []string{
		"eval.list", "eval.get", "eval.validate", "eval.run",
		"task.list", "task.get", "run.status", "run.cancel",
	}

	for _, method := range expected {
		assert.NotNil(t, reg.Lookup(method), "method %q should be registered", method)
	}

	// No extra methods
	assert.Len(t, reg.Methods(), len(expected))
}
