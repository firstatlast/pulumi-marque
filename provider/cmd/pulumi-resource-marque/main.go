// Command pulumi-resource-marque is the Marque Pulumi provider plugin.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/firstatlast/pulumi-marque/provider/marque"
)

// Version is the provider version. Overridden at build time via
// -ldflags "-X main.Version=x.y.z".
var Version = "0.1.1"

func main() {
	provider, err := marque.NewProvider()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build provider: %s\n", err)
		os.Exit(1)
	}
	if err := provider.Run(context.Background(), marque.Name, Version); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}
