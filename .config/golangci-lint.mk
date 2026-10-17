# The one golangci-lint pin. The Makefile includes this file; CI installs and
# runs the linter through the Makefile, so both read the version from here.
# The checksum is that of the release's linux-amd64 tarball.
GOLANGCI_LINT_VERSION := 2.14.0
GOLANGCI_LINT_SHA256 := ab90aeb7b066f92a33415b638a50fe5344bbb75a0d32ad30cc248d88f81032ab
