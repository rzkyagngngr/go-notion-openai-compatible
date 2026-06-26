package errors

import "fmt"

type NotionChatError struct {
	Message    string
	StatusCode int
}

func (e *NotionChatError) Error() string {
	return e.Message
}

func New(message string, statusCode int) *NotionChatError {
	if statusCode == 0 {
		statusCode = 500
	}
	return &NotionChatError{Message: message, StatusCode: statusCode}
}

func HTTPStatus(err error) int {
	if e, ok := err.(*NotionChatError); ok {
		return e.StatusCode
	}
	return 500
}

func Wrapf(format string, args ...any) *NotionChatError {
	return New(fmt.Sprintf(format, args...), 500)
}