// Package e2e creates, signs and sends an invoice to SUNAT beta. It needs the
// network, so it only runs with XHANDLER_E2E=true.
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/project-openubl/xhandler-go/internal/conformance"
	"github.com/project-openubl/xhandler-go/xbuilder"
	"github.com/project-openubl/xhandler-go/xsender"
	"github.com/project-openubl/xhandler-go/xsigner"
)

func TestCreateSignAndSendToSunatBeta(t *testing.T) {
	if os.Getenv("XHANDLER_E2E") != "true" {
		t.Skip("set XHANDLER_E2E=true to send to SUNAT beta")
	}
	ctx := context.Background()

	invoice := &xbuilder.Invoice{
		// A new number each run: SUNAT beta remembers the documents
		SerieNumero: fmt.Sprintf("F001-%d", time.Now().Unix()%100_000_000),
		Proveedor:   xbuilder.Proveedor{Ruc: "12345678912", RazonSocial: "Softgreen S.A.C."},
		Cliente: xbuilder.Cliente{
			TipoDocumentoIdentidad:   "6",
			NumeroDocumentoIdentidad: "12121212121",
			Nombre:                   "Carlos Feria",
		},
		Detalles: []xbuilder.Detalle{
			{Descripcion: "Item", Cantidad: xbuilder.DecimalFromInt(2), Precio: xbuilder.Ptr(xbuilder.DecimalFromInt(100))},
		},
	}
	xml, err := xbuilder.Build(ctx, invoice, xbuilder.Defaults{})
	if err != nil {
		t.Fatal(err)
	}

	signed, err := xsigner.Sign(ctx, []byte(xml), testKeyPair(t))
	if err != nil {
		t.Fatal(err)
	}

	sender := &xsender.FileSender{
		URLs:        xsender.BetaURLs,
		Credentials: xsender.Credentials{Username: "12345678959MODDATOS", Password: "MODDATOS"},
	}
	result, err := sender.SendFile(ctx, signed)
	if err != nil {
		t.Fatal(err)
	}
	if result.CDR == nil || result.CDR.ResponseCode != "0" || result.CDR.Status != xsender.StatusAccepted {
		t.Fatalf("SUNAT did not accept the invoice: %+v %+v", result.CDR, result.Fault)
	}
}

// testKeyPair is the test certificate of the conformance corpus, the one
// SUNAT beta accepts for RUC 12345678912.
func testKeyPair(t *testing.T) xsigner.KeyPair {
	for _, c := range conformance.Cases(t, "../testdata/conformance", "sign") {
		if c.Name != "pem/min_data_ruc" {
			continue
		}
		var in struct {
			Key struct {
				PEM struct {
					PrivateKey  string `json:"private_key"`
					Certificate string `json:"certificate"`
				} `json:"pem"`
			} `json:"key"`
		}
		if err := json.Unmarshal(c.Input, &in); err != nil {
			t.Fatal(err)
		}
		return xsigner.FromPEM(in.Key.PEM.PrivateKey, in.Key.PEM.Certificate)
	}
	t.Fatal("no test key in the corpus")
	return xsigner.KeyPair{}
}
