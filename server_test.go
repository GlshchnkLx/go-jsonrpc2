package jsonrpc2

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type serverSumParams struct {
	A int `json:"a"`
	B int `json:"b"`
}

type serverSumResult struct {
	Value int `json:"value"`
}

func TestServerExecuteRequestResponse(t *testing.T) {
	server := NewServer()
	server.HandleFunc("sum", func(param interface{}) (interface{}, error) {
		sumParams, ok := param.(serverSumParams)
		if !ok {
			t.Fatalf("expected serverSumParams, got %T", param)
		}

		return serverSumResult{Value: sumParams.A + sumParams.B}, nil
	}, serverSumParams{}, serverSumResult{})

	responseSlice := server.Execute(RequestSlice{
		{
			JsonRPC: "2.0",
			ID:      int64(1),
			Method:  "sum",
			Params:  json.RawMessage(`{"a":40,"b":2}`),
		},
	})

	if len(responseSlice) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responseSlice))
	}

	responseUnit := responseSlice[0]
	if responseUnit.ID != int64(1) {
		t.Fatalf("expected id 1, got %v", responseUnit.ID)
	}
	if responseUnit.Error != nil {
		t.Fatalf("unexpected response error: %v", responseUnit.Error)
	}

	var result serverSumResult
	if err := json.Unmarshal(responseUnit.Result, &result); err != nil {
		t.Fatalf("unexpected result unmarshal error: %v", err)
	}
	if result.Value != 42 {
		t.Fatalf("expected result 42, got %d", result.Value)
	}
}

func TestServerExecuteNotificationNoResponse(t *testing.T) {
	called := false
	server := NewServer()
	server.HandleFunc("notify", func(param interface{}) (interface{}, error) {
		called = true
		return true, nil
	}, nil, nil)

	responseSlice := server.Execute(RequestSlice{
		{
			JsonRPC: "2.0",
			Method:  "notify",
		},
	})

	if !called {
		t.Fatal("expected notification handler to be called")
	}
	if len(responseSlice) != 0 {
		t.Fatalf("expected no response for notification, got %d", len(responseSlice))
	}
}

func TestServerExecuteMethodNotFound(t *testing.T) {
	server := NewServer()

	responseSlice := server.Execute(RequestSlice{
		{
			JsonRPC: "2.0",
			ID:      int64(1),
			Method:  "missing",
		},
	})

	responseUnit := requireSingleServerResponse(t, responseSlice)
	requireServerErrorCode(t, responseUnit, -32601)
}

func TestServerExecuteInvalidParamsDoesNotCallHandler(t *testing.T) {
	called := false
	server := NewServer()
	server.HandleFunc("sum", func(param interface{}) (interface{}, error) {
		called = true
		return nil, nil
	}, serverSumParams{}, nil)

	responseSlice := server.Execute(RequestSlice{
		{
			JsonRPC: "2.0",
			ID:      int64(1),
			Method:  "sum",
			Params:  json.RawMessage(`{"a":"bad","b":2}`),
		},
	})

	if called {
		t.Fatal("handler should not be called when params are invalid")
	}

	responseUnit := requireSingleServerResponse(t, responseSlice)
	requireServerErrorCode(t, responseUnit, -32602)
}

func TestServerExecuteHandlerErrorMapping(t *testing.T) {
	server := NewServer()
	server.HandleFunc("custom", func(param interface{}) (interface{}, error) {
		return nil, NewErrorServerError(7, "custom failure")
	}, nil, nil)
	server.HandleFunc("internal", func(param interface{}) (interface{}, error) {
		return nil, fmt.Errorf("not a json-rpc error")
	}, nil, nil)

	responseSlice := server.Execute(RequestSlice{
		{
			JsonRPC: "2.0",
			ID:      int64(1),
			Method:  "custom",
		},
		{
			JsonRPC: "2.0",
			ID:      int64(2),
			Method:  "internal",
		},
	})

	if len(responseSlice) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(responseSlice))
	}
	requireServerErrorCode(t, responseSlice[0], -32007)
	requireServerErrorCode(t, responseSlice[1], -32603)
}

