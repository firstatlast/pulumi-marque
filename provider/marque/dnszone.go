package marque

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type DnsZone struct{}

// Record is one DNS entry inside a zone. Only pulumi tags here — JSON
// wire format for atproto uses different field names (`recordType`
// instead of `type`), handled explicitly by the MarshalJSON /
// UnmarshalJSON methods below. Mixing pulumi + json tags on the same
// struct confuses pulumi-go-provider's input decoder, which resolves
// input properties via json tags when present.
type Record struct {
	Name     string `pulumi:"name"`
	Type     string `pulumi:"type"`
	Value    string `pulumi:"value"`
	TTL      int    `pulumi:"ttl"`
	Priority *int   `pulumi:"priority,optional"`
}

// wireRecord mirrors the at.marque.dns#entry lexicon exactly.
type wireRecord struct {
	Name       string `json:"name"`
	RecordType string `json:"recordType"`
	Value      string `json:"value"`
	TTL        int    `json:"ttl"`
	Priority   *int   `json:"priority,omitempty"`
}

func (r Record) MarshalJSON() ([]byte, error) {
	return json.Marshal(wireRecord{
		Name: r.Name, RecordType: r.Type, Value: r.Value,
		TTL: r.TTL, Priority: r.Priority,
	})
}

func (r *Record) UnmarshalJSON(data []byte) error {
	var w wireRecord
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*r = Record{
		Name: w.Name, Type: w.RecordType, Value: w.Value,
		TTL: w.TTL, Priority: w.Priority,
	}
	return nil
}

type DnsZoneArgs struct {
	Domain  string   `pulumi:"domain"`
	Records []Record `pulumi:"records"`
}

type DnsZoneState struct {
	DnsZoneArgs
	// Repository DID whose PDS holds the record (i.e. the authenticated
	// identifier resolved to a DID).
	Repo string `pulumi:"repo"`
	// AT-URI of the at.marque.dns record (at://<did>/at.marque.dns/<rkey>).
	Uri string `pulumi:"uri"`
	// Current CID of the record.
	Cid string `pulumi:"cid"`
	// AT-URI of the at.marque.domain record used as the zone's subject.
	SubjectUri string `pulumi:"subjectUri"`
	SubjectCid string `pulumi:"subjectCid"`
	// Server-recorded creation timestamp.
	CreatedAt string `pulumi:"createdAt"`
}

var (
	_ infer.CustomResource[DnsZoneArgs, DnsZoneState] = (*DnsZone)(nil)
	_ infer.CustomRead[DnsZoneArgs, DnsZoneState]     = (*DnsZone)(nil)
	_ infer.CustomUpdate[DnsZoneArgs, DnsZoneState]   = (*DnsZone)(nil)
	_ infer.CustomDelete[DnsZoneState]                = (*DnsZone)(nil)
	_ infer.CustomDiff[DnsZoneArgs, DnsZoneState]     = (*DnsZone)(nil)
	_ infer.Annotated                                 = (*DnsZone)(nil)
)

func (z *DnsZone) Annotate(a infer.Annotator) {
	a.Describe(&z, "A Marque DNS zone (at.marque.dns record) — the full DNS record set for a domain.")
	a.SetToken("index", "DnsZone")
}

func (a *DnsZoneArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.Domain, "Fully qualified domain name this zone belongs to. Must be a domain already registered in the account's at.marque.domain records. Changing this replaces the zone.")
	an.Describe(&a.Records, "Full set of DNS records for the zone. Marque stores the whole zone as one atproto record, so writes are atomic and this list is authoritative — omitted entries are removed.")
}

func (r *Record) Annotate(an infer.Annotator) {
	an.Describe(&r.Name, "Record name relative to the zone apex. Use '@' for the apex itself.")
	an.Describe(&r.Type, "DNS record type. One of A, AAAA, CNAME, MX, TXT, SRV, NS, CAA.")
	an.Describe(&r.Value, "Record value / rdata.")
	an.Describe(&r.TTL, "Time-to-live in seconds.")
	an.Describe(&r.Priority, "Priority for MX and SRV records.")
}

