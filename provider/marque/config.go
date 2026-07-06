package marque

import (
	"context"
	"fmt"

	"github.com/pulumi/pulumi-go-provider/infer"
)

// DefaultService is the atproto entryway used when no explicit service is
// configured. bsky.social is the most common host for app-password auth;
// self-hosters can override.
const DefaultService = "https://bsky.social"

type Config struct {
	Service     string `pulumi:"service,optional"`
	Identifier  string `pulumi:"identifier,optional"`
	AppPassword string `pulumi:"appPassword,optional" provider:"secret"`

	client *Client
}

var (
	_ infer.Annotated       = (*Config)(nil)
	_ infer.CustomConfigure = (*Config)(nil)
)

func (c *Config) Annotate(a infer.Annotator) {
	a.Describe(&c.Service, "atproto service URL used for authentication (default https://bsky.social).")
	a.Describe(&c.Identifier, "atproto handle or DID whose PDS holds the DNS records.")
	a.Describe(&c.AppPassword, "atproto app password. Generate one in your account's app-passwords settings.")
	a.SetDefault(&c.Service, DefaultService, "MARQUE_SERVICE")
	a.SetDefault(&c.Identifier, "", "MARQUE_IDENTIFIER")
	a.SetDefault(&c.AppPassword, "", "MARQUE_APP_PASSWORD")
}

func (c *Config) Configure(ctx context.Context) error {
	if c.Identifier == "" {
		return fmt.Errorf("marque: identifier is required (set `marque:identifier` or MARQUE_IDENTIFIER)")
	}
	if c.AppPassword == "" {
		return fmt.Errorf("marque: appPassword is required (set `marque:appPassword` or MARQUE_APP_PASSWORD)")
	}
	client, err := NewClient(ctx, c.Service, c.Identifier, c.AppPassword)
	if err != nil {
		return err
	}
	c.client = client
	return nil
}

func clientFromContext(ctx context.Context) *Client {
	return infer.GetConfig[Config](ctx).client
}
