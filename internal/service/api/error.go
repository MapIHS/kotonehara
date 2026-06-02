package api

import (
	"encoding/json"
	"fmt"
	"strings"
)

func apiHTTPStatusError(service string, statusCode int, body []byte) error {
	var payload struct {
		Message string      `json:"message"`
		Error   interface{} `json:"error"`
	}

	message := ""
	if len(body) > 0 && json.Unmarshal(body, &payload) == nil {
		message = strings.TrimSpace(payload.Message)
		if message == "" && payload.Error != nil {
			message = strings.TrimSpace(fmt.Sprint(payload.Error))
		}
	}

	if message == "" {
		message = strings.Join(strings.Fields(string(body)), " ")
		if len(message) > 180 {
			message = message[:180]
		}
	}

	if message == "" {
		return fmt.Errorf("%s api http %d", service, statusCode)
	}
	return fmt.Errorf("%s api http %d: %s", service, statusCode, message)
}
