package httpclient

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestHeaderTimeoutRespectsRequestBudget(t *testing.T) {
	for _, timeout := range []time.Duration{0, 5 * time.Second, 60 * time.Second, 90 * time.Second} {
		for _, socks := range []bool{false, true} {
			var c *http.Client
			if socks {
				var err error
				c, err = newSOCKS5HTTPClient(timeout, "127.0.0.1:1055")
				if err != nil {
					t.Fatal(err)
				}
			} else {
				c = newHTTPClient(timeout)
			}
			tr := c.Transport.(*http.Transport)
			if tr.ResponseHeaderTimeout != timeout || c.Timeout != timeout {
				t.Errorf("socks=%t timeout=%s client=%s headers=%s", socks, timeout, c.Timeout, tr.ResponseHeaderTimeout)
			}
			tr.CloseIdleConnections()
		}
	}
}

func TestSOCKSHandshakeHonorsCancellation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()
	client, err := newSOCKS5HTTPClient(0, listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		conn, err := client.Transport.(*http.Transport).DialContext(ctx, "tcp", "example.com:80")
		if conn != nil {
			conn.Close()
		}
		done <- err
	}()
	select {
	case conn := <-accepted:
		defer conn.Close()
	case <-time.After(time.Second):
		t.Fatal("proxy did not accept connection")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected cancellation error")
		}
	case <-time.After(time.Second):
		t.Fatal("SOCKS handshake ignored cancellation")
	}
}
