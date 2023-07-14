// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/apigee/registry/pkg/encoding"
	"github.com/apigee-apihub-demo/import-google-apis/pkg/fetch"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	yaml "gopkg.in/yaml.v3"
)

var dir = "deps"
var top = "googleapis"
var provider = "google.com"
var source = "import-google-apis"
var updated = time.Now().Format("2006-01-02")

// List source repos.
var deps = []string{
	"https://github.com/googleapis/googleapis",
}
var out = "apis/google.com"

func main() {
	err := fetch.FetchDependencies(dir, deps)
	if err != nil {
		panic(err)
	}
	allProtos, err := listAllProtos(dir)
	if err != nil {
		panic(err)
	}

	index, err := readIndex(filepath.Join(dir, top))
	if err != nil {
		panic(err)
	}
	fmt.Printf("%d APIs\n", len(index.APIs))
	for _, api := range index.APIs {
		err := describeAPI(api, filepath.Join(dir, top), allProtos)
		if err != nil {
			log.Fatalf("error processing %s: %s", api, err)
		}
	}
}

type ApiIndex struct {
	APIs []*ApiIndexEntry `yaml:"apis"`
}

type ApiIndexEntry struct {
	Id                  string   `yaml:"id"`
	Directory           string   `yaml:"directory"`
	Version             string   `yaml:"version"`
	MajorVersion        string   `yaml:"majorVersion"`
	HostName            string   `yaml:"hostName"`
	Title               string   `yaml:"title"`
	Description         string   `yaml:"description"`
	ImportDirectories   []string `yaml:"importDirectories"`
	ConfigFile          string   `yaml:"configFile"`
	NameInServiceConfig string   `yaml:"nameInServiceConfig"`
}

func readIndex(dir string) (*ApiIndex, error) {
	p := filepath.Join(dir, "api-index-v1.json")
	bytes, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	index := &ApiIndex{}
	if err := yaml.Unmarshal(bytes, index); err != nil {
		return nil, err
	}
	return index, nil
}

// Build a list of all proto files in a directory.
func listAllProtos(dir string) ([]string, error) {
	protos := make([]string, 0)
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(p, ".proto") {
			protos = append(protos, p)
		}
		return nil
	})
	return protos, err
}

