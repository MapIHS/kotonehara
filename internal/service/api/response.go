package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

const (
	maxAPIResponseSize = 4 << 20
	maxMediaSize       = 256 << 20
)

var errResponseTooLarge = errors.New("response body melebihi batas")

func readResponseBody(resp *http.Response, limit int64) ([]byte, error) {
	if resp.ContentLength > limit {
		return nil, fmt.Errorf("%w %d byte", errResponseTooLarge, limit)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("%w %d byte", errResponseTooLarge, limit)
	}
	return body, nil
}

func decodeAPIResponse(resp *http.Response, dst any) error {
	body, err := readResponseBody(resp, maxAPIResponseSize)
	if err != nil {
		return err
	}
	return json.NewDecoder(bytes.NewReader(body)).Decode(dst)
}
