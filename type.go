package jsonrpc2

import (
	"encoding/json"
	"fmt"
	"io"
)

//--------------------------------------------------------------------------------//
// ERROR
//--------------------------------------------------------------------------------//

type Error struct {
	Code    int32           `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (err *Error) String() string {
	return fmt.Sprintf(`code: %d; message: "%s"; data: %v`, err.Code, err.Message, string(err.Data))
}

func (err *Error) Error() string {
	return err.String()
}

func (err *Error) Response() string {
	return fmt.Sprintf(`{"jsonrpc": "2.0", "id": null, "error": {"code": %d, "message": "%s", "data": %s}}`, err.Code, err.Message, string(err.Data))
}

//--------------------------------------------------------------------------------//
// REQUEST || NOTIFICATION
//--------------------------------------------------------------------------------//

type RequestByte interface {
	GetRequestByte() ([]byte, error)
	SetRequestByte([]byte) error
}

type RequestUnit struct {
	JsonRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func (requestUnit RequestUnit) GetRequestByte() (output []byte, err error) {
	return json.Marshal(requestUnit)
}

func (requestUnit *RequestUnit) SetRequestByte(input []byte) (err error) {
	return json.Unmarshal(input, requestUnit)
}

type RequestSlice []*RequestUnit

func (requestSlice RequestSlice) GetMap(extRequestSlice *RequestSlice) (requestMap RequestMap) {
	var (
		requestUnit     *RequestUnit
		newRequestSlice RequestSlice
	)

	requestMap = RequestMap{}
	newRequestSlice = RequestSlice{}

	for _, requestUnit = range requestSlice {
		if requestUnit.ID != nil {
			requestMap[requestUnit.ID] = requestUnit
		} else {
			newRequestSlice = append(newRequestSlice, requestUnit)
		}
	}

	if extRequestSlice != nil {
		*extRequestSlice = newRequestSlice
	}

	return
}

func (requestSlice RequestSlice) MarshalJSON() ([]byte, error) {
	if len(requestSlice) == 1 {
		return json.Marshal(requestSlice[0])
	}

	return json.Marshal([]*RequestUnit(requestSlice))
}

func (requestSlice *RequestSlice) UnmarshalJSON(input []byte) (err error) {
	var (
		requestUnit  RequestUnit
		requestArray []*RequestUnit
	)

	err = json.Unmarshal(input, &requestUnit)
	if err == nil {
		requestArray = []*RequestUnit{&requestUnit}
	} else {
		err = json.Unmarshal(input, &requestArray)
		if err != nil {
			return
		}
	}

	*requestSlice = requestArray

	return
}

func (requestSlice RequestSlice) GetRequestByte() (output []byte, err error) {
	return json.Marshal(requestSlice)
}

func (requestSlice *RequestSlice) SetRequestByte(input []byte) (err error) {
	return json.Unmarshal(input, requestSlice)
}

type RequestMap map[interface{}]*RequestUnit

func (requestMap RequestMap) GetSlice(extRequestSlice *RequestSlice) (requestSlice RequestSlice) {
	var requestUnit *RequestUnit

	requestSlice = RequestSlice{}

	for _, requestUnit = range requestMap {
		requestSlice = append(requestSlice, requestUnit)
	}

	if extRequestSlice != nil {
		for _, requestUnit = range *extRequestSlice {
			if requestUnit.ID == nil || requestMap[requestUnit.ID] == nil {
				requestSlice = append(requestSlice, requestUnit)
			}
		}
	}

	return
}

func (requestMap RequestMap) MarshalJSON() ([]byte, error) {
	return requestMap.GetSlice(nil).MarshalJSON()
}

func (requestMap *RequestMap) UnmarshalJSON(input []byte) (err error) {
	var requestSlice RequestSlice

	err = requestSlice.UnmarshalJSON(input)
	if err != nil {
		return
	}

	*requestMap = requestSlice.GetMap(nil)

	return
}

func (requestMap RequestMap) GetRequestByte() (output []byte, err error) {
	return json.Marshal(requestMap)
}

func (requestMap *RequestMap) SetRequestByte(input []byte) (err error) {
	return json.Unmarshal(input, requestMap)
}

func NewRequestSlice(inputInterace interface{}) (requestSlice RequestSlice, err error) {
	requestSlice = RequestSlice{}

	switch inputType := inputInterace.(type) {
	case nil:

	case []byte:
		err = json.Unmarshal(inputType, &requestSlice)
	case io.ReadCloser:
		err = json.NewDecoder(inputType).Decode(&requestSlice)
		if err == io.EOF {
			err = nil
		}
	default:
		err = fmt.Errorf("builder request detect unsupported type '%T'", inputType)
	}

	if err != nil {
		requestSlice = nil
	}

	return
}

//--------------------------------------------------------------------------------//
// RESPONSE
//--------------------------------------------------------------------------------//

type ResponseByte interface {
	GetResponseByte() ([]byte, error)
	SetResponseByte([]byte) error
}

type ResponseUnit struct {
	JsonRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

func (responseUnit ResponseUnit) GetResponseByte() (output []byte, err error) {
	return json.Marshal(responseUnit)
}

func (responseUnit *ResponseUnit) SetResponseByte(input []byte) (err error) {
	return json.Unmarshal(input, responseUnit)
}

type ResponseSlice []*ResponseUnit

func (responseSlice ResponseSlice) GetMap(extResponseSlice *ResponseSlice) (responseMap ResponseMap) {
	var (
		responseUnit     *ResponseUnit
		newResponseSlice ResponseSlice
	)

	responseMap = ResponseMap{}
	newResponseSlice = ResponseSlice{}

	for _, responseUnit = range responseSlice {
		if responseUnit.ID != nil {
			responseMap[responseUnit.ID] = responseUnit
		} else {
			newResponseSlice = append(newResponseSlice, responseUnit)
		}
	}

	if extResponseSlice != nil {
		*extResponseSlice = newResponseSlice
	}

	return
}

func (responseSlice ResponseSlice) MarshalJSON() ([]byte, error) {
	if len(responseSlice) == 1 {
		return json.Marshal(responseSlice[0])
	}

	return json.Marshal([]*ResponseUnit(responseSlice))
}

func (responseSlice *ResponseSlice) UnmarshalJSON(input []byte) (err error) {
	var (
		responseUnit  ResponseUnit
		responseArray []*ResponseUnit
	)

	err = json.Unmarshal(input, &responseUnit)
	if err == nil {
		responseArray = []*ResponseUnit{&responseUnit}
	} else {
		err = json.Unmarshal(input, &responseArray)
		if err != nil {
			return
		}
	}

	*responseSlice = responseArray

	return
}

func (responseSlice ResponseSlice) GetResponseByte() (output []byte, err error) {
	return json.Marshal(responseSlice)
}

func (responseSlice *ResponseSlice) SetResponseByte(input []byte) (err error) {
	return json.Unmarshal(input, responseSlice)
}

type ResponseMap map[interface{}]*ResponseUnit

func (responseMap ResponseMap) GetSlice(extResponseSlice *ResponseSlice) (responseSlice ResponseSlice) {
	var responseUnit *ResponseUnit

	responseSlice = ResponseSlice{}

	for _, responseUnit = range responseMap {
		responseSlice = append(responseSlice, responseUnit)
	}

	if extResponseSlice != nil {
		for _, responseUnit = range *extResponseSlice {
			if responseUnit.ID == nil || responseMap[responseUnit.ID] == nil {
				responseSlice = append(responseSlice, responseUnit)
			}
		}
	}

	return
}

func (responseMap ResponseMap) MarshalJSON() ([]byte, error) {
	return responseMap.GetSlice(nil).MarshalJSON()
}

func (responseMap *ResponseMap) UnmarshalJSON(input []byte) (err error) {
	var responseSlice ResponseSlice

	err = responseSlice.UnmarshalJSON(input)
	if err != nil {
		return
	}

	*responseMap = responseSlice.GetMap(nil)

	return
}

func (responseMap ResponseMap) GetResponseByte() (output []byte, err error) {
	return json.Marshal(responseMap)
}

func (responseMap *ResponseMap) SetResponseByte(input []byte) (err error) {
	return json.Unmarshal(input, responseMap)
}

func NewResponseSlice(inputInterace interface{}) (responseSlice ResponseSlice, err error) {
	responseSlice = ResponseSlice{}

	switch inputType := inputInterace.(type) {
	case nil:

	case []byte:
		err = json.Unmarshal(inputType, &responseSlice)
	case io.ReadCloser:
		err = json.NewDecoder(inputType).Decode(&responseSlice)
		if err == io.EOF {
			err = nil
		}
	default:
		err = fmt.Errorf("builder response detect unsupported type '%T'", inputType)
	}

	if err != nil {
		responseSlice = nil
	}

	return
}

//--------------------------------------------------------------------------------//
