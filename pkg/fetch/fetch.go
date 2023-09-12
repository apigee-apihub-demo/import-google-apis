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

package fetch

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Use git clone to fetch a list of repo dependencies.
func FetchDependencies(dir string, deps []string) error {
	err := os.MkdirAll(dir, 0777)
	if err != nil {
		return err
	}
	for _, d := range deps {
		parts := strings.Split(d, ";") // the second part is the path to the protos
		d = parts[0]
		target := filepath.Join(dir, filepath.Base(d))
		if exists(target) {
			continue
		}
		cmd := exec.Command("git", "clone", d)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Printf("%s", string(out))
			return err
		}
	}
	return nil
}

// Return true if a file exists.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Get the commit hash of a cloned directory.
func CommitHash(path string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = path
	if b, err := cmd.CombinedOutput(); err != nil {
		return "", err
	} else {
		return strings.TrimSpace(string(b)), nil
	}
}
