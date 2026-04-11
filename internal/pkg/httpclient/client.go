package httpclient

import (
	"net/http"
	"time"
)

// NewClient creates an HTTP client with the specified timeout.
func NewClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
	}
}

// NewAuthenticatedTransport creates an http.RoundTripper that adds
// authentication header to requests.
func NewAuthenticatedTransport(apiKey, headerName string) http.RoundTripper {
	return &authTransport{
		apiKey:     apiKey,
		headerName: headerName,
		transport:  http.DefaultTransport,
	}
}

type authTransport struct {
	apiKey     string
	headerName string
	transport  http.RoundTripper
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	if t.headerName != "" && t.apiKey != "" {
		req.Header.Set(t.headerName, t.apiKey)
	}
	return t.transport.RoundTrip(req)
}
