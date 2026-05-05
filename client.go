package jsonrpc2

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

//--------------------------------------------------------------------------------//
// CLIENT TRANSPORT
//--------------------------------------------------------------------------------//

type ClientTransport interface {
	Execute(RequestSlice) (ResponseSlice, error)
}

type ClientTransportHttp struct {
	ClientTransport

	endpoint string
	client   *http.Client
}

func (clientTransport *ClientTransportHttp) Execute(requestSlice RequestSlice) (responseSlice ResponseSlice, err error) {
	var (
		requestSliceJson   []byte
		requestSliceBuffer *bytes.Buffer
		httpResponse       *http.Response
		httpClient         *http.Client
	)

	requestSliceJson, err = requestSlice.MarshalJSON()
	if err != nil {
		return
	}

	requestSliceBuffer = bytes.NewBuffer(requestSliceJson)
	httpClient = clientTransport.client
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	httpResponse, err = httpClient.Post(clientTransport.endpoint, "application/json", requestSliceBuffer)
	if err != nil {
		return
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode < http.StatusOK || httpResponse.StatusCode >= http.StatusMultipleChoices {
		err = fmt.Errorf("http transport status code %d", httpResponse.StatusCode)
		return
	}

	responseSlice, err = NewResponseSlice(httpResponse.Body)
	if err != nil {
		return
	}

	return
}

func NewClientTransportHttp(endpoint string) *ClientTransportHttp {
	return NewClientTransportHttpWithClient(endpoint, nil)
}

func NewClientTransportHttpWithClient(endpoint string, httpClient *http.Client) *ClientTransportHttp {
	return &ClientTransportHttp{
		endpoint: endpoint,
		client:   httpClient,
	}
}

//--------------------------------------------------------------------------------//
// CLIENT EXECUTE
//--------------------------------------------------------------------------------//

type ClientExecuteUnit struct {
	ID interface{}

	method string
	option json.RawMessage

	result json.RawMessage
	error  *Error

	done     chan struct{}
	doneOnce sync.Once
}

func (executeUnit *ClientExecuteUnit) RequestUnit() *RequestUnit {
	if executeUnit == nil {
		return nil
	}

	requestUnit := &RequestUnit{
		JsonRPC: "2.0",
		Method:  executeUnit.method,
		Params:  executeUnit.option,
	}

	if executeUnit.ID != nil {
		requestUnit.ID = executeUnit.ID
	}

	return requestUnit
}

func (executeUnit *ClientExecuteUnit) Execute(responseUnit *ResponseUnit) {
	if executeUnit == nil {
		return
	}

	if responseUnit == nil {
		executeUnit.error = NewErrorInternalError("response is nil")
		executeUnit.finish()
		return
	}

	executeUnit.result = responseUnit.Result
	executeUnit.error = responseUnit.Error
	executeUnit.finish()
}

func (executeUnit *ClientExecuteUnit) Fail(err *Error) {
	if executeUnit == nil {
		return
	}

	executeUnit.error = err
	executeUnit.finish()
}

func (executeUnit *ClientExecuteUnit) Wait() {
	if executeUnit == nil || executeUnit.done == nil {
		return
	}

	<-executeUnit.done
}

func (executeUnit *ClientExecuteUnit) Response(result interface{}) *Error {
	if executeUnit == nil {
		return NewErrorInternalError("execute unit is nil")
	}

	executeUnit.Wait()

	if result != nil && executeUnit.result != nil {
		err := json.Unmarshal(executeUnit.result, result)
		if err != nil {
			return NewErrorParseError(err.Error())
		}
	}

	return executeUnit.error
}

func (executeUnit *ClientExecuteUnit) finish() {
	if executeUnit.done == nil {
		return
	}

	executeUnit.doneOnce.Do(func() {
		close(executeUnit.done)
	})
}

type ClientRequest interface {
	Wait()
	Response(interface{}) *Error
}

type ClientNotification interface {
	Wait()
}

type clientExecuteUnit = ClientExecuteUnit
type clientExecuteRequest = ClientRequest
type clientExecuteNotification = ClientNotification

//--------------------------------------------------------------------------------//
// CLIENT
//--------------------------------------------------------------------------------//

type Client struct {
	mutex     sync.Mutex
	transport ClientTransport

	executeIndex int64
	executeArray []*ClientExecuteUnit
}

func (client *Client) execute() {
	executeArray := client.takeExecuteArray()
	if len(executeArray) == 0 {
		return
	}

	client.executeArrayUnits(executeArray)
}

func (client *Client) executeArrayUnits(executeArray []*ClientExecuteUnit) {
	var (
		executeMap    = map[interface{}]*ClientExecuteUnit{}
		executeUnit   *ClientExecuteUnit
		requestSlice  = RequestSlice{}
		responseUnit  *ResponseUnit
		responseSlice ResponseSlice
		err           error
	)

	for _, executeUnit = range executeArray {
		if executeUnit == nil {
			continue
		}

		requestSlice = append(requestSlice, executeUnit.RequestUnit())
		if executeUnit.ID != nil {
			executeMap[clientResponseIDKey(executeUnit.ID)] = executeUnit
		}
	}

	if client.transport == nil {
		client.failExecuteArray(executeArray, NewErrorInternalError("client transport is nil"))
		return
	}

	responseSlice, err = client.transport.Execute(requestSlice)
	if err != nil {
		client.failExecuteArray(executeArray, NewErrorInternalError(err.Error()))
		return
	}

	for _, responseUnit = range responseSlice {
		if responseUnit == nil || responseUnit.ID == nil {
			continue
		}

		executeUnit = executeMap[clientResponseIDKey(responseUnit.ID)]
		if executeUnit == nil {
			continue
		}

		executeUnit.Execute(responseUnit)
		delete(executeMap, clientResponseIDKey(responseUnit.ID))
	}

	for _, executeUnit = range executeArray {
		if executeUnit == nil {
			continue
		}

		if executeUnit.ID != nil {
			if _, ok := executeMap[clientResponseIDKey(executeUnit.ID)]; ok {
				executeUnit.Fail(NewErrorInternalError(nil))
			}
		} else {
			executeUnit.finish()
		}
	}
}

func (client *Client) Execute(withIndex bool, method string, option interface{}) (executeUnit *ClientExecuteUnit) {
	var (
		err     error
		id      interface{}
		options json.RawMessage
	)

	client.mutex.Lock()
	client.executeIndex++
	if withIndex {
		id = client.executeIndex
	}
	client.mutex.Unlock()

	if option != nil {
		options, err = json.Marshal(option)
		if err != nil {
			executeUnit = newClientExecuteUnit(id, method, nil)
			executeUnit.Fail(NewErrorInvalidParams(err.Error()))
			return
		}
	}

	executeUnit = newClientExecuteUnit(id, method, options)

	client.mutex.Lock()
	client.executeArray = append(client.executeArray, executeUnit)
	client.mutex.Unlock()

	return
}

func (client *Client) Request(method string, option interface{}) ClientRequest {
	executeUnit := client.Execute(true, method, option)

	go client.execute()

	return executeUnit
}

func (client *Client) DeferRequest(method string, option interface{}) ClientRequest {
	executeUnit := client.Execute(true, method, option)
	return executeUnit
}

func (client *Client) Notification(method string, option interface{}) ClientNotification {
	executeUnit := client.Execute(false, method, option)

	go client.execute()

	return executeUnit
}

func (client *Client) DeferNotification(method string, option interface{}) ClientNotification {
	executeUnit := client.Execute(false, method, option)
	return executeUnit
}

func (client *Client) takeExecuteArray() []*ClientExecuteUnit {
	client.mutex.Lock()
	defer client.mutex.Unlock()

	if len(client.executeArray) == 0 {
		return nil
	}

	executeArray := client.executeArray
	client.executeArray = nil

	return executeArray
}

func (client *Client) failExecuteArray(executeArray []*ClientExecuteUnit, responseError *Error) {
	for _, executeUnit := range executeArray {
		if executeUnit == nil {
			continue
		}

		executeUnit.Fail(responseError)
	}
}

func newClientExecuteUnit(id interface{}, method string, option json.RawMessage) *ClientExecuteUnit {
	return &ClientExecuteUnit{
		ID:     id,
		method: method,
		option: option,
		done:   make(chan struct{}),
	}
}

func clientResponseIDKey(id interface{}) interface{} {
	switch value := id.(type) {
	case int:
		return int64(value)
	case int8:
		return int64(value)
	case int16:
		return int64(value)
	case int32:
		return int64(value)
	case int64:
		return value
	case uint:
		return int64(value)
	case uint8:
		return int64(value)
	case uint16:
		return int64(value)
	case uint32:
		return int64(value)
	case uint64:
		return int64(value)
	case float32:
		return clientFloatIDKey(float64(value))
	case float64:
		return clientFloatIDKey(value)
	case json.Number:
		if intValue, err := value.Int64(); err == nil {
			return intValue
		}
		return value.String()
	default:
		return value
	}
}

func clientFloatIDKey(value float64) interface{} {
	intValue := int64(value)
	if value == float64(intValue) {
		return intValue
	}

	return value
}

func NewClient(clientTransport ClientTransport) *Client {
	return &Client{
		transport: clientTransport,
	}
}

//--------------------------------------------------------------------------------//
