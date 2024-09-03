package jsonrpc2

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
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
}

func (clientTransport *ClientTransportHttp) Execute(requestSlice RequestSlice) (responseSlice ResponseSlice, err error) {
	var (
		requestSliceJson []byte

		requestSliceBuffer *bytes.Buffer
		httpResponse       *http.Response
	)

	requestSliceJson, err = requestSlice.MarshalJSON()
	if err != nil {
		return
	}

	requestSliceBuffer = bytes.NewBuffer(requestSliceJson)

	httpResponse, err = http.Post(clientTransport.endpoint, "application/json", requestSliceBuffer)
	if err != nil {
		return
	}

	responseSlice, err = NewResponseSlice(httpResponse.Body)
	if err != nil {
		return
	}

	return
}

func NewClientTransportHttp(endpoint string) *ClientTransportHttp {
	return &ClientTransportHttp{
		endpoint: endpoint,
	}
}

//--------------------------------------------------------------------------------//
// CLIENT EXECUTE
//--------------------------------------------------------------------------------//

type clientExecuteUnit struct {
	controlMutex chan interface{}
	executeMutex chan interface{}

	index int64

	method string
	option json.RawMessage

	result json.RawMessage
	error  *Error
}

func (executeUnit *clientExecuteUnit) Wait() {
	executeUnit.controlMutex <- true

	if executeUnit.executeMutex != nil {
		<-executeUnit.executeMutex
		executeUnit.executeMutex = nil
	}

	<-executeUnit.controlMutex
}

func (executeUnit *clientExecuteUnit) Response(result interface{}) *Error {
	executeUnit.Wait()

	if result != nil && executeUnit.result != nil {
		err := json.Unmarshal(executeUnit.result, result)
		if err != nil {
			return NewErrorParseError(err.Error())
		}
	}

	return executeUnit.error
}

type clientExecuteRequest interface {
	Wait()
	Response(interface{}) *Error
}

type clientExecuteNotification interface {
	Wait()
}

//--------------------------------------------------------------------------------//
// CLIENT
//--------------------------------------------------------------------------------//

type Client struct {
	mutex     chan interface{}
	transport ClientTransport

	executeIndex int64
	executeArray []*clientExecuteUnit
}

func (client *Client) execute() {
	var (
		executeIndex  int64
		executeMap    = map[int64]*clientExecuteUnit{}
		executeUnit   *clientExecuteUnit
		requestUnit   *RequestUnit
		requestSlice  = RequestSlice{}
		responseUnit  *ResponseUnit
		responseSlice ResponseSlice
		err           error
	)

	client.mutex <- true
	defer func() {
		<-client.mutex
	}()

	if len(client.executeArray) == 0 {
		return
	}

	for _, executeUnit = range client.executeArray {
		requestUnit = &RequestUnit{
			JsonRPC: "2.0",
			Method:  executeUnit.method,
			Params:  executeUnit.option,
		}

		if executeUnit.index >= 0 {
			requestUnit.ID = executeUnit.index
		}

		executeMap[executeUnit.index] = executeUnit
		requestSlice = append(requestSlice, requestUnit)
	}

	client.executeArray = []*clientExecuteUnit{}

	responseSlice, err = client.transport.Execute(requestSlice)

	if err != nil {
		for _, executeUnit = range executeMap {
			executeUnit.error = NewErrorInternalError(err.Error())
			executeUnit.executeMutex <- true
		}

		return
	}

	for _, responseUnit = range responseSlice {
		if responseUnit.ID != nil {
			executeIndexFloat, ok := responseUnit.ID.(float64)
			if ok {
				executeIndex = int64(executeIndexFloat)
			}

			executeUnit := executeMap[executeIndex]
			if executeUnit != nil {
				executeUnit.result = responseUnit.Result
				executeUnit.error = responseUnit.Error

				delete(executeMap, executeIndex)
				executeUnit.executeMutex <- true
			} else {
				fmt.Println("client.execute", "unknown executeUnit", responseUnit)
			}
		} else {
			fmt.Println("client.execute", "unknown response unit", responseUnit)
		}
	}

	for _, executeUnit = range executeMap {
		if executeUnit.index >= 0 {
			executeUnit.error = NewErrorInternalError(nil)
		}

		executeUnit.executeMutex <- true
	}
}

func (client *Client) Execute(withIndex bool, method string, option interface{}) (executeUnit *clientExecuteUnit) {
	var err error

	client.mutex <- true
	client.executeIndex++

	executeUnit = &clientExecuteUnit{
		controlMutex: make(chan interface{}, 1),
		method:       method,
	}

	if withIndex {
		executeUnit.index = client.executeIndex
	} else {
		executeUnit.index = -client.executeIndex
	}

	if option != nil {
		executeUnit.option, err = json.Marshal(option)
		if err != nil {
			executeUnit.error = NewErrorInvalidParams(err.Error())
			<-client.mutex
			return
		}
	}

	executeUnit.executeMutex = make(chan interface{}, 1)

	if client.executeArray == nil {
		client.executeArray = []*clientExecuteUnit{executeUnit}
	} else {
		client.executeArray = append(client.executeArray, executeUnit)
	}

	<-client.mutex

	return
}

func (client *Client) Request(method string, option interface{}) clientExecuteRequest {
	executeUnit := client.Execute(true, method, option)

	go client.execute()

	return executeUnit
}

func (client *Client) DeferRequest(method string, option interface{}) clientExecuteRequest {
	executeUnit := client.Execute(true, method, option)
	return executeUnit
}

func (client *Client) Notification(method string, option interface{}) clientExecuteNotification {
	executeUnit := client.Execute(false, method, option)

	go client.execute()

	return executeUnit
}

func (client *Client) DeferNotification(method string, option interface{}) clientExecuteNotification {
	executeUnit := client.Execute(false, method, option)
	return executeUnit
}

func NewClient(clientTransport ClientTransport) *Client {
	return &Client{
		mutex:     make(chan interface{}, 1),
		transport: clientTransport,

		executeIndex: 0,
		executeArray: nil,
	}
}

//--------------------------------------------------------------------------------//