// zoneRkey is the record key convention for at.marque.dns: the FQDN, matching
// the sibling at.marque.domain collection.
func zoneRkey(domain string) string { return domain }

// zoneRecordBody assembles the JSON body for a putRecord call, including
// the $type discriminator, the strongRef subject, and the entries.
func zoneRecordBody(domain string, subject strongRef, records []Record, createdAt string) map[string]any {
	return map[string]any{
		"$type":     CollectionDns,
		"domain":    domain,
		"subject":   subject,
		"records":   records,
		"createdAt": createdAt,
	}
}

type strongRef struct {
	URI string `json:"uri"`
	CID string `json:"cid"`
}

// lookupDomainSubject fetches the at.marque.domain record for `domain` and
// returns a strongRef suitable for the zone's subject field. Errors if the
// domain is not registered on the authenticated account.
func lookupDomainSubject(ctx context.Context, client *Client, domain string) (strongRef, error) {
	cid, err := client.GetRecord(ctx, client.DID(), CollectionDomain, domain, nil)
	if err != nil {
		if IsNotFound(err) {
			return strongRef{}, fmt.Errorf("marque: no at.marque.domain record for %q on repo %s — register the domain at marque.at first", domain, client.DID())
		}
		return strongRef{}, err
	}
	uri := fmt.Sprintf("at://%s/%s/%s", client.DID(), CollectionDomain, domain)
	return strongRef{URI: uri, CID: cid}, nil
}

func (DnsZone) Create(ctx context.Context, req infer.CreateRequest[DnsZoneArgs]) (infer.CreateResponse[DnsZoneState], error) {
	in := req.Inputs
	id := in.Domain

	if req.DryRun {
		return infer.CreateResponse[DnsZoneState]{
			ID:     id,
			Output: DnsZoneState{DnsZoneArgs: in},
		}, nil
	}

	client := clientFromContext(ctx)
	subject, err := lookupDomainSubject(ctx, client, in.Domain)
	if err != nil {
		return infer.CreateResponse[DnsZoneState]{}, err
	}

	createdAt := nowRFC3339()
	body := zoneRecordBody(in.Domain, subject, in.Records, createdAt)
	uri, cid, err := client.PutRecord(ctx, client.DID(), CollectionDns, zoneRkey(in.Domain), body)
	if err != nil {
		return infer.CreateResponse[DnsZoneState]{}, fmt.Errorf("putRecord %s: %w", CollectionDns, err)
	}
	return infer.CreateResponse[DnsZoneState]{
		ID: id,
		Output: DnsZoneState{
			DnsZoneArgs: in,
			Repo:        client.DID(),
			Uri:         uri,
			Cid:         cid,
			SubjectUri:  subject.URI,
			SubjectCid:  subject.CID,
			CreatedAt:   createdAt,
		},
	}, nil
}

func (DnsZone) Read(ctx context.Context, req infer.ReadRequest[DnsZoneArgs, DnsZoneState]) (infer.ReadResponse[DnsZoneArgs, DnsZoneState], error) {
	domain := req.State.Domain
	if domain == "" {
		domain = req.ID
	}
	client := clientFromContext(ctx)
	var raw struct {
		Domain    string    `json:"domain"`
		Subject   strongRef `json:"subject"`
		Records   []Record  `json:"records"`
		CreatedAt string    `json:"createdAt"`
	}
	cid, err := client.GetRecord(ctx, client.DID(), CollectionDns, zoneRkey(domain), &raw)
	if err != nil {
		if IsNotFound(err) {
			return infer.ReadResponse[DnsZoneArgs, DnsZoneState]{}, nil
		}
		return infer.ReadResponse[DnsZoneArgs, DnsZoneState]{}, err
	}
	state := DnsZoneState{
		DnsZoneArgs: DnsZoneArgs{Domain: raw.Domain, Records: raw.Records},
		Repo:        client.DID(),
		Uri:         fmt.Sprintf("at://%s/%s/%s", client.DID(), CollectionDns, zoneRkey(domain)),
		Cid:         cid,
		SubjectUri:  raw.Subject.URI,
		SubjectCid:  raw.Subject.CID,
		CreatedAt:   raw.CreatedAt,
	}
	return infer.ReadResponse[DnsZoneArgs, DnsZoneState]{
		ID:     domain,
		Inputs: state.DnsZoneArgs,
		State:  state,
	}, nil
}

