package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/hirokuni-kitahara/misc/kustomize-build-poc/pkg/provenance"
)

func main() {
	if len(os.Args) <= 2 {
		fmt.Fprintln(os.Stderr, "specify provenance.json path and signing key path")
		return
	}
	provPath := os.Args[1]
	privKeyPath := os.Args[2]

	overwriteImageArtifact := ""
	if len(os.Args) >= 4 {
		overwriteImageArtifact = os.Args[3]
	}

	if overwriteImageArtifact != "" {
		newProvPath, err := provenance.OverwriteArtifactInProvenance(provPath, overwriteImageArtifact)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error from provenance.OverwriteArtifactInProvenance():", err.Error())
			return
		}
		provPath = newProvPath
	}

	attestation, err := provenance.GenerateAttestation(provPath, privKeyPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error from provenance.GenerateAttestation():", err.Error())
		return
	}
	attestationJson, _ := json.Marshal(attestation)
	fmt.Println(string(attestationJson))
	return
}
