# pulumi-marque

A [Pulumi](https://www.pulumi.com) provider for [Marque](https://marque.at) — the atproto-native domain registrar and DNS host.

> Status: **early development** — API surface is still stabilizing.

## What it manages

Marque doesn't publish a bespoke DNS REST API. Instead, DNS zones are stored as atproto records in the account's own PDS under the `at.marque.dns` collection (schema: [at.marque.dns · Lexicon Garden](https://lexicon.garden/lexicon/did:plc:nckosudltxrtrjkt4zz4jy5y/at.marque.dns)). This provider is a thin declarative wrapper over the standard `com.atproto.repo` XRPC methods (`getRecord`, `putRecord`, `deleteRecord`).

### Resources

- `marque:index:DnsZone` — the full DNS record set for one domain. Marque stores the whole zone as one atproto record, so writes are atomic and the `records` input is authoritative (entries you omit are removed).
- `marque:index:Domain` — the `at.marque.domain` record for a domain already registered on the account. Adopts the existing registration and manages its mutable fields (nameservers, DNSSEC, auto-renew, WHOIS privacy, linked atproto handle). Registration and de-registration happen in Marque's UI, not through this resource — `create` refuses to invent a record and `delete` is a no-op that detaches management without touching the underlying record.

### Prerequisites

- The domain must already be registered at [marque.at](https://marque.at) — this provider does not register or transfer domains. It creates/updates the DNS zone that sits alongside an existing `at.marque.domain` record.
- An **app password** on the account whose PDS holds the domain record (bsky.social's app-password settings, or your own PDS's equivalent).

## Configuration

| Config | Env var | Secret | Description |
|--------|---------|--------|-------------|
| `marque:service` | `MARQUE_SERVICE` | no | atproto service URL (default `https://bsky.social`). Set to your PDS if the account is self-hosted. |
| `marque:identifier` | `MARQUE_IDENTIFIER` | no | atproto handle or DID whose PDS holds the DNS records. |
| `marque:appPassword` | `MARQUE_APP_PASSWORD` | yes | atproto app password. |

## Usage

```typescript
import * as marque from "@firstatlast/marque";

new marque.DnsZone("example-com", {
    domain: "example.com",
    records: [
        { name: "@",   type: "CNAME", value: "your-target.example.net", ttl: 300 },
        { name: "www", type: "CNAME", value: "your-target.example.net", ttl: 300 },
        { name: "@",   type: "MX",    value: "mail.example.net", ttl: 300, priority: 10 },
        { name: "_atproto", type: "TXT", value: "did=did:plc:...", ttl: 300 },
    ],
});

new marque.Domain("example-com-domain", {
    domain: "example.com",
    nameServers: ["ns1.marque.at", "ns2.marque.at"],
    dnssec: true,
    autoRenew: true,
    whoisPrivacy: "on",
});
```

## Repository layout

```
provider/
├── cmd/pulumi-resource-marque/   # plugin entrypoint
└── marque/                        # provider implementation
    ├── client.go                  # atproto XRPC client (createSession, get/put/delete record)
    ├── config.go                  # provider config
    ├── dnszone.go                 # DnsZone resource
    ├── domain.go                  # Domain resource
    └── provider.go                # provider registration
sdk/                                # generated SDKs (make sdk)
examples/                           # runnable examples
```

## Development

```bash
make build     # build the plugin binary
make schema    # regenerate provider/cmd/pulumi-resource-marque/schema.json
make sdk       # regenerate Go + Node.js SDKs
make install   # install the plugin locally for pulumi up testing
make test      # run unit tests
```

## Design notes

- **Whole-zone resource, not per-record.** The `at.marque.dns` record contains the entire zone in one array — reads/writes are atomic, so a single Pulumi resource matches the domain model cleanly.
- **rkey convention.** Zone records use the FQDN as the record key, mirroring the sibling `at.marque.domain` collection.
- **Subject strongRef.** Every zone references its parent `at.marque.domain` record. The provider auto-resolves this on Create/Update by fetching the domain record's current CID, so re-registrations are handled transparently.
- **Order-insensitive diff.** Reordering entries in code doesn't produce a diff; only true set changes do.
- **`Domain` is adopt-only.** `Create` requires the `at.marque.domain` record to already exist and refuses to invent one — registration goes through Marque's purchase flow. `Delete` is a no-op that detaches management without touching the underlying record, because that record encodes registry-level ownership rather than provider-created infrastructure.

## License

Apache-2.0. See [LICENSE](LICENSE).
