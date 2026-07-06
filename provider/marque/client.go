// Package marque wraps the atproto XRPC endpoints Marque uses to store
// domain and DNS records.
//
// Marque doesn't have a bespoke REST API — DNS zones are just atproto
// records in the user's PDS under the `at.marque.dns` collection, and the
// registration itself is `at.marque.domain`. We manage them with the
// standard com.atproto.repo XRPC methods (getRecord/putRecord/deleteRecord).
package marque

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Collections managed / referenced by this provider.
const (
	CollectionDns    = "at.marque.dns"
	CollectionDomain = "at.marque.domain"
)

// Client is a minimal atproto XRPC client. It authenticates once via
// com.atproto.server.createSession and reuses the resulting accessJwt.
type Client struct {
	http     *http.Client
	service  string
	accessJwt string
	did       string
	handle    string
}

// APIError is returned when an XRPC call responds with a non-2xx status.
type APIError struct {
	Status int
	Method string
	Body   string
	Kind   string // atproto "error" field, e.g. "InvalidRequest"
}

func (e *APIError) Error() string {
	if e.Kind != "" {
		return fmt.Sprintf("marque: %s %s (%d): %s", e.Method, e.Kind, e.Status, strings.TrimSpace(e.Body))
	}
	return fmt.Sprintf("marque: %s failed (%d): %s", e.Method, e.Status, strings.TrimSpace(e.Body))
}

// IsNotFound reports whether err is a "RecordNotFound" style error.
func IsNotFound(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	if apiErr.Status == http.StatusNotFound {
		return true
	}
	k := strings.ToLower(apiErr.Kind)
	return strings.Contains(k, "notfound") || strings.Contains(strings.ToLower(apiErr.Body), "could not locate record")
}

// NewClient authenticates against `service` using the identifier + app
// password and returns a ready-to-use client. The service URL is the
// entryway or PDS that hosts the account's session endpoint.
func NewClient(ctx context.Context, service, identifier, appPassword string) (*Client, error) {
	if service == "" {
		service = DefaultService
	}
	service = strings.TrimRight(service, "/")
	c := &Client{
		http:    &http.Client{Timeout: 30 * time.Second},
		service: service,
	}
	if err := c.createSession(ctx, identifier, appPassword); err != nil {
		return nil, err
	}
	return c, nil
}

// DID returns the authenticated repository's DID.
func (c *Client) DID() string { return c.did }

// createSession calls com.atproto.server.createSession and caches the JWT.
func (c *Client) createSession(ctx context.Context, identifier, appPassword string) error {
	var resp struct {
		AccessJwt string `json:"accessJwt"`
		DID       string `json:"did"`
		Handle    string `json:"handle"`
	}
	req := map[string]string{"identifier": identifier, "password": appPassword}
	if err := c.postJSON(ctx, "com.atproto.server.createSession", req, &resp); err != nil {
		return fmt.Errorf("createSession: %w", err)
	}
	c.accessJwt = resp.AccessJwt
	c.did = resp.DID
	c.handle = resp.Handle
	return nil
}

// GetRecord fetches a single record. Returns the raw value plus its CID.
// The returned value is the record body (i.e. the object under the `$type`
// key), decoded into the provided out pointer.
func (c *Client) GetRecord(ctx context.Context, repo, collection, rkey string, out any) (cid string, err error) {
	q := fmt.Sprintf("repo=%s&collection=%s&rkey=%s",
		urlEscape(repo), urlEscape(collection), urlEscape(rkey))
	var resp struct {
		URI   string          `json:"uri"`
		CID   string          `json:"cid"`
		Value json.RawMessage `json:"value"`
	}
	if err := c.getJSON(ctx, "com.atproto.repo.getRecord?"+q, &resp); err != nil {
		return "", err
	}
	if out != nil {
		if err := json.Unmarshal(resp.Value, out); err != nil {
			return "", fmt.Errorf("decode record value: %w", err)
		}
	}
	return resp.CID, nil
}

// PutRecord upserts a record at (repo, collection, rkey) with the given
// body. The body must include the `$type` field. Returns the new CID.
func (c *Client) PutRecord(ctx context.Context, repo, collection, rkey string, record any) (uri, cid string, err error) {
	req := map[string]any{
		"repo":       repo,
		"collection": collection,
		"rkey":       rkey,
		"record":     record,
	}
	var resp struct {
		URI string `json:"uri"`
		CID string `json:"cid"`
	}
	if err := c.postJSON(ctx, "com.atproto.repo.putRecord", req, &resp); err != nil {
		return "", "", err
	}
	return resp.URI, resp.CID, nil
}

// DeleteRecord removes a record from the repo.
func (c *Client) DeleteRecord(ctx context.Context, repo, collection, rkey string) error {
	req := map[string]any{
		"repo":       repo,
		"collection": collection,
		"rkey":       rkey,
	}
	return c.postJSON(ctx, "com.atproto.repo.deleteRecord", req, nil)
}

// ---------------------------------------------------------------------------
// low-level XRPC transport
// ---------------------------------------------------------------------------

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.service+"/xrpc/"+path, nil)
	if err != nil {
		return err
	}
	c.authorize(req)
	return c.doJSON(req, http.MethodGet+" "+path, out)
}

func (c *Client) postJSON(ctx context.Context, method string, body, out any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.service+"/xrpc/"+method, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.authorize(req)
	return c.doJSON(req, method, out)
}

func (c *Client) authorize(req *http.Request) {
	if c.accessJwt != "" {
		req.Header.Set("Authorization", "Bearer "+c.accessJwt)
	}
}

func (c *Client) doJSON(req *http.Request, label string, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("marque: %s: %w", label, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if out == nil || len(body) == 0 {
			return nil
		}
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("marque: %s: decode response: %w", label, err)
		}
		return nil
	}
	apiErr := &APIError{Status: resp.StatusCode, Method: label, Body: string(body)}
	var wire struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &wire); err == nil {
		apiErr.Kind = wire.Error
		if wire.Message != "" {
			apiErr.Body = wire.Message
		}
	}
	return apiErr
}

func urlEscape(s string) string {
	// Minimal escape: XRPC identifiers and rkeys are already restricted to
	// safe characters (letters, digits, `.`, `:`, `-`, `_`). Only encode `&`
	// to guard the query separator, and space for paranoia.
	s = strings.ReplaceAll(s, "&", "%26")
	s = strings.ReplaceAll(s, " ", "%20")
	return s
}
