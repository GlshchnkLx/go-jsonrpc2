package jsonrpc2

import "encoding/json"

//--------------------------------------------------------------------------------//

func NewError(errorCode int32, errorMessage string, errorData interface{}) *Error {
	var (
		errorDataJson json.RawMessage
		err           error
	)

	if errorData != nil {
		errorDataJson, err = json.Marshal(errorData)
		if err != nil {
			return NewErrorInternalError(nil)
		}
	}

	return &Error{
		Code:    errorCode,
		Message: errorMessage,
		Data:    errorDataJson,
	}
}

func NewErrorParseError(errorData interface{}) (err *Error) {
	return NewError(-32700, "Parse error", errorData)
}

func NewErrorInvalidRequest(errorData interface{}) (err *Error) {
	return NewError(-32600, "Invalid Request", errorData)
}

func NewErrorMethodNotFound(errorData interface{}) (err *Error) {
	return NewError(-32601, "Method not found", errorData)
}

func NewErrorInvalidParams(errorData interface{}) (err *Error) {
	return NewError(-32602, "Invalid params", errorData)
}

func NewErrorInternalError(errorData interface{}) (err *Error) {
	return NewError(-32603, "Internal error", errorData)
}

func NewErrorServerError(errorCodePart int32, errorData interface{}) (err *Error) {
	if errorCodePart < 0 {
		errorCodePart = 0
	}

	if 99 < errorCodePart {
		errorCodePart = 99
	}

	return NewError(-32000-errorCodePart, "Server error", errorData)
}

//--------------------------------------------------------------------------------//
