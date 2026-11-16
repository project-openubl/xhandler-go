package xsender

import (
	"fmt"
	"log/slog"
)

// URLs are the SUNAT services where a company sends its documents.
type URLs struct {
	Invoice             string `json:"invoice"`
	PerceptionRetention string `json:"perception_retention"`
	// Despatch is the REST API of the guias de remision; SUNAT has no beta
	// for it.
	Despatch string `json:"despatch"`
}

// BetaURLs are the SUNAT beta (testing) services. SUNAT has no beta for the
// guias de remision, so Despatch is empty.
var BetaURLs = URLs{
	Invoice:             "https://e-beta.sunat.gob.pe/ol-ti-itcpfegem-beta/billService",
	PerceptionRetention: "https://e-beta.sunat.gob.pe/ol-ti-itemision-otroscpe-gem-beta/billService",
}

// ProductionURLs are the SUNAT production services.
var ProductionURLs = URLs{
	Invoice:             "https://e-factura.sunat.gob.pe/ol-ti-itcpfegem/billService",
	PerceptionRetention: "https://e-factura.sunat.gob.pe/ol-ti-itemision-otroscpe-gem/billService",
	Despatch:            "https://api-cpe.sunat.gob.pe/v1/contribuyente/gem",
}

const (
	// BillConsultServiceURL looks up documents already sent (GetStatus, GetStatusCDR).
	BillConsultServiceURL = "https://e-factura.sunat.gob.pe/ol-it-wsconscpegem/billConsultService"
	// BillValidServiceURL validates documents (ValidateCDP, ValidateFile).
	BillValidServiceURL = "https://e-factura.sunat.gob.pe/ol-it-wsconsvalidcpe/billValidService"
)

// Credentials of the company.
type Credentials struct {
	// Username is the SOL user, e.g. "20123456789MODDATOS".
	Username string `json:"username"`
	Password string `json:"password"`
	// ClientID and ClientSecret are the API client of the REST services
	// (guias de remision), from SUNAT's SOL menu.
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
	// Token is a token of the guias obtained elsewhere; when set, it is used
	// as is instead of asking SUNAT for one.
	Token string `json:"-"`
}

// String implements fmt.Stringer without the secrets.
func (c Credentials) String() string {
	return fmt.Sprintf("Credentials{Username: %q, <secrets redacted>}", c.Username)
}

// GoString implements fmt.GoStringer without the secrets.
func (c Credentials) GoString() string { return c.String() }

// LogValue implements slog.LogValuer without the secrets.
func (c Credentials) LogValue() slog.Value {
	return slog.GroupValue(slog.String("username", c.Username))
}

// Target is a SUNAT service for a file or a ticket.
type Target struct {
	Protocol string `json:"protocol"` // "soap" or "rest"
	URL      string `json:"url"`
	// Action of the SOAP files: "bill", "summary" or "pack".
	Action string `json:"action,omitempty"`
}

func (t Target) isREST() bool { return t.Protocol == "rest" }

// File is a zip for SUNAT.
type File struct {
	Filename string `json:"filename"`
	Zip      []byte `json:"zip"` // base64 in JSON
}

// ConsultDocument is a document already sent to SUNAT.
type ConsultDocument struct {
	RUC string `json:"ruc"`
	// TipoComprobante is the Catalog1 code, e.g. "01".
	TipoComprobante string `json:"tipo_comprobante"`
	Serie           string `json:"serie"`
	Numero          uint32 `json:"numero"`
}

// ValidateCDP are the criteria of validaCDPcriterios.
type ValidateCDP struct {
	ConsultDocument
	// TipoDocumentoIdentidadReceptor is the Catalog6 code.
	TipoDocumentoIdentidadReceptor   string `json:"tipo_documento_identidad_receptor"`
	NumeroDocumentoIdentidadReceptor string `json:"numero_documento_identidad_receptor"`
	// FechaEmision is "dd/MM/yyyy".
	FechaEmision       string  `json:"fecha_emision"`
	ImporteTotal       string  `json:"importe_total"`
	NumeroAutorizacion *string `json:"numero_autorizacion"`
}

// SendResult is the answer to a sent file (a CDR, a ticket to verify later
// or a fault) and where it went. The answers (SendFileAnswer, VerifyResult,
// ConsultResult, CDR, Fault...) are generated from the core's schema in
// models_gen.go.
type SendResult struct {
	SendFileAnswer
	// Where the file went and where its ticket is verified.
	SendTarget         Target  `json:"-"`
	VerifyTicketTarget *Target `json:"-"`
}
