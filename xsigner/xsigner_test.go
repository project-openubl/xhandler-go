package xsigner_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/project-openubl/xhandler-go/v6"
	"github.com/project-openubl/xhandler-go/v6/internal/conformance"
	"github.com/project-openubl/xhandler-go/v6/xbuilder"
	"github.com/project-openubl/xhandler-go/v6/xsigner"
)

const corpus = "../testdata/conformance"

type caseInput struct {
	XML []byte `json:"xml"`
	Key struct {
		PEM *struct {
			PrivateKey  string `json:"private_key"`
			Certificate string `json:"certificate"`
		} `json:"pem"`
		PKCS12 *struct {
			DER      []byte `json:"der"`
			Password string `json:"password"`
		} `json:"pkcs12"`
	} `json:"key"`
}

func (in caseInput) signingKey() xsigner.SigningKey {
	if in.Key.PEM != nil {
		return xsigner.FromPEM(in.Key.PEM.PrivateKey, in.Key.PEM.Certificate)
	}
	return xsigner.FromPKCS12(in.Key.PKCS12.DER, in.Key.PKCS12.Password)
}

func signCase(t *testing.T, name string) caseInput {
	t.Helper()
	for _, c := range conformance.Cases(t, corpus, "sign") {
		if c.Name == name {
			var in caseInput
			if err := json.Unmarshal(c.Input, &in); err != nil {
				t.Fatal(err)
			}
			return in
		}
	}
	t.Fatalf("no sign case %s", name)
	return caseInput{}
}

func TestKeyPairHidesItsSecrets(t *testing.T) {
	key := signCase(t, "pem/min_data_ruc").signingKey()
	printed := fmt.Sprintf("%v %+v %s %#v", key, key, key, key)
	if strings.Contains(printed, "PRIVATE KEY") {
		t.Fatalf("the signing key shows its private key: %s", printed)
	}
}

// The corpus cases, through the Go API.
func TestSignCorpus(t *testing.T) {
	for _, c := range conformance.Cases(t, corpus, "sign") {
		if strings.HasPrefix(c.Name, "errors/xml_not_base64") {
			continue // not expressible with []byte
		}
		t.Run(c.Name, func(t *testing.T) {
			var in caseInput
			if err := json.Unmarshal(c.Input, &in); err != nil {
				t.Fatal(err)
			}
			signed, err := xsigner.Sign(context.Background(), in.XML, in.signingKey())

			if c.Expected.Error != nil {
				var xerr *xhandler.Error
				if !errors.As(err, &xerr) || string(xerr.Kind) != c.Expected.Error.Kind {
					t.Fatalf("expected error %q, got %v", c.Expected.Error.Kind, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var expected string
			if err := json.Unmarshal(c.Expected.Ok, &expected); err != nil {
				t.Fatal(err)
			}
			if base64.StdEncoding.EncodeToString(signed) != expected {
				t.Fatalf("signed XML differs:\n%s", signed)
			}
		})
	}
}

func TestSignsBuiltDocuments(t *testing.T) {
	invoice := &xbuilder.Invoice{
		SerieNumero: "F001-1",
		Proveedor:   xbuilder.Proveedor{Ruc: "12345678912", RazonSocial: "Softgreen S.A.C."},
		Cliente: xbuilder.Cliente{
			TipoDocumentoIdentidad:   "6",
			NumeroDocumentoIdentidad: "12121212121",
			Nombre:                   "Carlos Feria",
		},
		Detalles: []xbuilder.Detalle{
			{Descripcion: "Item", Cantidad: xbuilder.DecimalFromInt(1), Precio: xbuilder.Ptr(xbuilder.DecimalFromInt(100))},
		},
	}
	xml, err := xbuilder.Build(context.Background(), invoice, xbuilder.Defaults{})
	if err != nil {
		t.Fatal(err)
	}

	key := signCase(t, "pkcs12/llama")
	signed, err := xsigner.Sign(context.Background(), []byte(xml), key.signingKey())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(signed), `<ds:Signature xmlns:ds="http://www.w3.org/2000/09/xmldsig#" Id="PROJECT-OPENUBL-SIGN">`) {
		t.Fatalf("no signature in:\n%s", signed)
	}
}
