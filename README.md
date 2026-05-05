# go-jsonrpc2

Small JSON-RPC 2.0 client and server library for Go.

`go-jsonrpc2` focuses on the common HTTP-based JSON-RPC workflow: register typed server handlers, call methods from a client, send notifications, and work with single or batched JSON-RPC payloads. The package keeps the API compact and uses the standard library for HTTP and JSON handling.

## Features

- JSON-RPC 2.0 request, response, notification, and batch encoding.
- HTTP server implementation via `http.Handler`.
- HTTP client transport based on `net/http`.
- Typed server params through `encoding/json` unmarshalling.
- Client-side request and notification APIs.
- Direct `RequestSlice` and `ResponseSlice` support for explicit batches.
- JSON-RPC error helpers for standard error codes.

## Installation

Use Go modules:

```sh
go get github.com/GlshchnkLx/go-jsonrpc2
```

Then import the package:

```go
import jsonrpc2 "github.com/GlshchnkLx/go-jsonrpc2"
```

## Server Example

```go
package main

import (
	"fmt"
	"log"
	"net/http"

	jsonrpc2 "github.com/GlshchnkLx/go-jsonrpc2"
)

type SumParams struct {
	A int `json:"a"`
	B int `json:"b"`
}

type SumResult struct {
	Value int `json:"value"`
}

func main() {
	server := jsonrpc2.NewServer()

	server.HandleFunc("sum", func(param interface{}) (interface{}, error) {
		params, ok := param.(SumParams)
		if !ok {
			return nil, jsonrpc2.NewErrorInvalidParams("expected sum params")
		}

		return SumResult{Value: params.A + params.B}, nil
	}, SumParams{}, SumResult{})

	server.HandleFunc("notify", func(param interface{}) (interface{}, error) {
		fmt.Println("notification received")
		return true, nil
	}, nil, nil)

	http.Handle("/rpc", server)

	log.Println("JSON-RPC server listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
```

Example request:

```sh
curl -X POST http://localhost:8080/rpc \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"sum","params":{"a":40,"b":2}}'
```

Response:

```json
{"jsonrpc":"2.0","id":1,"result":{"value":42}}
```

Notifications omit `id`, so the server executes the handler and returns no response body:

```sh
curl -X POST http://localhost:8080/rpc \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"notify"}'
```

## Client Example

```go
package main

import (
	"fmt"
	"log"

	jsonrpc2 "github.com/GlshchnkLx/go-jsonrpc2"
)

type SumParams struct {
	A int `json:"a"`
	B int `json:"b"`
}

type SumResult struct {
	Value int `json:"value"`
}

func main() {
	transport := jsonrpc2.NewClientTransportHttp("http://localhost:8080/rpc")
	client := jsonrpc2.NewClient(transport)

	var result SumResult
	if err := client.Request("sum", SumParams{A: 40, B: 2}).Response(&result); err != nil {
		log.Fatal(err)
	}

	fmt.Println(result.Value)
}
```

## Notifications

Use notifications for fire-and-forget calls. They do not include a JSON-RPC request id and do not expect a response.

```go
notification := client.Notification("notify", nil)
notification.Wait()
```

`Wait` only waits until the client transport call has completed.

## Batched Calls

The package has internal support for sending a `RequestSlice` as a JSON-RPC batch, and the HTTP transport can execute a batch directly. This is useful when you want to build the batch yourself:

```go
import "encoding/json"

transport := jsonrpc2.NewClientTransportHttp("http://localhost:8080/rpc")

responses, err := transport.Execute(jsonrpc2.RequestSlice{
	{
		JsonRPC: "2.0",
		ID:      int64(1),
		Method:  "sum",
		Params:  json.RawMessage(`{"a":40,"b":2}`),
	},
	{
		JsonRPC: "2.0",
		Method:  "notify",
	},
})

if err != nil {
	log.Fatal(err)
}

fmt.Println(responses)
```

The higher-level `Client.Request` and `Client.Notification` helpers execute automatically. `DeferRequest` and `DeferNotification` queue calls internally, but the public API currently does not expose a batch flush method, so direct `ClientTransport.Execute` is the practical public option for explicit batching.

## Errors

Handlers can return any `error`. If the error is a `*jsonrpc2.Error`, its JSON-RPC code is returned as-is. Other errors are converted to an internal error.

```go
return nil, jsonrpc2.NewErrorInvalidParams("missing field a")
```

Available helpers:

- `NewErrorParseError`
- `NewErrorInvalidRequest`
- `NewErrorMethodNotFound`
- `NewErrorInvalidParams`
- `NewErrorInternalError`
- `NewErrorServerError`

## Notes

- The server accepts only HTTP `POST` requests.
- Request params are unmarshaled into the type passed to `HandleFunc`.
- A request without `id` is treated as a notification.
- A single request/response is encoded as a JSON object; multiple units are encoded as a JSON array.

## License

See [LICENSE](LICENSE).
