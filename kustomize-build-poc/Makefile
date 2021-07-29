
.PHONY: genprov gen-attestation

genprov:
	@echo building binary for cli
	go mod tidy
	CGO_ENABLED=0 GOARCH=amd64 GO111MODULE=on go build -ldflags="-s -w" -a -o genprov ./cmd/genprov

gen-attestation:
	@echo building binary for cli
	go mod tidy
	CGO_ENABLED=0 GOARCH=amd64 GO111MODULE=on go build -ldflags="-s -w" -a -o gen-attestation ./cmd/gen-attestation