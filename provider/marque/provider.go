package marque

import (
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

// Name is the provider (package) name. It must match the plugin binary
// name pulumi-resource-<Name>.
const Name = "marque"

// NewProvider builds the Marque provider.
func NewProvider() (p.Provider, error) {
	return infer.NewProviderBuilder().
		WithNamespace("firstatlast").
		WithDisplayName("Marque").
		WithDescription("A Pulumi provider for Marque — atproto-native domain registrar and DNS host.").
		WithHomepage("https://marque.at").
		WithRepository("https://github.com/firstatlast/pulumi-marque").
		WithPublisher("firstatlast").
		WithPluginDownloadURL("github://api.github.com/firstatlast/pulumi-marque").
		WithGoImportPath("github.com/firstatlast/pulumi-marque/sdk/go/marque").
		WithLicense("Apache-2.0").
		WithKeywords("marque", "atproto", "dns", "domain", "category/network").
		WithConfig(infer.Config(&Config{})).
		WithResources(
			infer.Resource(&DnsZone{}),
		).
		Build()
}
