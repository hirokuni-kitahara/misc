// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package kustomizeutil

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	"sigs.k8s.io/kustomize/api/filesys"
	"sigs.k8s.io/kustomize/api/types"
)

const kustomizationFileName = "kustomization.yaml"
const gitCmd = "git"

type KustomizationResource struct {
	GitRepo *GitRepoResult
	File    *FileInfo
}

func LoadKustomization(fpath, baseDir string, inRemoteRepo bool) ([]*KustomizationResource, error) {
	isDir, err := IsDir(fpath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to judge if %s is directory or not", fpath)
	}
	if isDir {
		fpath = filepath.Join(fpath, kustomizationFileName)
	}
	if !FileExists(fpath) {
		return nil, fmt.Errorf("%s does not exists", fpath)
	}
	data, err := ioutil.ReadFile(fpath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read %s", fpath)
	}
	var k *types.Kustomization
	err = yaml.Unmarshal(data, &k)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal a content of %s into %T", fpath, k)
	}
	k.FixKustomizationPostUnmarshalling()

	// these resoruces are used as "provenance materials" later
	// files in a local filesystem --> File resource
	// all resources in a remote git repository --> GitRepo resource
	resources := []*KustomizationResource{}
	if !inRemoteRepo {
		kustFileHash, err := Sha256Hash(fpath)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get sha256 hash of %s", fpath)
		}
		kustRes := &KustomizationResource{File: &FileInfo{Name: fpath, Hash: kustFileHash}}
		resources = append(resources, kustRes)
	}
	for _, res := range k.Resources {
		if IsRepositoryResource(res) {
			repo, err := prepareBaseDirForRemoteRepository(res)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to create a directory for a git repository resource %s", res)
			}
			kustPath := filepath.Join(repo.RootDir, repo.Path, kustomizationFileName)
			baseDir := filepath.Join(repo.RootDir, repo.Path)
			rr := &KustomizationResource{GitRepo: repo}
			resources = append(resources, rr)
			remoteResources, err := LoadKustomization(kustPath, baseDir, true)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to load resources in a git repository %s", res)
			}
			resources = append(resources, remoteResources...)
		} else {
			rPath := filepath.Clean(filepath.Join(baseDir, res))
			rIsFile, err := IsFile(rPath)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to judge if %s is a file or not", res)
			}
			if rIsFile {
				if inRemoteRepo {
					// ignore local file resources inside remote repository
					// because they can be identified only with repo information
					continue
				}
				// files in a local filesystem should be included in resources as File resource
				rHash, err := Sha256Hash(rPath)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to???get sha256 hash of %s", rPath)
				}
				fr := &KustomizationResource{File: &FileInfo{Name: rPath, Hash: rHash}}
				resources = append(resources, fr)
			} else {
				// otherwise, this resource points a directory that contains a sub kustomization.yaml
				// so load it and add resources
				kustFile := filepath.Clean(filepath.Join(rPath, kustomizationFileName))

				if !inRemoteRepo {
					// if this is not in a remote repository, the kustomization.yaml will be added to File resources
					kustHash, err := Sha256Hash(kustFile)
					if err != nil {
						return nil, errors.Wrapf(err, "failed to???get sha256 hash of %s", kustFile)
					}
					fr := &KustomizationResource{File: &FileInfo{Name: kustFile, Hash: kustHash}}
					resources = append(resources, fr)
				}
				// load a sub kustomization.yaml
				kustFileDir := filepath.Dir(kustFile)
				subResources, err := LoadKustomization(kustFile, kustFileDir, inRemoteRepo)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to load a kustomization file %s", kustFile)
				}
				resources = append(resources, subResources...)
			}
		}
	}

	return resources, nil
}

