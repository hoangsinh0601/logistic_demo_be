package response

// Response represents a standard API response format
type Response struct {
	Status     string      `json:"status"`      // "success" or "error"
	StatusCode int         `json:"status_code"` // HTTP status code
	Data       interface{} `json:"data,omitempty"`
	Error      string      `json:"error,omitempty"`
}

// Success returns a standard success response wrapping the data
func Success(statusCode int, data interface{}) Response {
	return Response{
		Status:     "success",
		StatusCode: statusCode,
		Data:       data,
	}
}

// Error returns a standard error response wrapping the error message
func Error(statusCode int, err string) Response {
	return Response{
		Status:     "error",
		StatusCode: statusCode,
		Error:      err,
	}
}
