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

// Status is the SUNAT verdict of a document.
type Status string

const (
	StatusAccepted   Status = "ACEPTADO"
	StatusRejected   Status = "RECHAZADO"
	StatusException  Status = "EXCEPCION"
	StatusInProgress Status = "EN_PROCESO"
	StatusUnknown    Status = "UNKNOWN"
)

// CDR is the receipt SUNAT gives back (Constancia de Recepcion).
type CDR struct {
	Zip          []byte   `json:"zip"`
	ResponseCode string   `json:"response_code"`
	Description  string   `json:"description"`
	Notes        []string `json:"notes"`
	Status       Status   `json:"status"`
	// StatusCode is the ticket status, only when verifying a ticket.
	StatusCode string `json:"status_code,omitempty"`
}

// Fault is an error answered by SUNAT.
type Fault struct {
	// Code is SUNAT's raw code, e.g. "soap-env:Client.0111".
	Code string `json:"code"`
	// SunatCode is the number in Code (111), when it has one.
	SunatCode *uint32 `json:"sunat_code"`
	Message   string  `json:"message"`
	Status    Status  `json:"status"`
}

// SendResult is the answer to a sent file: a CDR, a ticket to verify later
// (summaries, voided documents, packs, guias) or a fault.
type SendResult struct {
	CDR    *CDR    `json:"cdr"`
	Ticket *string `json:"ticket"`
	Fault  *Fault  `json:"fault"`
	// Where the file went and where its ticket is verified.
	SendTarget         Target  `json:"-"`
	VerifyTicketTarget *Target `json:"-"`
}

// VerifyResult is the answer to a ticket: a CDR, no CDR yet (e.g. still in
// progress) or a fault.
type VerifyResult struct {
	CDR   *CDR   `json:"cdr"`
	NoCDR *NoCDR `json:"no_cdr"`
	Fault *Fault `json:"fault"`
}

// NoCDR is a ticket status without CDR, e.g. "98" (in progress).
type NoCDR struct {
	StatusCode string `json:"status_code"`
	Status     Status `json:"status"`
}

// ConsultResult is the answer of the consult and validation services.
type ConsultResult struct {
	Status *ConsultStatus `json:"status"`
	Fault  *Fault         `json:"fault"`
}

// ConsultStatus is e.g. "0001" (accepted) for GetStatus, or "0" for ValidateCDP.
type ConsultStatus struct {
	StatusCode    string  `json:"status_code"`
	StatusMessage *string `json:"status_message"`
	// CDRZip is the CDR of GetStatusCDR.
	CDRZip []byte `json:"cdr_zip"`
}
