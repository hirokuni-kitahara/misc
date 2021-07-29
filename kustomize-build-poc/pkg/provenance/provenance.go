package provenance

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hirokuni-kitahara/misc/kustomize-build-poc/pkg/kustomizeutil"
	intoto "github.com/in-toto/in-toto-golang/in_toto"
	"github.com/in-toto/in-toto-golang/pkg/ssl"
	"github.com/theupdateframework/go-tuf/encrypted"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

const cosignPwdEnvKey = "COSIGN_PASSWORD"

func GenerateProvenance(imageName, digest, kustomizeBase string, startTime, finishTime time.Time) (*intoto.Statement, error) {

	subjects := []intoto.Subject{}
	subjects = append(subjects, intoto.Subject{
		Name: imageName,
		Digest: intoto.DigestSet{
			"sha256": digest,
		},
	})

	materials, err := generateMaterialsFromKustomization(kustomizeBase)
	if err != nil {
		return nil, err
	}

	// TODO: set correct recipe
	entryPoint := ""
	recipe := intoto.ProvenanceRecipe{
		EntryPoint: entryPoint,
		Arguments:  []string{},
	}
	it := &intoto.Statement{
		StatementHeader: intoto.StatementHeader{
			Type:          intoto.StatementInTotoV01,
			PredicateType: intoto.PredicateProvenanceV01,
			Subject:       subjects,
		},
		Predicate: intoto.ProvenancePredicate{
			Metadata: intoto.ProvenanceMetadata{
				Reproducible:    true,
				BuildStartedOn:  &startTime,
				BuildFinishedOn: &finishTime,
			},

			Materials: materials,
			Recipe:    recipe,
		},
	}
	return it, nil
}

func GenerateAttestation(provPath, privKeyPath string) (*ssl.Envelope, error) {
	b, err := ioutil.ReadFile(provPath)
	if err != nil {
		return nil, err
	}
	ecdsaPriv, _ := ioutil.ReadFile(filepath.Clean(privKeyPath))
	pb, _ := pem.Decode(ecdsaPriv)
	pwd := os.Getenv(cosignPwdEnvKey) //GetPass(true)
	x509Encoded, err := encrypted.Decrypt(pb.Bytes, []byte(pwd))
	if err != nil {
		return nil, err
	}
	priv, err := x509.ParsePKCS8PrivateKey(x509Encoded)
	if err != nil {
		return nil, err
	}

	signer, err := ssl.NewEnvelopeSigner(&IntotoSigner{
		priv: priv.(*ecdsa.PrivateKey),
	})
	if err != nil {
		return nil, err
	}

	envelope, err := signer.SignPayload("application/vnd.in-toto+json", b)
	if err != nil {
		return nil, err
	}

	// Now verify
	err = signer.Verify(envelope)
	if err != nil {
		return nil, err
	}
	return envelope, nil
}

func generateMaterialsFromKustomization(kustomizeBase string) ([]intoto.ProvenanceMaterial, error) {
	materials := []intoto.ProvenanceMaterial{}
	resources, err := kustomizeutil.LoadKustomization(kustomizeBase, "", false)
	if err != nil {
		return nil, err
	}
	for _, r := range resources {
		m := resourceToMaterial(r)
		if m == nil {
			continue
		}
		materials = append(materials, *m)
	}
	return materials, nil
}

func resourceToMaterial(kr *kustomizeutil.KustomizationResource) *intoto.ProvenanceMaterial {
	if kr.File == nil && kr.GitRepo == nil {
		return nil
	} else if kr.File != nil {
		m := &intoto.ProvenanceMaterial{
			URI: kr.File.Name,
			Digest: intoto.DigestSet{
				"hash": kr.File.Hash,
			},
		}
		return m
	} else if kr.GitRepo != nil {
		m := &intoto.ProvenanceMaterial{
			URI: kr.GitRepo.URL,
			Digest: intoto.DigestSet{
				"commit":   kr.GitRepo.CommitID,
				"revision": kr.GitRepo.Revision,
				"path":     kr.GitRepo.Path,
			},
		}
		return m
	}
	return nil
}

func GetImageDigest(imageRef string) (string, error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return "", err
	}
	img, err := remote.Image(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return "", err
	}
	hash, err := img.Digest()
	if err != nil {
		return "", err
	}
	hashValue := strings.TrimPrefix(hash.String(), "sha256:")
	return hashValue, nil
}

type IntotoSigner struct {
	priv *ecdsa.PrivateKey
}

func (it *IntotoSigner) Sign(data []byte) ([]byte, string, error) {
	h := sha256.Sum256(data)
	sig, err := it.priv.Sign(rand.Reader, h[:], crypto.SHA256)
	if err != nil {
		return nil, "", err
	}
	return sig, "", nil
}

func (it *IntotoSigner) Verify(_ string, data, sig []byte) error {
	h := sha256.Sum256(data)
	ok := ecdsa.VerifyASN1(&it.priv.PublicKey, h[:], sig)
	if ok {
		return nil
	}
	return errors.New("invalid signature")
}
