package jsonrpc2

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type clientTransportFunc func(RequestSlice) (ResponseSlice, error)

func (transportFunc clientTransportFunc) Execute(requestSlice RequestSlice) (ResponseSlice, error) {
	return transportFunc(requestSlice)
}

func TestClientRequestResponse(t *testing.T) {
	client := NewClient(clientTransportFunc(func(requestSlice RequestSlice) (ResponseSlice, error) {
		if len(requestSlice) != 1 {
			t.Fatalf("expected 1 request, got %d", len(requestSlice))
		}

		requestUnit := requestSlice[0]
		if requestUnit.JsonRPC != "2.0" {
			t.Fatalf("expected jsonrpc 2.0, got %q", requestUnit.JsonRPC)
		}
		if requestUnit.Method != "sum" {
			t.Fatalf("expected method sum, got %q", requestUnit.Method)
		}
		if requestUnit.ID == nil {
			t.Fatal("expected request id")
		}

		return ResponseSlice{
			{
				JsonRPC: "2.0",
				ID:      float64(1),
				Result:  json.RawMessage(`{"value":42}`),
			},
		}, nil
	}))

	var response struct {
		Value int `json:"value"`
	}

	err := client.Request("sum", map[string]int{"a": 40, "b": 2}).Response(&response)
	if err != nil {
		t.Fatalf("unexpected response error: %v", err)
	}
	if response.Value != 42 {
		t.Fatalf("expected response value 42, got %d", response.Value)
	}
}

func TestClientDeferBatchWithNotification(t *testing.T) {
	client := NewClient(clientTransportFunc(func(requestSlice RequestSlice) (ResponseSlice, error) {
		if len(requestSlice) != 2 {
			t.Fatalf("expected 2 requests, got %d", len(requestSlice))
		}
		if requestSlice[0].ID == nil {
			t.Fatal("expected request id")
		}
		if requestSlice[1].ID != nil {
			t.Fatalf("expected notification without id, got %v", requestSlice[1].ID)
		}

		return ResponseSlice{
			{
				JsonRPC: "2.0",
				ID:      requestSlice[0].ID,
				Result:  json.RawMessage(`"ok"`),
			},
		}, nil
	}))

	request := client.DeferRequest("request", nil)
	notification := client.DeferNotification("notify", nil)

	client.execute()

	var response string
	if err := request.Response(&response); err != nil {
		t.Fatalf("unexpected response error: %v", err)
	}
	if response != "ok" {
		t.Fatalf("expected ok response, got %q", response)
	}

	notification.Wait()
}

func TestClientTransportError(t *testing.T) {
	client := NewClient(clientTransportFunc(func(requestSlice RequestSlice) (ResponseSlice, error) {
		return nil, fmt.Errorf("network is down")
	}))

	err := client.Request("sum", nil).Response(nil)
	if err == nil {
		t.Fatal("expected response error")
	}
	if err.Code != -32603 {
		t.Fatalf("expected internal error code, got %d", err.Code)
	}
}

func TestClientInvalidParamsMarshalError(t *testing.T) {
	called := make(chan struct{}, 1)
	client := NewClient(clientTransportFunc(func(requestSlice RequestSlice) (ResponseSlice, error) {
		called <- struct{}{}
		return nil, nil
	}))

	executeUnit := client.Execute(true, "bad", func() {})
	err := executeUnit.Response(nil)
	if err == nil {
		t.Fatal("expected invalid params error")
	}
	if err.Code != -32602 {
		t.Fatalf("expected invalid params code, got %d", err.Code)
	}

	client.execute()
	select {
	case <-called:
		t.Fatal("transport should not be called when params cannot be marshaled")
	default:
	}
}

func TestClientExecuteDoesNotHoldMutexDuringTransport(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})

	client := NewClient(clientTransportFunc(func(requestSlice RequestSlice) (ResponseSlice, error) {
		close(started)
		<-release

		return ResponseSlice{
			{
				JsonRPC: "2.0",
				ID:      requestSlice[0].ID,
				Result:  json.RawMessage(`true`),
			},
		}, nil
	}))

	request := client.Request("first", nil)
	<-started

	appended := make(chan struct{})
	go func() {
		client.DeferRequest("second", nil)
		close(appended)
	}()

	select {
	case <-appended:
	case <-time.After(time.Second):
		t.Fatal("defer request blocked while transport was executing")
	}

	close(release)

	var response bool
	if err := request.Response(&response); err != nil {
		t.Fatalf("unexpected response error: %v", err)
	}
	if !response {
		t.Fatal("expected true response")
	}
}

func TestClientTransportHttp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
			t.Fatalf("expected content type application/json, got %q", contentType)
		}

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("unexpected body read error: %v", err)
		}

		requestSlice, err := NewRequestSlice(body)
		if err != nil {
			t.Fatalf("unexpected request decode error: %v", err)
		}
		if len(requestSlice) != 1 {
			t.Fatalf("expected 1 request, got %d", len(requestSlice))
		}

		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"ok"}`))
	}))
	defer server.Close()

	transport := NewClientTransportHttpWithClient(server.URL, server.Client())
	responseSlice, err := transport.Execute(RequestSlice{
		{
			JsonRPC: "2.0",
			ID:      int64(1),
			Method:  "ping",
		},
	})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if len(responseSlice) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responseSlice))
	}
}

func TestClientTransportHttpStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	transport := NewClientTransportHttpWithClient(server.URL, server.Client())
	_, err := transport.Execute(RequestSlice{
		{
			JsonRPC: "2.0",
			ID:      int64(1),
			Method:  "ping",
		},
	})
	if err == nil {
		t.Fatal("expected status error")
	}
}
