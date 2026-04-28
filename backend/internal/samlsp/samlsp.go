// Package samlsp provides the SAML 2.0 Service Provider skeleton.
//
// Actual IdP integration is deferred until the IdP metadata XML is received.
// This package wires up the crewjam/saml library, exposes ACS / metadata
// endpoints, and converts SAML assertions into Topbid JWTs.
package samlsp

import (
	"context"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/crewjam/saml/samlsp"
)

// Config holds SAML SP configuration.
// All values are read from environment variables at startup.
type Config struct {
	// EntityID is the SP entity ID (e.g. "https://topbid.example.com/saml").
	EntityID string
	// RootURL is the application root URL used for ACS/SLO callbacks.
	RootURL string
	// CertPath is the path to the SP certificate PEM file.
	CertPath string
	// KeyPath is the path to the SP private key PEM file.
	KeyPath string
	// IdPMetadataURL is the IdP metadata endpoint. Empty = SAML disabled.
	IdPMetadataURL string
}

// Middleware wraps the crewjam/saml middleware and provides hooks
// for Topbid-specific user provisioning.
type Middleware struct {
	inner *samlsp.Middleware
}

// New creates a SAML SP middleware from the given config.
// Returns an error if certificates cannot be loaded or IdP metadata
// cannot be fetched.
func New(cfg *Config) (*Middleware, error) {
	keyPair, err := tls.LoadX509KeyPair(cfg.CertPath, cfg.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("load SAML keypair: %w", err)
	}

	leaf, err := x509.ParseCertificate(keyPair.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("parse SAML leaf cert: %w", err)
	}

	rsaKey, ok := keyPair.PrivateKey.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("SAML key must be RSA")
	}

	idpMetadataURL, err := url.Parse(cfg.IdPMetadataURL)
	if err != nil {
		return nil, fmt.Errorf("parse IdP metadata URL: %w", err)
	}

	idpMetadata, err := samlsp.FetchMetadata(
		context.Background(),
		http.DefaultClient,
		*idpMetadataURL,
	)
	if err != nil {
		return nil, fmt.Errorf("fetch IdP metadata: %w", err)
	}

	rootURL, err := url.Parse(cfg.RootURL)
	if err != nil {
		return nil, fmt.Errorf("parse root URL: %w", err)
	}

	sp, err := samlsp.New(samlsp.Options{
		EntityID:    cfg.EntityID,
		URL:         *rootURL,
		Key:         rsaKey,
		Certificate: leaf,
		IDPMetadata: idpMetadata,
	})
	if err != nil {
		return nil, fmt.Errorf("create SAML SP: %w", err)
	}

	slog.Info("SAML SP initialized",
		"entity_id", cfg.EntityID,
		"acs_url", rootURL.String()+"/saml/acs",
	)

	return &Middleware{inner: sp}, nil
}

// Handler returns the http.Handler that serves SAML metadata and ACS endpoints.
// Mount this at /saml/ in the router.
//
//	r.Handle("/saml/*", sp.Handler())
func (m *Middleware) Handler() http.Handler {
	return m.inner
}

// RequireAccount returns middleware that requires SAML authentication.
// For routes behind this middleware, unauthenticated users are redirected
// to the IdP login page.
func (m *Middleware) RequireAccount() func(http.Handler) http.Handler {
	return m.inner.RequireAccount
}
