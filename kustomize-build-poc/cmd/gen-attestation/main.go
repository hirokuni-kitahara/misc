package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/hirokuni-kitahara/misc/kustomize-build-poc/pkg/provenance"
)

func main() {
	if len(os.Args) <= 2 {
		fmt.Println("specify provenance.json path and signing key path")
		return
	}
	provPath := os.Args[1]
	privKeyPath := os.Args[2]

	attestation, err := provenance.GenerateAttestation(provPath, privKeyPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "err:", err.Error())
		return
	}
	attestationJson, _ := json.Marshal(attestation)
	fmt.Println(string(attestationJson))
	return
}