func (DnsZone) Update(ctx context.Context, req infer.UpdateRequest[DnsZoneArgs, DnsZoneState]) (infer.UpdateResponse[DnsZoneState], error) {
	in := req.Inputs
	if req.DryRun {
		return infer.UpdateResponse[DnsZoneState]{
			Output: DnsZoneState{
				DnsZoneArgs: in,
				Repo:        req.State.Repo,
				Uri:         req.State.Uri,
				Cid:         req.State.Cid,
				SubjectUri:  req.State.SubjectUri,
				SubjectCid:  req.State.SubjectCid,
				CreatedAt:   req.State.CreatedAt,
			},
		}, nil
	}

	client := clientFromContext(ctx)
	// Re-resolve the subject in case the underlying domain record was
	// re-created (which would change its CID) since we last synced.
	subject, err := lookupDomainSubject(ctx, client, in.Domain)
	if err != nil {
		return infer.UpdateResponse[DnsZoneState]{}, err
	}
	createdAt := nowRFC3339()
	body := zoneRecordBody(in.Domain, subject, in.Records, createdAt)
	uri, cid, err := client.PutRecord(ctx, client.DID(), CollectionDns, zoneRkey(in.Domain), body)
	if err != nil {
		return infer.UpdateResponse[DnsZoneState]{}, fmt.Errorf("putRecord %s: %w", CollectionDns, err)
	}
	return infer.UpdateResponse[DnsZoneState]{
		Output: DnsZoneState{
			DnsZoneArgs: in,
			Repo:        client.DID(),
			Uri:         uri,
			Cid:         cid,
			SubjectUri:  subject.URI,
			SubjectCid:  subject.CID,
			CreatedAt:   createdAt,
		},
	}, nil
}

func (DnsZone) Delete(ctx context.Context, req infer.DeleteRequest[DnsZoneState]) (infer.DeleteResponse, error) {
	client := clientFromContext(ctx)
	if err := client.DeleteRecord(ctx, req.State.Repo, CollectionDns, zoneRkey(req.State.Domain)); err != nil {
		if IsNotFound(err) {
			return infer.DeleteResponse{}, nil
		}
		return infer.DeleteResponse{}, err
	}
	return infer.DeleteResponse{}, nil
}

// Diff: domain change is a replace (rkey changes); records change is an
// in-place update via putRecord.
func (DnsZone) Diff(_ context.Context, req infer.DiffRequest[DnsZoneArgs, DnsZoneState]) (infer.DiffResponse, error) {
	in := req.Inputs
	old := req.State.DnsZoneArgs
	diff := map[string]p.PropertyDiff{}
	if in.Domain != old.Domain {
		diff["domain"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	if !recordsEqual(in.Records, old.Records) {
		diff["records"] = p.PropertyDiff{Kind: p.Update}
	}
	return infer.DiffResponse{
		HasChanges:   len(diff) > 0,
		DetailedDiff: diff,
	}, nil
}

// recordsEqual compares two record lists as sets on (name, type, value, ttl,
// priority). Order is treated as insignificant so re-arranging entries in
// code doesn't produce noisy diffs.
func recordsEqual(a, b []Record) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make([]bool, len(b))
	for _, ra := range a {
		matched := false
		for i, rb := range b {
			if seen[i] {
				continue
			}
			if reflect.DeepEqual(ra, rb) {
				seen[i] = true
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339) }
