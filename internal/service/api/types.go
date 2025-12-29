package api

type APIResponse[T any] struct {
	Code    int         `json:"code"`
	Status  string      `json:"status"`
	Message string      `json:"message"`
	Data    T           `json:"data,omitempty"`
	Error   interface{} `json:"error,omitempty"`
}
