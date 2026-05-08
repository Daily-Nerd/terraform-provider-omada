BINARY_NAME = terraform-provider-omada
DEV_OVERRIDES_PATH = $(shell pwd)

.PHONY: build test testacc testacc-network testacc-data fmt fmtcheck vet dev clean

build:
	go build -o $(BINARY_NAME) .

# Unit tests only (no controller required).
test:
	go test -v -count=1 ./internal/client/ ./internal/resources/

# Acceptance tests against a real controller.
# Required env: OMADA_URL, OMADA_USERNAME, OMADA_PASSWORD, OMADA_TEST_SITE_ID
# Optional:     OMADA_SITE, OMADA_SKIP_TLS_VERIFY (default true)
#
# These tests CREATE and DESTROY real controller resources. NEVER run against
# a production controller — use a dedicated test site or the Docker dev image.
testacc:
	@if [ -z "$$OMADA_URL" ] || [ -z "$$OMADA_USERNAME" ] || [ -z "$$OMADA_PASSWORD" ] || [ -z "$$OMADA_TEST_SITE_ID" ]; then \
		echo "ERROR: missing required env vars."; \
		echo "  OMADA_URL=$$OMADA_URL"; \
		echo "  OMADA_USERNAME=$$OMADA_USERNAME"; \
		echo "  OMADA_PASSWORD=$$([ -n "$$OMADA_PASSWORD" ] && echo set || echo unset)"; \
		echo "  OMADA_TEST_SITE_ID=$$OMADA_TEST_SITE_ID"; \
		echo "See docs/CONTRIBUTING.md for setup."; \
		exit 1; \
	fi
	TF_ACC=1 go test -v -count=1 -timeout 60m ./internal/provider/...

# Run only network-related acceptance tests.
testacc-network:
	TF_ACC=1 go test -v -count=1 -timeout 30m -run 'TestAccResourceNetwork|TestAccDataSourceNetworks' ./internal/provider/...

# Run only data source acceptance tests (read-only, safest).
testacc-data:
	TF_ACC=1 go test -v -count=1 -timeout 30m -run 'TestAccDataSource' ./internal/provider/...

fmt:
	go fmt ./...

fmtcheck:
	@gofmt -l . | grep -v vendor | tee /dev/stderr | (! read)

vet:
	go vet ./...

dev: build
	@echo "Binary built at $(DEV_OVERRIDES_PATH)/$(BINARY_NAME)"
	@echo "Ensure your ~/.terraformrc has dev_overrides pointing to: $(DEV_OVERRIDES_PATH)"

clean:
	rm -f $(BINARY_NAME)
	rm -rf dist/
