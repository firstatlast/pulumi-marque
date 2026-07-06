// Minimal example: manage the DNS zone for an existing Marque-registered domain.
//
// Prerequisites:
//   1. The domain is already registered at marque.at (this provider does
//      not register domains).
//   2. An app password exists on the account, set via one of:
//        pulumi config set --secret marque:appPassword ...
//        export MARQUE_APP_PASSWORD=...
//   3. The identifier (handle or DID) is set:
//        pulumi config set marque:identifier you.bsky.social
//        export MARQUE_IDENTIFIER=you.bsky.social

import * as marque from "@firstatlast/marque";

new marque.DnsZone("example-com", {
    domain: "example.com",
    records: [
        { name: "@",   type: "CNAME", value: "your-target.example.net", ttl: 300 },
        { name: "www", type: "CNAME", value: "your-target.example.net", ttl: 300 },
        { name: "@",   type: "MX",    value: "mail.example.net", ttl: 300, priority: 10 },
    ],
});
