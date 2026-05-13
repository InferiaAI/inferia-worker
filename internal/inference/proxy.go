// Package inference exposes the worker's HTTP proxy for /v1/* requests.
// Incoming auth has already been checked by the auth middleware; this layer
// resolves the target deployment and forwards (streaming-aware) to the local
// model container.
package inference

import (
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// EndpointResolver returns the base URL of the local model container for a
// deployment, or "" if not loaded. Implemented by *runtime.Runtime.
type EndpointResolver interface {
	EndpointURL(deploymentID string) string
}

// Config wires up the proxy.
type Config struct {
	Runtime  EndpointResolver
	Resolver PathResolver
}

// PathResolver decides which deployment a request targets. If Resolver is set
// it wins; otherwise the named Header is consulted; otherwise "".
type PathResolver struct {
	Header   string
	Resolver func(*fiber.Ctx) string
}

// Resolve picks the deployment id for the current request.
func (p PathResolver) Resolve(c *fiber.Ctx) string {
	if p.Resolver != nil {
		return p.Resolver(c)
	}
	if p.Header != "" {
		return c.Get(p.Header)
	}
	return ""
}

// httpClient is the upstream client. A bare *http.Client is sufficient — we
// stream the response body verbatim. Long timeout because LLM generations can
// take minutes.
var httpClient = &http.Client{
	Timeout: 30 * time.Minute,
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		// Don't buffer streaming bodies; flush as bytes arrive.
		DisableCompression:    true,
		MaxIdleConns:          20,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 0, // unlimited; some LLMs are slow to first token
	},
}

// NewProxy returns a Fiber handler that forwards /v1/* to the resolved
// endpoint. Other paths pass through untouched (so /healthz, /metrics, etc.
// remain locally served).
func NewProxy(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if !strings.HasPrefix(c.Path(), "/v1/") {
			return c.Next()
		}
		deploymentID := cfg.Resolver.Resolve(c)
		base := ""
		if deploymentID != "" {
			base = cfg.Runtime.EndpointURL(deploymentID)
		}
		if base == "" {
			c.Set("Retry-After", "5")
			return c.Status(fiber.StatusServiceUnavailable).SendString("deployment not loaded")
		}

		// Build upstream URL.
		path := c.Path()
		query := string(c.Request().URI().QueryString())
		url := strings.TrimRight(base, "/") + path
		if query != "" {
			url += "?" + query
		}

		// Build upstream request, copying method, body, and headers.
		body := c.Body()
		upstreamReq, err := http.NewRequestWithContext(c.UserContext(), c.Method(), url, strings.NewReader(string(body)))
		if err != nil {
			return c.Status(fiber.StatusBadGateway).SendString(err.Error())
		}
		c.Request().Header.VisitAll(func(k, v []byte) {
			key := string(k)
			// Hop-by-hop headers should not be forwarded.
			if isHopByHop(key) {
				return
			}
			upstreamReq.Header.Add(key, string(v))
		})

		resp, err := httpClient.Do(upstreamReq)
		if err != nil {
			return c.Status(fiber.StatusBadGateway).SendString("upstream: " + err.Error())
		}
		// Copy status + headers (excluding hop-by-hop).
		for k, vs := range resp.Header {
			if isHopByHop(k) {
				continue
			}
			for _, v := range vs {
				c.Response().Header.Add(k, v)
			}
		}
		c.Status(resp.StatusCode)

		// Streaming write: Fiber/fasthttp uses SetBodyStream to forward chunks
		// without buffering the whole response.
		c.Context().SetBodyStream(&streamReader{r: resp.Body}, -1)
		return nil
	}
}

// streamReader forwards reads from the upstream body. fasthttp will call
// Close() once it finishes streaming, so we don't close on intermediate errors.
type streamReader struct{ r io.ReadCloser }

func (s *streamReader) Read(p []byte) (int, error) { return s.r.Read(p) }
func (s *streamReader) Close() error               { return s.r.Close() }

var hopByHop = map[string]struct{}{
	"connection":          {},
	"keep-alive":          {},
	"proxy-authenticate":  {},
	"proxy-authorization": {},
	"te":                  {},
	"trailers":            {},
	"transfer-encoding":   {},
	"upgrade":             {},
}

func isHopByHop(h string) bool {
	_, ok := hopByHop[strings.ToLower(h)]
	return ok
}
