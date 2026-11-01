package oidc

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
)

// Keys holds the provider's RS256 signing key and the public keys it publishes.
type Keys struct {
	signing jwk.Key
	public  jwk.Set
}

// LoadKeys loads the signing key (from SigningKey or SigningKeyFile) and the optional previous
// key. Without a configured key a fresh one is generated, which is only suitable for dev: tokens
// stop verifying after a restart.
func LoadKeys(c *Config) (*Keys, error) {
	pemData := []byte(c.SigningKey)
	if len(pemData) == 0 && c.SigningKeyFile != "" {
		b, err := os.ReadFile(c.SigningKeyFile)
		if err != nil {
			return nil, fmt.Errorf("oidc signing key: %w", err)
		}
		pemData = b
	}
	var priv *rsa.PrivateKey
	var err error
	if len(pemData) == 0 {
		log.Printf("oidc: no signing key configured (oidc.signing_key_file or OIDC_SIGNING_KEY); generated a temporary one, so tokens won't verify after a restart")
		priv, err = rsa.GenerateKey(rand.Reader, 2048)
	} else {
		priv, err = parseRSAPrivateKey(pemData)
	}
	if err != nil {
		return nil, fmt.Errorf("oidc signing key: %w", err)
	}
	signing, err := rsaJWK(priv)
	if err != nil {
		return nil, err
	}
	public := jwk.NewSet()
	pub, err := signing.PublicKey()
	if err != nil {
		return nil, err
	}
	_ = public.AddKey(pub)
	if c.PreviousSigningKeyFile != "" {
		b, err := os.ReadFile(c.PreviousSigningKeyFile)
		if err != nil {
			return nil, fmt.Errorf("oidc previous signing key: %w", err)
		}
		prevPriv, err := parseRSAPrivateKey(b)
		if err != nil {
			return nil, fmt.Errorf("oidc previous signing key: %w", err)
		}
		prev, err := rsaJWK(prevPriv)
		if err != nil {
			return nil, err
		}
		prevPub, err := prev.PublicKey()
		if err != nil {
			return nil, err
		}
		_ = public.AddKey(prevPub)
	}
	return &Keys{signing: signing, public: public}, nil
}

func rsaJWK(priv *rsa.PrivateKey) (jwk.Key, error) {
	if priv.N.BitLen() < 2048 {
		return nil, errors.New("oidc signing key must be at least 2048 bits")
	}
	k, err := jwk.FromRaw(priv)
	if err != nil {
		return nil, err
	}
	if err := jwk.AssignKeyID(k, jwk.WithThumbprintHash(crypto.SHA256)); err != nil {
		return nil, err
	}
	_ = k.Set(jwk.AlgorithmKey, jwa.RS256)
	_ = k.Set(jwk.KeyUsageKey, jwk.ForSignature)
	return k, nil
}

func parseRSAPrivateKey(data []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("no PEM block found")
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	rk, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("signing key must be RSA")
	}
	return rk, nil
}

// GenerateKeyPEM returns a new 2048-bit RSA private key in PKCS#8 PEM form.
func GenerateKeyPEM() ([]byte, error) {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}
