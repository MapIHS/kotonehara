package api

import (
	"context"
	"net/http"
	"net/url"
)

func (c *Client) profileRequest(ctx context.Context, path string, query url.Values, out any) error {
	endpoint, err := c.jobURL(path)
	if err != nil {
		return err
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return err
	}
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return readAPIError("profile", resp)
	}
	return decodeAPIResponse(resp, out)
}
