package xsender

import (
	"context"

	"github.com/project-openubl/xhandler-go/internal/core"
)

// FileSender sends the signed documents of one company. The document decides
// everything else: the file name, the service and the operation.
type FileSender struct {
	URLs        URLs
	Credentials Credentials
	// Client sends the requests; a FileSender without one uses a default client.
	Client *SunatClient
}

var defaultClient = NewSunatClient(nil)

func (s *FileSender) client() *SunatClient {
	if s.Client != nil {
		return s.Client
	}
	return defaultClient
}

type prepared struct {
	File               File    `json:"file"`
	SendTarget         Target  `json:"send_target"`
	VerifyTicketTarget *Target `json:"verify_ticket_target"`
}

// SendFile zips the signed XML and sends it to its SUNAT service.
func (s *FileSender) SendFile(ctx context.Context, xml []byte) (SendResult, error) {
	var p prepared
	err := core.Invoke(ctx, "prepare_send_file", map[string]any{"xml": xml, "urls": s.URLs}, &p)
	if err != nil {
		return SendResult{}, err
	}
	return s.send(ctx, p)
}

// SendPack sends several invoices in one zip named name (sendPack).
func (s *FileSender) SendPack(ctx context.Context, name string, files [][]byte) (SendResult, error) {
	var p prepared
	input := map[string]any{"name": name, "files": files, "urls": s.URLs}
	if err := core.Invoke(ctx, "prepare_send_pack", input, &p); err != nil {
		return SendResult{}, err
	}
	return s.send(ctx, p)
}

func (s *FileSender) send(ctx context.Context, p prepared) (SendResult, error) {
	result := SendResult{SendTarget: p.SendTarget, VerifyTicketTarget: p.VerifyTicketTarget}
	request := call("send_file", map[string]any{"target": p.SendTarget, "file": p.File})
	err := s.client().exchange(ctx, request, p.SendTarget.isREST(), s.Credentials, &result)
	return result, err
}

// VerifyTicket asks the service target for the status of a ticket.
func (s *FileSender) VerifyTicket(ctx context.Context, target Target, ticket string) (VerifyResult, error) {
	var result VerifyResult
	request := call("verify_ticket", map[string]any{"target": target, "ticket": ticket})
	err := s.client().exchange(ctx, request, target.isREST(), s.Credentials, &result)
	return result, err
}

// VerifyFileTicket is VerifyTicket with the service of the sent document.
func (s *FileSender) VerifyFileTicket(ctx context.Context, xml []byte, ticket string) (VerifyResult, error) {
	var target Target
	err := core.Invoke(ctx, "verify_ticket_target", map[string]any{"xml": xml, "urls": s.URLs}, &target)
	if err != nil {
		return VerifyResult{}, err
	}
	return s.VerifyTicket(ctx, target, ticket)
}
