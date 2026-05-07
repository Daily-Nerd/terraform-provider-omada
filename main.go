package main

import (
	"context"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/Daily-Nerd/terraform-provider-omada/internal/provider"
)

// version is set by GoReleaser at build time via ldflags.
var version = "dev"

func main() {
	err := providerserver.Serve(context.Background(), provider.New, providerserver.ServeOpts{
		Address: "registry.terraform.io/dailynerd/omada",
	})
	if err != nil {
		log.Fatal(err)
	}
}
