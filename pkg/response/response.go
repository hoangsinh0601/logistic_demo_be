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

// PaginatedResponse wraps list data with pagination metadata
type PaginatedResponse struct {
	Items      interface{} `json:"items"`
	Page       int         `json:"page"`
	Limit      int         `json:"limit"`
	Total      int64       `json:"total"`
	TotalPages int         `json:"total_pages"`
}

// SuccessWithPagination returns a paginated success response
func SuccessWithPagination(statusCode int, items interface{}, page, limit int, total int64) Response {
	totalPages := int(total) / limit
	if int(total)%limit != 0 {
		totalPages++
	}

	return Response{
		Status:     "success",
		StatusCode: statusCode,
		Data: PaginatedResponse{
			Items:      items,
			Page:       page,
			Limit:      limit,
			Total:      total,
			TotalPages: totalPages,
		},
	}
}
