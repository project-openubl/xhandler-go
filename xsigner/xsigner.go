// Package xsigner signs UBL documents with the enveloped RSA-SHA256 XML
// signature SUNAT requires. The signature is placed in the first empty
// ext:ExtensionContent of the document, as rendered by the xbuilder package.
package xsigner

import (
	"context"
	"encoding/json"

	"github.com/project-openubl/xhandler-go/internal/core"
)

// KeyPair is the private key and certificate that sign the documents. The
// key is checked when signing.
type KeyPair struct {
	pem    *pemKey
	pkcs12 *pkcs12Key
}

// MarshalJSON implements json.Marshaler with the form the core expects.
func (k KeyPair) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		PEM    *pemKey    `json:"pem,omitempty"`
		PKCS12 *pkcs12Key `json:"pkcs12,omitempty"`
	}{k.pem, k.pkcs12})
}

type pemKey struct {
	PrivateKey  string `json:"private_key"`
	Certificate string `json:"certificate"`
}

type pkcs12Key struct {
	DER      []byte `json:"der"` // base64 in JSON
	Password string `json:"password"`
}

// FromPEM returns the key pair of a PEM private key (PKCS#8 or PKCS#1) and a
// PEM X.509 certificate.
func FromPEM(privateKey, certificate string) KeyPair {
	return KeyPair{pem: &pemKey{privateKey, certificate}}
}

// FromPKCS12 returns the key pair stored in a PKCS#12 keystore (.pfx, .p12).
func FromPKCS12(der []byte, password string) KeyPair {
	return KeyPair{pkcs12: &pkcs12Key{der, password}}
}

// Sign returns the XML with its signature. The XML may be UTF-8 or
// ISO-8859-1; the signed XML is UTF-8.
func Sign(ctx context.Context, xml []byte, key KeyPair) ([]byte, error) {
	input := struct {
		XML []byte  `json:"xml"`
		Key KeyPair `json:"key"`
	}{xml, key}
	var signed []byte
	err := core.Invoke(ctx, "sign", input, &signed)
	return signed, err
}
