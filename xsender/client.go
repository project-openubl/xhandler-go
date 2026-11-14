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
	// SecurityAPIURL is where the tokens of the guias are requested; SUNAT's
	// when empty. Set it before the first call.
	SecurityAPIURL string

	http   *http.Client
	mu     sync.Mutex // guards tokens
	tokens map[string]token
	// requesting is held while a token is requested, so concurrent calls
	// don't revoke each other's tokens
	requesting sync.Mutex
}

// Pauses before retrying a request SUNAT's front proxy refused (rate limiting).
var rateLimitPauses = []time.Duration{time.Second, 2 * time.Second}

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

// exchange sends a call and reads its answer into answer. SUNAT revokes the
// tokens of the guias before they expire: a REST call rejected with HTTP 401
// is retried once with a new token. A SOAP call refused by SUNAT's rate
// limiting is retried after a pause.
func (c *SunatClient) exchange(ctx context.Context, request any, rest bool, credentials Credentials, answer any) error {
	if rest {
		tok, status, body, err := c.sendWithToken(ctx, request, credentials)
		if err == nil && status == http.StatusUnauthorized && credentials.Token == "" {
			c.forget(credentials, tok)
			_, status, body, err = c.sendWithToken(ctx, request, credentials)
		}
		if err != nil {
			return err
		}
		return parse(ctx, request, status, body, answer)
	}

	for attempt := 0; ; attempt++ {
		status, body, err := c.send(ctx, request, credentials, nil)
		if err != nil {
			return err
		}
		err = parse(ctx, request, status, body, answer)
		var xerr *core.Error
		if attempt == len(rateLimitPauses) || !errors.As(err, &xerr) || xerr.Kind != "rate_limited" {
			return err
		}
		select {
		case <-time.After(rateLimitPauses[attempt]):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func parse(ctx context.Context, request any, status int, body string, answer any) error {
	return core.Invoke(ctx, "parse", map[string]any{"call": request, "status": status, "body": body}, answer)
}

// sendWithToken sends a REST call and returns the token it used.
func (c *SunatClient) sendWithToken(ctx context.Context, request any, credentials Credentials) (string, int, string, error) {
	tok, err := c.token(ctx, credentials)
	if err != nil {
		return "", 0, "", err
	}
	status, body, err := c.send(ctx, request, credentials, &tok)
	return tok, status, body, err
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
	if connectionFailed(err) {
		response, err = do() // once more: the request never reached SUNAT
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

// connectionFailed tells the errors of requests that never reached SUNAT:
// the connection (DNS included) or the proxy connection failed or timed out.
func connectionFailed(err error) bool {
	var opErr *net.OpError
	return errors.As(err, &opErr) && (opErr.Op == "dial" || opErr.Op == "proxyconnect")
}

// token returns the token of the credentials, a cached one (reused until a
// minute before it expires), or asks SUNAT for a new one. One token is
// requested at a time.
func (c *SunatClient) token(ctx context.Context, credentials Credentials) (string, error) {
	if credentials.Token != "" {
		return credentials.Token, nil
	}
	key := tokenKey(credentials)
	if tok, ok := c.cachedToken(key); ok {
		return tok, nil
	}
	c.requesting.Lock()
	defer c.requesting.Unlock()
	if tok, ok := c.cachedToken(key); ok {
		return tok, nil
	}

	var answer struct {
		Token struct {
			AccessToken string `json:"access_token"`
			ExpiresIn   int64  `json:"expires_in"`
		} `json:"token"`
	}
	request := call("token", map[string]any{"url": nilIfEmpty(c.SecurityAPIURL)})
	if err := c.exchange(ctx, request, false, credentials, &answer); err != nil {
		return "", err
	}
	lifetime := time.Duration(max(answer.Token.ExpiresIn-60, 0)) * time.Second
	c.mu.Lock()
	c.tokens[key] = token{answer.Token.AccessToken, time.Now().Add(lifetime)}
	c.mu.Unlock()
	return answer.Token.AccessToken, nil
}

func tokenKey(credentials Credentials) string {
	return credentials.ClientID + ":" + credentials.Username
}

func (c *SunatClient) cachedToken(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cached, ok := c.tokens[key]
	return cached.value, ok && time.Now().Before(cached.reuseBefore)
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// forget drops the rejected token, unless another call already replaced it.
func (c *SunatClient) forget(credentials Credentials, rejected string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if key := tokenKey(credentials); c.tokens[key].value == rejected {
		delete(c.tokens, key)
	}
}
