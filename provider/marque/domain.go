package marque

import (
	"context"
	"fmt"
	"reflect"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type Domain struct{}

type DomainArgs struct {
	Domain          string   `pulumi:"domain"`
	NameServers     []string `pulumi:"nameServers,optional"`
	Dnssec          *bool    `pulumi:"dnssec,optional"`
	AutoRenew       *bool    `pulumi:"autoRenew,optional"`
	WhoisPrivacy    *string  `pulumi:"whoisPrivacy,optional"`
	AtprotoHandle   *string  `pulumi:"atprotoHandle,optional"`
}

type DomainState struct {
	DomainArgs
	Repo            string `pulumi:"repo"`
	Uri             string `pulumi:"uri"`
	Cid             string `pulumi:"cid"`
	Status          string `pulumi:"status"`
	RegisteredAt    string `pulumi:"registeredAt"`
	ExpiresAt       string `pulumi:"expiresAt"`
	AtprotoVerified *bool  `pulumi:"atprotoVerified,optional"`
}

// domainWire mirrors the at.marque.domain lexicon on the wire. Server-managed
// fields (status, registeredAt, expiresAt, createdAt, atprotoVerified) are
// preserved verbatim across merges; mutable fields are overwritten from
// user inputs when present.
type domainWire struct {
	Type            string   `json:"$type,omitempty"`
	Domain          string   `json:"domain"`
	Status          string   `json:"status,omitempty"`
	RegisteredAt    string   `json:"registeredAt,omitempty"`
	ExpiresAt       string   `json:"expiresAt,omitempty"`
	CreatedAt       string   `json:"createdAt,omitempty"`
	NameServers     []string `json:"nameServers,omitempty"`
	Dnssec          *bool    `json:"dnssec,omitempty"`
	AutoRenew       *bool    `json:"autoRenew,omitempty"`
	WhoisPrivacy    *string  `json:"whoisPrivacy,omitempty"`
	AtprotoHandle   *string  `json:"atprotoHandle,omitempty"`
	AtprotoVerified *bool    `json:"atprotoVerified,omitempty"`
}

var (
	_ infer.CustomResource[DomainArgs, DomainState] = (*Domain)(nil)
	_ infer.CustomRead[DomainArgs, DomainState]     = (*Domain)(nil)
	_ infer.CustomUpdate[DomainArgs, DomainState]   = (*Domain)(nil)
	_ infer.CustomDelete[DomainState]               = (*Domain)(nil)
	_ infer.CustomDiff[DomainArgs, DomainState]     = (*Domain)(nil)
	_ infer.Annotated                               = (*Domain)(nil)
)

func (d *Domain) Annotate(a infer.Annotator) {
	a.Describe(&d, "A Marque domain (at.marque.domain record). Adopts an existing registration and manages its mutable fields — nameservers, DNSSEC, auto-renew, WHOIS privacy, and the linked atproto handle. Does not register or delete domains.")
	a.SetToken("index", "Domain")
}

func (a *DomainArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.Domain, "Fully qualified domain name. Must already be registered on the authenticated account at marque.at — this resource adopts the existing record rather than creating one. Changing this replaces the resource.")
	an.Describe(&a.NameServers, "Authoritative nameservers for the domain (up to 8).")
	an.Describe(&a.Dnssec, "Whether DNSSEC is enabled for the domain.")
	an.Describe(&a.AutoRenew, "Whether the domain auto-renews before expiry.")
	an.Describe(&a.WhoisPrivacy, "WHOIS privacy setting. One of: on, off, gdpr, registry.")
	an.Describe(&a.AtprotoHandle, "atproto handle to link to this domain.")
}

func domainRkey(domain string) string { return domain }

// fetchDomainWire loads the current at.marque.domain record body. Returns
// IsNotFound-compatible errors when the record is absent.
func fetchDomainWire(ctx context.Context, client *Client, domain string) (domainWire, string, error) {
	var w domainWire
	cid, err := client.GetRecord(ctx, client.DID(), CollectionDomain, domainRkey(domain), &w)
	if err != nil {
		return domainWire{}, "", err
	}
	return w, cid, nil
}

// mergeInputs overlays user-supplied mutable fields onto an existing record
// body, leaving server-managed fields untouched.
func mergeInputs(existing domainWire, in DomainArgs) domainWire {
	out := existing
	out.Type = CollectionDomain
	out.Domain = in.Domain
	if in.NameServers != nil {
		out.NameServers = in.NameServers
	}
	if in.Dnssec != nil {
		out.Dnssec = in.Dnssec
	}
	if in.AutoRenew != nil {
		out.AutoRenew = in.AutoRenew
	}
	if in.WhoisPrivacy != nil {
		out.WhoisPrivacy = in.WhoisPrivacy
	}
	if in.AtprotoHandle != nil {
		out.AtprotoHandle = in.AtprotoHandle
	}
	return out
}

func stateFromWire(in DomainArgs, w domainWire, repo, cid string) DomainState {
	uri := fmt.Sprintf("at://%s/%s/%s", repo, CollectionDomain, domainRkey(in.Domain))
	return DomainState{
		DomainArgs:      in,
		Repo:            repo,
		Uri:             uri,
		Cid:             cid,
		Status:          w.Status,
		RegisteredAt:    w.RegisteredAt,
		ExpiresAt:       w.ExpiresAt,
		AtprotoVerified: w.AtprotoVerified,
	}
}