func Sha256Hash(fpath string) (string, error) {
	f, err := os.Open(fpath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	hash := fmt.Sprintf("%x", h.Sum(nil))
	return hash, nil
}

type GitRepoResult struct {
	RootDir  string
	URL      string
	Revision string
	CommitID string
	Path     string
}

type FileInfo struct {
	Name string
	Hash string
}

func prepareBaseDirForRemoteRepository(url string) (*GitRepoResult, error) {
	r := &GitRepoResult{}
	r.URL, r.Revision, r.Path = parseGitURLinKustomization(url)
	cDir, err := filesys.NewTmpConfirmedDir()
	if err != nil {
		return nil, err
	}
	r.RootDir = cDir.String()

	_, err = CmdExec(gitCmd, r.RootDir, "init")
	if err != nil {
		return nil, err
	}
	_, err = CmdExec(gitCmd, r.RootDir, "remote", "add", "origin", r.URL)
	if err != nil {
		return nil, err
	}
	rev := "HEAD"
	if r.Revision != "" {
		rev = r.Revision
	}
	_, err = CmdExec(gitCmd, r.RootDir, "fetch", "--depth=1", "origin", rev)
	if err != nil {
		return nil, err
	}
	_, err = CmdExec(gitCmd, r.RootDir, "checkout", "FETCH_HEAD")
	if err != nil {
		return nil, err
	}
	commitGetOut, err := CmdExec(gitCmd, r.RootDir, "rev-parse", "FETCH_HEAD")
	if err != nil {
		return nil, err
	}
	r.CommitID = strings.TrimSuffix(commitGetOut, "\n")

	_, err = CmdExec(gitCmd, r.RootDir, "submodule", "update", "--init", "--recursive")
	if err != nil {
		return nil, err
	}
	return r, nil
}

func parseGitURLinKustomization(urlInKustomization string) (string, string, string) {
	host, orgRepo, path, gitRef, gitSuff := parseGitUrl(urlInKustomization)
	return host + orgRepo + gitSuff, gitRef, path
}

func CmdExec(baseCmd, dir string, args ...string) (string, error) {
	cmd := exec.Command(baseCmd, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if dir != "" {
		cmd.Dir = dir
	}
	err := cmd.Run()
	if err != nil {
		return "", errors.Wrap(err, stderr.String())
	}
	out := stdout.String()
	return out, nil
}

func IsRepositoryResource(path string) bool {
	host, orgRepo, _, _, _ := parseGitUrl(path)
	if host != "" && orgRepo != "" {
		return true
	}
	return false
}

func IsFileResource(path string) bool {
	return !IsRepositoryResource(path)
}

func IsFile(name string) (bool, error) {
	isDir, err := IsDir(name)
	if err != nil {
		return false, err
	}
	return !isDir, nil
}

func IsDir(name string) (bool, error) {
	fInfo, err := os.Stat(name)
	if err != nil {
		return false, err
	}
	return fInfo.IsDir(), nil
}

func FileExists(fpath string) bool {
	if _, err := os.Stat(fpath); err == nil {
		return true
	}
	return false
}

const (
	refQuery      = "?ref="
	refQueryRegex = "\\?(version|ref)="
	gitSuffix     = ".git"
	gitDelimiter  = "_git/"
)

// From strings like git@github.com:someOrg/someRepo.git or
// https://github.com/someOrg/someRepo?ref=someHash, extract
// the parts.
func parseGitUrl(n string) (
	host string, orgRepo string, path string, gitRef string, gitSuff string) {

	if strings.Contains(n, gitDelimiter) {
		index := strings.Index(n, gitDelimiter)
		// Adding _git/ to host
		host = normalizeGitHostSpec(n[:index+len(gitDelimiter)])
		orgRepo = strings.Split(strings.Split(n[index+len(gitDelimiter):], "/")[0], "?")[0]
		path, gitRef = peelQuery(n[index+len(gitDelimiter)+len(orgRepo):])
		return
	}
	host, n = parseHostSpec(n)
	gitSuff = gitSuffix
	if strings.Contains(n, gitSuffix) {
		index := strings.Index(n, gitSuffix)
		orgRepo = n[0:index]
		n = n[index+len(gitSuffix):]
		path, gitRef = peelQuery(n)
		return
	}

	i := strings.Index(n, "/")
	if i < 1 {
		return "", "", "", "", ""
	}
	j := strings.Index(n[i+1:], "/")
	if j >= 0 {
		j += i + 1
		orgRepo = n[:j]
		path, gitRef = peelQuery(n[j+1:])
		return
	}
	path = ""
	orgRepo, gitRef = peelQuery(n)
	return host, orgRepo, path, gitRef, gitSuff
}

func peelQuery(arg string) (string, string) {

	r, _ := regexp.Compile(refQueryRegex)
	j := r.FindStringIndex(arg)

	if len(j) > 0 {
		return arg[:j[0]], arg[j[0]+len(r.FindString(arg)):]
	}
	return arg, ""
}

func parseHostSpec(n string) (string, string) {
	var host string
	// Start accumulating the host part.
	for _, p := range []string{
		// Order matters here.
		"git::", "gh:", "ssh://", "https://", "http://",
		"git@", "github.com:", "github.com/"} {
		if len(p) < len(n) && strings.ToLower(n[:len(p)]) == p {
			n = n[len(p):]
			host += p
		}
	}
	if host == "git@" {
		i := strings.Index(n, "/")
		if i > -1 {
			host += n[:i+1]
			n = n[i+1:]
		} else {
			i = strings.Index(n, ":")
			if i > -1 {
				host += n[:i+1]
				n = n[i+1:]
			}
		}
		return host, n
	}

	// If host is a http(s) or ssh URL, grab the domain part.
	for _, p := range []string{
		"ssh://", "https://", "http://"} {
		if strings.HasSuffix(host, p) {
			i := strings.Index(n, "/")
			if i > -1 {
				host = host + n[0:i+1]
				n = n[i+1:]
			}
			break
		}
	}

	return normalizeGitHostSpec(host), n
}

func normalizeGitHostSpec(host string) string {
	s := strings.ToLower(host)
	if strings.Contains(s, "github.com") {
		if strings.Contains(s, "git@") || strings.Contains(s, "ssh:") {
			host = "git@github.com:"
		} else {
			host = "https://github.com/"
		}
	}
	if strings.HasPrefix(s, "git::") {
		host = strings.TrimPrefix(s, "git::")
	}
	return host
}
