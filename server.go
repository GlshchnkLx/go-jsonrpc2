package jsonrpc2

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
)

//--------------------------------------------------------------------------------//
// SERVER HANDLER
//--------------------------------------------------------------------------------//

type ServerHandlerFunc func(interface{}) (interface{}, error)

type ServerHandlerUnit struct {
	Request  reflect.Type
	Response reflect.Type
	Function ServerHandlerFunc
}

func (handler *ServerHandlerUnit) Execute(requestUnit *RequestUnit) (responseUnit *ResponseUnit) {
	var (
		requestParamInterface interface{}
		requestParamReflect   reflect.Value
		responseResult        interface{}
		responseResultJson    json.RawMessage
		responseError         *Error
		err                   error
		ok                    bool
	)

	if requestUnit == nil {
		return
	}

	if handler.Function != nil {
		if requestUnit.Params != nil {
			if handler.Request != nil {
				requestParamReflect = reflect.New(handler.Request).Elem()
				err = json.Unmarshal(requestUnit.Params, requestParamReflect.Addr().Interface())
				if err == nil {
					requestParamInterface = requestParamReflect.Interface()
				} else {
					responseError = NewErrorInvalidParams(err.Error())
				}
			}
		}

		if responseError == nil {
			responseResult, err = handler.Function(requestParamInterface)
			if responseResult == nil && err == nil {
				err = fmt.Errorf("handler function return nothing")
			}

			if responseResult != nil && err != nil {
				responseResult = nil
			}

			if err != nil {
				responseError, ok = err.(*Error)
				if !ok {
					responseError = NewErrorInternalError(err.Error())
				}
			}
		}
	} else {
		responseError = NewErrorMethodNotFound("handler function is nil")
	}

	if requestUnit.ID != nil && requestUnit.ID != false && requestUnit.ID != true {
		if responseResult != nil {
			responseResultJson, err = json.Marshal(responseResult)
			if err != nil {
				responseError = NewErrorInternalError(err.Error())
			}
		}

		responseUnit = &ResponseUnit{
			JsonRPC: requestUnit.JsonRPC,
			ID:      requestUnit.ID,
			Result:  responseResultJson,
			Error:   responseError,
		}
	}

	return
}

//--------------------------------------------------------------------------------//
// SERVER
//--------------------------------------------------------------------------------//

type Server struct {
	handlerMap map[string]ServerHandlerUnit
}

func (server *Server) HandleFunc(method string, handleFunc ServerHandlerFunc, request interface{}, response interface{}) {
	var (
		handlerRequest  reflect.Type = nil
		handlerResponse reflect.Type = nil
	)

	if request != nil {
		handlerRequest = reflect.TypeOf(request)
	}

	if response != nil {
		handlerResponse = reflect.TypeOf(response)
	}

	server.handlerMap[method] = ServerHandlerUnit{
		Request:  handlerRequest,
		Response: handlerResponse,
		Function: handleFunc,
	}
}

func (server *Server) Execute(requestSlice RequestSlice) (responseSlice ResponseSlice) {
	var (
		requestUnit  *RequestUnit
		handlerUnit  ServerHandlerUnit
		responseUnit *ResponseUnit
		ok           bool
	)

	if requestSlice == nil {
		return
	}

	responseSlice = ResponseSlice{}

	for _, requestUnit = range requestSlice {
		if requestUnit.JsonRPC == "2.0" {
			handlerUnit, ok = server.handlerMap[requestUnit.Method]
			if ok {
				responseUnit = handlerUnit.Execute(requestUnit)
			}
		} else {
			responseSlice = append(responseSlice, &ResponseUnit{JsonRPC: "2.0", Error: NewErrorInvalidRequest(nil)})
		}

		if requestUnit.ID != nil && requestUnit.ID != false && requestUnit.ID != true {
			if ok {
				if responseUnit != nil {
					responseSlice = append(responseSlice, responseUnit)
				} else {
					responseSlice = append(responseSlice, &ResponseUnit{JsonRPC: "2.0", ID: requestUnit.ID, Error: NewErrorInternalError("response is nil")})
				}
			} else {
				responseSlice = append(responseSlice, &ResponseUnit{JsonRPC: "2.0", ID: requestUnit.ID, Error: NewErrorMethodNotFound(fmt.Sprintf(`handler "%s" not founded`, requestUnit.Method))})
			}
		}
	}

	return
}

func (server *Server) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		requestSlice, err := NewRequestSlice(r.Body)

		if err != nil {
			http.Error(rw, NewErrorParseError(err.Error()).Response(), http.StatusOK)
			return
		}

		if len(requestSlice) == 0 {
			http.Error(rw, NewErrorInvalidRequest("request is empty").Response(), http.StatusOK)
			return
		}

		responseSlice := server.Execute(requestSlice)

		if len(responseSlice) > 0 {
			jsonData, err := json.Marshal(responseSlice)

			if err != nil {
				http.Error(rw, NewErrorInternalError(err.Error()).Response(), http.StatusOK)
				return
			}

			rw.Write(jsonData)
		}
	}
}

func NewServer() *Server {
	return &Server{
		handlerMap: map[string]ServerHandlerUnit{},
	}
}

//--------------------------------------------------------------------------------//
