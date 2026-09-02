package weixin

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestClient starts an httptest.Server serving handler and returns a
// Client pointed at it plus a teardown func.
func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
}
