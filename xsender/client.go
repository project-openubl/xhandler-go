// Package xsender sends signed UBL documents to SUNAT: the SOAP services
// (billService, consult and validation) and the REST API of the guias de
// remision with its OAuth token.
//
// The xhandler core builds every request and reads every answer (CDR,
// tickets, faults); this package only sends them over HTTP.
package xsender

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/project-openubl/xhandler-go/internal/core"
)

// SunatClient sends the requests to SUNAT and keeps the OAuth tokens of the
// guias de remision. Share one between the FileSenders of several companies.
type SunatClient struct {
	http   *http.Client
	mu     sync.Mutex
	tokens map[string]token
}

type token struct {
	value       string
	reuseBefore time.Time
}

// NewSunatClient returns a client that uses httpClient, or one with a 30s
// connection timeout and a 120s total timeout when nil.
func NewSunatClient(httpClient *http.Client) *SunatClient {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 120 * time.Second,
			Transport: &http.Transport{
				Proxy:       http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{Timeout: 30 * time.Second}).DialContext,
			},
		}
	}
	return &SunatClient{http: httpClient, tokens: map[string]token{}}
}

// GetStatus is billConsultService getStatus: the status of a document already sent.
func (c *SunatClient) GetStatus(ctx context.Context, url string, document ConsultDocument, credentials Credentials) (ConsultResult, error) {
	return c.consult(ctx, call("get_status", map[string]any{"url": url, "document": document}), credentials)
}

// GetStatusCDR is billConsultService getStatusCdr: the CDR of a document already sent.
func (c *SunatClient) GetStatusCDR(ctx context.Context, url string, document ConsultDocument, credentials Credentials) (ConsultResult, error) {
	return c.consult(ctx, call("get_status_cdr", map[string]any{"url": url, "document": document}), credentials)
}

// ValidateCDP is billValidService validaCDPcriterios.
func (c *SunatClient) ValidateCDP(ctx context.Context, url string, cdp ValidateCDP, credentials Credentials) (ConsultResult, error) {
	return c.consult(ctx, call("validate_cdp", map[string]any{"url": url, "cdp": cdp}), credentials)
}

// ValidateFile is billValidService verificaCPEarchivo.
func (c *SunatClient) ValidateFile(ctx context.Context, url string, file File, credentials Credentials) (ConsultResult, error) {
	return c.consult(ctx, call("validate_file", map[string]any{"url": url, "file": file}), credentials)
}

func (c *SunatClient) consult(ctx context.Context, request any, credentials Credentials) (ConsultResult, error) {
	var result ConsultResult
	err := c.exchange(ctx, request, false, credentials, &result)
	return result, err
}

// call is the JSON of a core call, e.g. {"get_status": {...}}.
func call(name string, args map[string]any) map[string]any {
	return map[string]any{name: args}
}

// exchange sends a call and reads its answer into answer. A REST call first
// gets a token; a token SUNAT rejects (HTTP 401) is forgotten.
func (c *SunatClient) exchange(ctx context.Context, request any, rest bool, credentials Credentials, answer any) error {
	var tok *string
	if rest {
		value, err := c.token(ctx, credentials)
		if err != nil {
			return err
		}
		tok = &value
	}
	status, body, err := c.send(ctx, request, credentials, tok)
	if err != nil {
		return err
	}
	if rest && status == http.StatusUnauthorized {
		c.forget(credentials)
	}
	return core.Invoke(ctx, "parse", map[string]any{"call": request, "status": status, "body": body}, answer)
}

// send builds the request of the call with the core and sends it.
func (c *SunatClient) send(ctx context.Context, request any, credentials Credentials, tok *string) (int, string, error) {
	var req struct {
		Method  string      `json:"method"`
		URL     string      `json:"url"`
		Headers [][2]string `json:"headers"`
		Body    *string     `json:"body"`
	}
	input := map[string]any{"call": request, "credentials": credentials, "token": tok}
	if err := core.Invoke(ctx, "request", input, &req); err != nil {
		return 0, "", err
	}

	do := func() (*http.Response, error) {
		var body io.Reader
		if req.Body != nil {
			body = bytes.NewBufferString(*req.Body)
		}
		httpRequest, err := http.NewRequestWithContext(ctx, req.Method, req.URL, body)
		if err != nil {
			return nil, err
		}
		for _, header := range req.Headers {
			httpRequest.Header.Set(header[0], header[1])
		}
		return c.http.Do(httpRequest)
	}
	response, err := do()
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Op == "dial" {
		response, err = do() // once more when the connection failed
	}
	if err != nil {
		return 0, "", fmt.Errorf("xsender: %w", err)
	}
	defer response.Body.Close() //nolint:errcheck // the body was read
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return 0, "", fmt.Errorf("xsender: %w", err)
	}
	return response.StatusCode, string(body), nil
}

// token returns a cached token, reused until a minute before it expires, or
// asks SUNAT for a new one.
func (c *SunatClient) token(ctx context.Context, credentials Credentials) (string, error) {
	key := credentials.ClientID + ":" + credentials.Username
	c.mu.Lock()
	cached, ok := c.tokens[key]
	c.mu.Unlock()
	if ok && time.Now().Before(cached.reuseBefore) {
		return cached.value, nil
	}

	var answer struct {
		Token struct {
			AccessToken string `json:"access_token"`
			ExpiresIn   int64  `json:"expires_in"`
		} `json:"token"`
	}
	if err := c.exchange(ctx, "token", false, credentials, &answer); err != nil {
		return "", err
	}
	lifetime := time.Duration(max(answer.Token.ExpiresIn-60, 0)) * time.Second
	c.mu.Lock()
	c.tokens[key] = token{answer.Token.AccessToken, time.Now().Add(lifetime)}
	c.mu.Unlock()
	return answer.Token.AccessToken, nil
}

func (c *SunatClient) forget(credentials Credentials) {
	c.mu.Lock()
	delete(c.tokens, credentials.ClientID+":"+credentials.Username)
	c.mu.Unlock()
}