func (Domain) Create(ctx context.Context, req infer.CreateRequest[DomainArgs]) (infer.CreateResponse[DomainState], error) {
	in := req.Inputs
	id := in.Domain

	if req.DryRun {
		return infer.CreateResponse[DomainState]{
			ID:     id,
			Output: DomainState{DomainArgs: in},
		}, nil
	}

	client := clientFromContext(ctx)
	existing, _, err := fetchDomainWire(ctx, client, in.Domain)
	if err != nil {
		if IsNotFound(err) {
			return infer.CreateResponse[DomainState]{}, fmt.Errorf("marque: no at.marque.domain record for %q on repo %s — register the domain at marque.at first", in.Domain, client.DID())
		}
		return infer.CreateResponse[DomainState]{}, err
	}

	merged := mergeInputs(existing, in)
	uri, cid, err := client.PutRecord(ctx, client.DID(), CollectionDomain, domainRkey(in.Domain), merged)
	if err != nil {
		return infer.CreateResponse[DomainState]{}, fmt.Errorf("putRecord %s: %w", CollectionDomain, err)
	}
	state := stateFromWire(in, merged, client.DID(), cid)
	state.Uri = uri
	return infer.CreateResponse[DomainState]{ID: id, Output: state}, nil
}

func (Domain) Read(ctx context.Context, req infer.ReadRequest[DomainArgs, DomainState]) (infer.ReadResponse[DomainArgs, DomainState], error) {
	domain := req.State.Domain
	if domain == "" {
		domain = req.ID
	}
	client := clientFromContext(ctx)
	w, cid, err := fetchDomainWire(ctx, client, domain)
	if err != nil {
		if IsNotFound(err) {
			return infer.ReadResponse[DomainArgs, DomainState]{}, nil
		}
		return infer.ReadResponse[DomainArgs, DomainState]{}, err
	}
	inputs := DomainArgs{
		Domain:          w.Domain,
		NameServers:     w.NameServers,
		Dnssec:          w.Dnssec,
		AutoRenew:       w.AutoRenew,
		WhoisPrivacy:    w.WhoisPrivacy,
		AtprotoHandle:   w.AtprotoHandle,
	}
	state := stateFromWire(inputs, w, client.DID(), cid)
	return infer.ReadResponse[DomainArgs, DomainState]{
		ID:     domain,
		Inputs: inputs,
		State:  state,
	}, nil
}

func (Domain) Update(ctx context.Context, req infer.UpdateRequest[DomainArgs, DomainState]) (infer.UpdateResponse[DomainState], error) {
	in := req.Inputs
	if req.DryRun {
		return infer.UpdateResponse[DomainState]{
			Output: DomainState{
				DomainArgs:      in,
				Repo:            req.State.Repo,
				Uri:             req.State.Uri,
				Cid:             req.State.Cid,
				Status:          req.State.Status,
				RegisteredAt:    req.State.RegisteredAt,
				ExpiresAt:       req.State.ExpiresAt,
				AtprotoVerified: req.State.AtprotoVerified,
			},
		}, nil
	}

	client := clientFromContext(ctx)
	existing, _, err := fetchDomainWire(ctx, client, in.Domain)
	if err != nil {
		if IsNotFound(err) {
			return infer.UpdateResponse[DomainState]{}, fmt.Errorf("marque: at.marque.domain record for %q disappeared from repo %s", in.Domain, client.DID())
		}
		return infer.UpdateResponse[DomainState]{}, err
	}

	merged := mergeInputs(existing, in)
	uri, cid, err := client.PutRecord(ctx, client.DID(), CollectionDomain, domainRkey(in.Domain), merged)
	if err != nil {
		return infer.UpdateResponse[DomainState]{}, fmt.Errorf("putRecord %s: %w", CollectionDomain, err)
	}
	state := stateFromWire(in, merged, client.DID(), cid)
	state.Uri = uri
	return infer.UpdateResponse[DomainState]{Output: state}, nil
}

// Delete is intentionally a no-op. An at.marque.domain record represents a
// registry-level fact (that this account owns the domain) rather than
// provider-created infrastructure. Deleting the record would either be
// rejected server-side or, worse, orphan the domain in Marque's UI. Removing
// this resource from Pulumi state should detach management without touching
// the underlying record.
func (Domain) Delete(_ context.Context, _ infer.DeleteRequest[DomainState]) (infer.DeleteResponse, error) {
	return infer.DeleteResponse{}, nil
}

func (Domain) Diff(_ context.Context, req infer.DiffRequest[DomainArgs, DomainState]) (infer.DiffResponse, error) {
	in := req.Inputs
	old := req.State.DomainArgs
	diff := map[string]p.PropertyDiff{}
	if in.Domain != old.Domain {
		diff["domain"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	if !stringSlicesEqual(in.NameServers, old.NameServers) {
		diff["nameServers"] = p.PropertyDiff{Kind: p.Update}
	}
	if !boolPtrEqual(in.Dnssec, old.Dnssec) {
		diff["dnssec"] = p.PropertyDiff{Kind: p.Update}
	}
	if !boolPtrEqual(in.AutoRenew, old.AutoRenew) {
		diff["autoRenew"] = p.PropertyDiff{Kind: p.Update}
	}
	if !stringPtrEqual(in.WhoisPrivacy, old.WhoisPrivacy) {
		diff["whoisPrivacy"] = p.PropertyDiff{Kind: p.Update}
	}
	if !stringPtrEqual(in.AtprotoHandle, old.AtprotoHandle) {
		diff["atprotoHandle"] = p.PropertyDiff{Kind: p.Update}
	}
	return infer.DiffResponse{
		HasChanges:   len(diff) > 0,
		DetailedDiff: diff,
	}, nil
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}

func boolPtrEqual(a, b *bool) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func stringPtrEqual(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