// Compile an API description and create Registry YAML.
func describeAPI(apiIndex *ApiIndexEntry, root string, allProtos []string) error {
	bytes, err := yaml.Marshal(apiIndex)
	if err != nil {
		return err
	}
	fmt.Printf("%s\n", bytes)
	container := filepath.Join(root, apiIndex.Directory)
	log.Printf("%s\n", container)
	// Get all of the protos in the specified container.
	protos := make([]string, 0)
	err = filepath.Walk(container, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(p, ".proto") {
			protos = append(protos, p)
		}
		return nil
	})
	if err != nil {
		return err
	}
	// Skip APIs with no protos.
	if len(protos) == 0 {
		log.Printf("%s has no protos!", apiIndex.Title)
		return nil
	}
	// Compile the protos and get a list of everything they import.
	all, err := referencedProtos(protos, "")
	if err != nil {
		return err
	}

	// Get the apiID and versionID for use in Registry YAML.
	apiID := strings.TrimSuffix(apiIndex.NameInServiceConfig, ".googleapis.com")
	versionID := apiIndex.Version
	specID := "protos"

	// Make the directory for the API and version YAML.
	err = os.MkdirAll(filepath.Join(out, apiID, versionID, specID), 0777)
	if err != nil {
		return err
	}

	// Collect the listed files into the spec directory.
	specDir := filepath.Join(out, apiID, versionID, specID)
	for _, a := range all {
		for _, p := range allProtos {
			if strings.HasSuffix(p, a) {
				err := copyFile(p, filepath.Join(specDir, a))
				if err != nil {
					return err
				}
				break
			}
		}
	}

	// If we have service config, copy that into the archive.
	serviceConfigPath := filepath.Join(container, apiIndex.ConfigFile)
	localPath := filepath.Join(specDir, apiIndex.Directory, apiIndex.ConfigFile)
	err = copyFile(serviceConfigPath, localPath)
	if err != nil {
		return err
	}

	// Build and save info.yaml for the spec.
	apiSpec := &encoding.ApiSpec{
		Header: encoding.Header{
			ApiVersion: encoding.RegistryV1,
			Kind:       "Spec",
			Metadata: encoding.Metadata{
				Parent: "apis/" + provider + "-" + apiID + "/versions/" + versionID,
				Name:   specID,
				Labels: map[string]string{
					"provider": strings.ReplaceAll(provider, ".", "-"),
					"updated":  updated,
					"source":   source,
				},
				Annotations: map[string]string{
					"config":    apiIndex.ConfigFile,
					"directory": apiIndex.Directory,
					"host":      apiIndex.HostName,
				},
			},
		},
		Data: encoding.ApiSpecData{
			FileName: "protos.zip",
			MimeType: "application/x.protobuf+zip",
		},
	}
	b, err := encoding.EncodeYAML(apiSpec)
	if err != nil {
		return err
	}
	name := filepath.Join(out, apiID, versionID, specID, "info.yaml")
	err = os.WriteFile(name, b, 0664)
	if err != nil {
		return err
	}

	// Build and save info.yaml for the version.
	apiVersion := &encoding.ApiVersion{
		Header: encoding.Header{
			ApiVersion: encoding.RegistryV1,
			Kind:       "Version",
			Metadata: encoding.Metadata{
				Parent: "apis/" + provider + "-" + apiID,
				Name:   versionID,
				Labels: map[string]string{
					"provider": strings.ReplaceAll(provider, ".", "-"),
					"updated":  updated,
					"source":   source,
				},
			},
		},
		Data: encoding.ApiVersionData{
			DisplayName: versionID,
		},
	}
	b, err = encoding.EncodeYAML(apiVersion)
	if err != nil {
		return err
	}
	name = filepath.Join(out, apiID, versionID, "info.yaml")
	err = os.WriteFile(name, b, 0664)
	if err != nil {
		return err
	}

	// Build and save info.yaml for the API.
	api := &encoding.Api{
		Header: encoding.Header{
			ApiVersion: encoding.RegistryV1,
			Kind:       "API",
			Metadata: encoding.Metadata{
				Name: provider + "-" + apiID,
				Labels: map[string]string{
					"provider": strings.ReplaceAll(provider, ".", "-"),
					"updated":  updated,
					"source":   source,
				},
			},
		},
		Data: encoding.ApiData{
			DisplayName: displayName(apiIndex.Title),
		},
	}
	b, err = encoding.EncodeYAML(api)
	if err != nil {
		return err
	}
	name = filepath.Join(out, apiID, "info.yaml")
	err = os.WriteFile(name, b, 0664)
	if err != nil {
		return err
	}
	// Done!
	return nil
}

// Copy a file from one path to another, ensuring that the destination directory exists.
func copyFile(src, dest string) error {
	dir := filepath.Dir(dest)
	err := os.MkdirAll(dir, 0777)
	if err != nil {
		return err
	}
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, input, 0644)
}

// Get all the protos that are referenced in the compilation of a list of protos.
// "root" is the root directory for the compilation.
func referencedProtos(protos []string, root string) ([]string, error) {
	tempDir, err := os.MkdirTemp("", "proto-import-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempDir)
	args := []string{"-o", tempDir + "/proto.pb", "--include_imports"}

	imports := []string{}
	for _, d := range deps {
		imports = append(imports, "-I")
		imports = append(imports, strings.ReplaceAll(filepath.Join(dir, filepath.Base(d)), ";", "/"))
	}

	args = append(args, imports...)
	args = append(args, protos...)
	cmd := exec.Command("protoc", args...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("%s", string(out))
		return nil, fmt.Errorf("failed to compile protos with protoc: %s", err)
	}
	return protosFromFileDescriptorSet(tempDir + "/proto.pb")
}

// Get all the protos listed as dependencies in a file descriptor set.
func protosFromFileDescriptorSet(filename string) ([]string, error) {
	bytes, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	fds := &descriptorpb.FileDescriptorSet{}
	err = proto.Unmarshal(bytes, fds)
	if err != nil {
		return nil, err
	}
	filenameset := make(map[string]bool)
	for _, file := range fds.File {
		filename = *file.Name
		if !strings.HasPrefix(filename, "google/protobuf/") {
			filenameset[filename] = true
		}
	}
	filenames := make([]string, 0)
	for k := range filenameset {
		filenames = append(filenames, k)
	}
	sort.Strings(filenames)
	return filenames, nil
}

func displayName(name string) string {
	if !strings.HasPrefix(name, "Google") {
		name = "Google " + name
	}
	return name
}
