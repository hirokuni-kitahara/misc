package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/hirokuni-kitahara/misc/kustomize-build-poc/pkg/provenance"
)

func main() {
	if len(os.Args) <= 1 {
		fmt.Println("specify kustomize base path")
		return
	}
	kustomizeBaseDir := os.Args[1]
	artifactPath := os.Args[2]
	digest := ""
	var err error

	digest, err = provenance.GetDigestOfArtifact(artifactPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "err:", err.Error())
		return
	}
	startTime := time.Now().UTC()
	finishTime := time.Now().UTC()
	prov, err := provenance.GenerateProvenance(artifactPath, digest, kustomizeBaseDir, startTime, finishTime)
	if err != nil {
		fmt.Fprintln(os.Stderr, "err:", err.Error())
		return
	}
	provJson, _ := json.Marshal(prov)
	fmt.Println(string(provJson))
	return
}
