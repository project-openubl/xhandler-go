package xsender_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/project-openubl/xhandler-go/v6"
	"github.com/project-openubl/xhandler-go/v6/internal/conformance"
	"github.com/project-openubl/xhandler-go/v6/xsender"
)

const corpus = "../testdata/conformance"

// sunat answers every request, whatever its host, with handle. A handle
// returning status 0 fails the connection.
type sunat struct {
	mu       sync.Mutex
	requests []*http.Request
	handle   func(r *http.Request, body string) (int, string)
	headers  http.Header // of every answer
}

func (s *sunat) RoundTrip(r *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	s.requests = append(s.requests, r)
	s.mu.Unlock()
	status, answer := s.handle(r, string(body))
	if status == 0 {
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	}
	return &http.Response{StatusCode: status, Header: s.headers, Body: io.NopCloser(strings.NewReader(answer)), Request: r}, nil
}

func (s *sunat) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.requests)
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
	// Only guías have one
	if result.CDR.DocumentDescription != nil {
		t.Errorf("unexpected document description %q", *result.CDR.DocumentDescription)
	}
	if result.SendTarget.URL != xsender.BetaURLs.Invoice || result.VerifyTicketTarget != nil {
		t.Errorf("unexpected targets %+v", result)
	}
	request := server.requests[0]
	if request.Header.Get("SOAPAction") != "urn:sendBill" || request.URL.String() != xsender.BetaURLs.Invoice {
		t.Errorf("unexpected request %s %v", request.URL, request.Header)
	}
}

// The core gives a guía's CDR the URL its printed QR encodes
func TestCDRCarriesTheDocumentDescription(t *testing.T) {
	var cdr xsender.CDR
	err := json.Unmarshal([]byte(`{"zip": "", "response_code": "0", "description": "",
		"notes": [], "status": "ACEPTADO",
		"document_description": "https://e-factura.sunat.gob.pe/v1/contribuyente/gre/comprobantes/descargaqr?hashqr=abc"}`), &cdr)
	if err != nil {
		t.Fatal(err)
	}
	if cdr.DocumentDescription == nil ||
		*cdr.DocumentDescription != "https://e-factura.sunat.gob.pe/v1/contribuyente/gre/comprobantes/descargaqr?hashqr=abc" {
		t.Errorf("unexpected document description %v", cdr.DocumentDescription)
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
		if server.count() == 1 {
			return 401, "<html><title>401 Authorization Required</title></html>"
		}
		return status, body
	}

	result, err := server.sender(xsender.BetaURLs).SendFile(context.Background(), document(t, "F001-1"))
	if err != nil {
		t.Fatal(err)
	}
	if result.CDR == nil || server.count() != 2 {
		t.Fatalf("got %+v after %d requests", result, server.count())
	}
}

func TestFailedConnectionsAreRetriedOnce(t *testing.T) {
	status, body := answer(t, "send_file_cdr")
	server := &sunat{}
	server.handle = func(*http.Request, string) (int, string) {
		if server.count() == 1 {
			return 0, "" // connection refused
		}
		return status, body
	}

	result, err := server.sender(xsender.BetaURLs).SendFile(context.Background(), document(t, "F001-1"))
	if err != nil || result.CDR == nil || server.count() != 2 {
		t.Fatalf("got %+v, %v after %d requests", result, err, server.count())
	}
}

// A token given in the credentials is used as is, and never replaced
func TestASuppliedTokenIsUsedAsIs(t *testing.T) {
	server := &sunat{handle: func(r *http.Request, _ string) (int, string) {
		if r.Header.Get("Authorization") != "Bearer given-token" {
			t.Errorf("unexpected request %s %v", r.URL, r.Header)
		}
		return 401, `{"cod":"401","msg":"Unauthorized"}`
	}}
	sender := server.sender(xsender.ProductionURLs)
	sender.Credentials = xsender.Credentials{Token: "given-token"}

	if _, err := sender.SendFile(context.Background(), document(t, "despatch-advice-T")); err != nil {
		t.Fatal(err)
	}
	if server.count() != 1 {
		t.Fatalf("got %d requests, want 1", server.count())
	}
}

func TestCredentialsHideTheirSecrets(t *testing.T) {
	credentials := xsender.Credentials{Username: "user", Password: "the-password", ClientSecret: "the-secret", Token: "the-token"}
	var logged strings.Builder
	slog.New(slog.NewTextHandler(&logged, nil)).Info("sending", "credentials", credentials)
	printed := fmt.Sprintf("%v %+v %#v %s", credentials, credentials, credentials, logged.String())
	for _, secret := range []string{"the-password", "the-secret", "the-token"} {
		if strings.Contains(printed, secret) {
			t.Fatalf("the credentials show %q: %s", secret, printed)
		}
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

// Guia errors carry the HTTP status and SUNAT's code, from the body or a header
func TestGuiaErrorsCarryBothCodes(t *testing.T) {
	respond := func(r *http.Request, _ string) (int, string) {
		if strings.Contains(r.URL.Path, "/oauth2/token/") {
			return 200, `{"access_token":"the-token","expires_in":3600}`
		}
		return 422, `{"cod":"422","msg":"Unprocessable Entity","errors":[{"cod":"1033","msg":"Registrado previamente"}]}`
	}
	server := &sunat{handle: respond}
	result, err := server.sender(xsender.ProductionURLs).SendFile(context.Background(), document(t, "despatch-advice-T"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Fault == nil || result.Fault.Code != "1033" || result.Fault.HTTPStatus != 422 {
		t.Fatalf("unexpected fault %+v", result.Fault)
	}

	server = &sunat{handle: respond, headers: http.Header{"Cod": {"2033"}}}
	result, err = server.sender(xsender.ProductionURLs).SendFile(context.Background(), document(t, "despatch-advice-T"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Fault == nil || result.Fault.Code != "2033" || result.Fault.Status != xsender.StatusRejected {
		t.Fatalf("unexpected fault %+v", result.Fault)
	}
}
