package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
)

func (c *Client) Brat(ctx context.Context, targetURL string) ([]byte, error) {
	u, err := neturl.Parse(c.baseURL)
	if err != nil {
		return nil, err
	}
	u.Path = "/api/brat"

	q := u.Query()
	q.Set("text", targetURL)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "image/*")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("brat api http %d", resp.StatusCode)
	}

	imgBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return imgBytes, nil
}
