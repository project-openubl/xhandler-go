// Package xsigner signs UBL documents with the enveloped RSA-SHA256 XML
// signature SUNAT requires. The signature is placed in the first empty
// ext:ExtensionContent of the document, as rendered by the xbuilder package.
package xsigner

import (
	"context"

	"github.com/project-openubl/xhandler-go/v6/internal/core"
)

// SigningKey is the private key and certificate that sign the documents. The
// key is checked when signing. It never prints nor marshals its secrets.
type SigningKey struct {
	pem    *pemKey
	pkcs12 *pkcs12Key
}

// String implements fmt.Stringer without the secrets.
func (k SigningKey) String() string {
	return "SigningKey(<redacted>)"
}

type pemKey struct {
	PrivateKey  string `json:"private_key"`
	Certificate string `json:"certificate"`
}

type pkcs12Key struct {
	DER      []byte `json:"der"` // base64 in JSON
	Password string `json:"password"`
}

// FromPEM returns the signing key of a PEM private key (PKCS#8 or PKCS#1) and a
// PEM X.509 certificate.
func FromPEM(privateKey, certificate string) SigningKey {
	return SigningKey{pem: &pemKey{privateKey, certificate}}
}

// FromPKCS12 returns the signing key stored in a PKCS#12 keystore (.pfx, .p12).
func FromPKCS12(der []byte, password string) SigningKey {
	return SigningKey{pkcs12: &pkcs12Key{der, password}}
}

// Sign returns the XML with its signature. The XML may be UTF-8 or
// ISO-8859-1 (as declared in its XML declaration); the signed XML is UTF-8.
func Sign(ctx context.Context, xml []byte, key SigningKey) ([]byte, error) {
	type wireKey struct {
		PEM    *pemKey    `json:"pem,omitempty"`
		PKCS12 *pkcs12Key `json:"pkcs12,omitempty"`
	}
	input := struct {
		XML []byte  `json:"xml"`
		Key wireKey `json:"key"`
	}{xml, wireKey{key.pem, key.pkcs12}}
	var signed []byte
	err := core.Invoke(ctx, "sign", input, &signed)
	return signed, err
}