func TestServerExecuteHandlerReturnsNothing(t *testing.T) {
	server := NewServer()
	server.HandleFunc("nothing", func(param interface{}) (interface{}, error) {
		return nil, nil
	}, nil, nil)

	responseSlice := server.Execute(RequestSlice{
		{
			JsonRPC: "2.0",
			ID:      int64(1),
			Method:  "nothing",
		},
	})

	responseUnit := requireSingleServerResponse(t, responseSlice)
	requireServerErrorCode(t, responseUnit, -32603)
}

func TestServerExecuteBatchInvalidRequestDoesNotReusePreviousResponse(t *testing.T) {
	server := NewServer()
	server.HandleFunc("ok", func(param interface{}) (interface{}, error) {
		return "ok", nil
	}, nil, nil)

	responseSlice := server.Execute(RequestSlice{
		{
			JsonRPC: "2.0",
			ID:      int64(1),
			Method:  "ok",
		},
		{
			JsonRPC: "1.0",
			ID:      int64(2),
			Method:  "ok",
		},
	})

	if len(responseSlice) != 2 {
		t.Fatalf("expected 2 responses, got %d: %#v", len(responseSlice), responseSlice)
	}
	if responseSlice[0].ID != int64(1) {
		t.Fatalf("expected first response id 1, got %v", responseSlice[0].ID)
	}
	if responseSlice[1].ID != int64(2) {
		t.Fatalf("expected second response id 2, got %v", responseSlice[1].ID)
	}
	requireServerErrorCode(t, responseSlice[1], -32600)
}

func TestServerServeHTTPValidRequest(t *testing.T) {
	server := NewServer()
	server.HandleFunc("sum", func(param interface{}) (interface{}, error) {
		sumParams := param.(serverSumParams)
		return serverSumResult{Value: sumParams.A + sumParams.B}, nil
	}, serverSumParams{}, serverSumResult{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"sum","params":{"a":40,"b":2}}`))

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}

	responseSlice, err := NewResponseSlice(recorder.Body.Bytes())
	if err != nil {
		t.Fatalf("unexpected response decode error: %v", err)
	}

	responseUnit := requireSingleServerResponse(t, responseSlice)
	if responseUnit.Error != nil {
		t.Fatalf("unexpected response error: %v", responseUnit.Error)
	}

	var result serverSumResult
	if err := json.Unmarshal(responseUnit.Result, &result); err != nil {
		t.Fatalf("unexpected result unmarshal error: %v", err)
	}
	if result.Value != 42 {
		t.Fatalf("expected result 42, got %d", result.Value)
	}
}

func TestServerServeHTTPParseError(t *testing.T) {
	server := NewServer()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{`))

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}

	responseSlice, err := NewResponseSlice(recorder.Body.Bytes())
	if err != nil {
		t.Fatalf("unexpected response decode error: %v", err)
	}

	responseUnit := requireSingleServerResponse(t, responseSlice)
	requireServerErrorCode(t, responseUnit, -32700)
}

func TestServerServeHTTPNotificationWritesNoBody(t *testing.T) {
	server := NewServer()
	server.HandleFunc("notify", func(param interface{}) (interface{}, error) {
		return true, nil
	}, nil, nil)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"jsonrpc":"2.0","method":"notify"}`))

	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	if recorder.Body.Len() != 0 {
		t.Fatalf("expected empty response body, got %q", recorder.Body.String())
	}
}

func requireSingleServerResponse(t *testing.T, responseSlice ResponseSlice) *ResponseUnit {
	t.Helper()

	if len(responseSlice) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responseSlice))
	}
	if responseSlice[0] == nil {
		t.Fatal("expected response unit, got nil")
	}

	return responseSlice[0]
}

func requireServerErrorCode(t *testing.T, responseUnit *ResponseUnit, code int32) {
	t.Helper()

	if responseUnit.Error == nil {
		t.Fatalf("expected error code %d, got nil error", code)
	}
	if responseUnit.Error.Code != code {
		t.Fatalf("expected error code %d, got %d", code, responseUnit.Error.Code)
	}
}
