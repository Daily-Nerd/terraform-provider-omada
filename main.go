package main

import (
	"context"
	"log"

	"github.com/Daily-Nerd/terraform-provider-omada/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

// version is set by GoReleaser at build time via ldflags.
var version = "dev"

func main() {
	err := providerserver.Serve(context.Background(), provider.New, providerserver.ServeOpts{
		Address: "registry.terraform.io/daily-nerd/omada",
	})
	if err != nil {
		log.Fatal(err)
	}
}
