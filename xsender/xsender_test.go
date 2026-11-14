package xsender_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/project-openubl/xhandler-go"
	"github.com/project-openubl/xhandler-go/internal/conformance"
	"github.com/project-openubl/xhandler-go/xsender"
)

const corpus = "../testdata/conformance"

// sunat answers every request, whatever its host, with handle.
type sunat struct {
	mu       sync.Mutex
	requests []*http.Request
	handle   func(r *http.Request, body string) (int, string)
}

func (s *sunat) RoundTrip(r *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	s.requests = append(s.requests, r)
	s.mu.Unlock()
	status, answer := s.handle(r, string(body))
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(answer)), Request: r}, nil
}

func (s *sunat) sender(urls xsender.URLs) *xsender.FileSender {
	return &xsender.FileSender{
		URLs: urls,
		Credentials: xsender.Credentials{
			Username: "12345678959MODDATOS", Password: "MODDATOS",
			ClientID: "client-id", ClientSecret: "client-secret",
		},
		Client: xsender.NewSunatClient(&http.Client{Transport: s}),
	}
}

// corpusCase returns the input of a corpus case, e.g. ("parse", "send_file_cdr").
func corpusCase(t *testing.T, op, name string) conformance.Case {
	t.Helper()
	for _, c := range conformance.Cases(t, corpus, op) {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no corpus case %s/%s", op, name)
	return conformance.Case{}
}

// answer is the SUNAT answer (status and body) recorded in a parse case.
func answer(t *testing.T, name string) (int, string) {
	var in struct {
		Status int    `json:"status"`
		Body   string `json:"body"`
	}
	if err := json.Unmarshal(corpusCase(t, "parse", name).Input, &in); err != nil {
		t.Fatal(err)
	}
	return in.Status, in.Body
}

// document is a UBL file of the corpus, e.g. "F001-1".
func document(t *testing.T, name string) []byte {
	var in struct {
		XML []byte `json:"xml"`
	}
	if err := json.Unmarshal(corpusCase(t, "prepare_send_file", name).Input, &in); err != nil {
		t.Fatal(err)
	}
	return in.XML
}

func TestSendFileReadsTheCDR(t *testing.T) {
	status, body := answer(t, "send_file_cdr")
	server := &sunat{handle: func(*http.Request, string) (int, string) { return status, body }}

	result, err := server.sender(xsender.BetaURLs).SendFile(context.Background(), document(t, "F001-1"))
	if err != nil {
		t.Fatal(err)
	}
	if result.CDR == nil || result.CDR.Status != xsender.StatusAccepted || len(result.CDR.Zip) == 0 {
		t.Fatalf("unexpected result %+v", result)
	}
	if result.SendTarget.URL != xsender.BetaURLs.Invoice || result.VerifyTicketTarget != nil {
		t.Errorf("unexpected targets %+v", result)
	}
	request := server.requests[0]
	if request.Header.Get("SOAPAction") != "urn:sendBill" || request.URL.String() != xsender.BetaURLs.Invoice {
		t.Errorf("unexpected request %s %v", request.URL, request.Header)
	}
}

// SUNAT answers SOAP faults with HTTP 500
func TestSendFileReadsFaults(t *testing.T) {
	status, body := answer(t, "send_file_rejected")
	server := &sunat{handle: func(*http.Request, string) (int, string) { return status, body }}

	result, err := server.sender(xsender.BetaURLs).SendFile(context.Background(), document(t, "F001-1"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Fault == nil || result.Fault.Status != xsender.StatusRejected {
		t.Fatalf("unexpected result %+v", result)
	}
}

func TestUnreadableAnswersAreResponseErrors(t *testing.T) {
	server := &sunat{handle: func(*http.Request, string) (int, string) { return 503, "<html>Service Unavailable</html>" }}

	_, err := server.sender(xsender.BetaURLs).SendFile(context.Background(), document(t, "F001-1"))
	var xerr *xhandler.Error
	if !errors.As(err, &xerr) || xerr.Kind != xhandler.KindResponse {
		t.Fatalf("expected a response error, got %v", err)
	}
}

func TestVerifyFileTicketInProgress(t *testing.T) {
	status, body := answer(t, "verify_ticket_in_progress")
	server := &sunat{handle: func(*http.Request, string) (int, string) { return status, body }}

	result, err := server.sender(xsender.BetaURLs).VerifyFileTicket(context.Background(), document(t, "RA-20200328-1"), "1703154974517")
	if err != nil {
		t.Fatal(err)
	}
	if result.NoCDR == nil || result.NoCDR.Status != xsender.StatusInProgress {
		t.Fatalf("unexpected result %+v", result)
	}
}

// SUNAT's front proxy answers some requests with a 401 page: they are retried
func TestRateLimitedRequestsAreRetried(t *testing.T) {
	status, body := answer(t, "send_file_cdr")
	server := &sunat{}
	server.handle = func(*http.Request, string) (int, string) {
		if len(server.requests) == 1 {
			return 401, "<html><title>401 Authorization Required</title></html>"
		}
		return status, body
	}

	result, err := server.sender(xsender.BetaURLs).SendFile(context.Background(), document(t, "F001-1"))
	if err != nil {
		t.Fatal(err)
	}
	if result.CDR == nil || len(server.requests) != 2 {
		t.Fatalf("got %+v after %d requests", result, len(server.requests))
	}
}

// Guias de remision: one token is reused until SUNAT rejects it (HTTP 401),
// then the call is retried with a new one
func TestGuiasReuseTheTokenUntilRejected(t *testing.T) {
	var tokens, sends int
	server := &sunat{handle: func(r *http.Request, body string) (int, string) {
		if strings.Contains(r.URL.Path, "/oauth2/token/") {
			tokens++
			if !strings.Contains(body, "grant_type=password") {
				t.Errorf("unexpected token request %s", body)
			}
			return 200, `{"access_token":"the-token","expires_in":3600}`
		}
		sends++
		if r.Header.Get("Authorization") != "Bearer the-token" {
			t.Errorf("unexpected authorization %q", r.Header.Get("Authorization"))
		}
		if sends == 2 {
			return 401, `{"cod":"401","msg":"Unauthorized"}`
		}
		return 200, `{"numTicket":"a7b3c1d2-0001"}`
	}}
	sender := server.sender(xsender.ProductionURLs)
	guia := document(t, "despatch-advice-T")

	for range 3 {
		result, err := sender.SendFile(context.Background(), guia)
		if err != nil {
			t.Fatal(err)
		}
		if result.Ticket == nil {
			t.Fatalf("expected a ticket, got %+v", result)
		}
	}
	// The second send is rejected once and retried with a new token
	if tokens != 2 || sends != 4 {
		t.Fatalf("got %d tokens for %d sends, want 2 for 4", tokens, sends)
	}
}

func TestTheSecurityAPIURLCanBeSet(t *testing.T) {
	var tokenURL string
	server := &sunat{handle: func(r *http.Request, _ string) (int, string) {
		if strings.Contains(r.URL.Path, "/oauth2/token/") {
			tokenURL = r.URL.String()
			return 200, `{"access_token":"the-token","expires_in":3600}`
		}
		return 200, `{"numTicket":"a7b3c1d2-0001"}`
	}}
	sender := server.sender(xsender.ProductionURLs)
	sender.Client.SecurityAPIURL = "http://localhost:8080"

	if _, err := sender.SendFile(context.Background(), document(t, "despatch-advice-T")); err != nil {
		t.Fatal(err)
	}
	if tokenURL != "http://localhost:8080/v1/clientessol/client-id/oauth2/token/" {
		t.Fatalf("token requested at %s", tokenURL)
	}
}

func TestGetStatusCDR(t *testing.T) {
	status, body := answer(t, "get_status_cdr")
	server := &sunat{handle: func(*http.Request, string) (int, string) { return status, body }}
	client := xsender.NewSunatClient(&http.Client{Transport: server})

	document := xsender.ConsultDocument{RUC: "12345678912", TipoComprobante: "01", Serie: "F001", Numero: 1}
	result, err := client.GetStatusCDR(context.Background(), xsender.BillConsultServiceURL, document, xsender.Credentials{Username: "u", Password: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status == nil || result.Status.StatusCode != "0004" || base64.StdEncoding.EncodeToString(result.Status.CDRZip) != "UEsDBA==" {
		t.Fatalf("unexpected result %+v", result.Status)
	}
	if server.requests[0].Header.Get("SOAPAction") != "urn:getStatusCdr" {
		t.Errorf("unexpected SOAPAction %q", server.requests[0].Header.Get("SOAPAction"))
	}
}
