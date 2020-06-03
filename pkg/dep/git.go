// Copyright 2020 GM Cruise LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dep

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	log "github.com/golang/glog"
	"go.starlark.net/starlark"

	"github.com/cruise-automation/isopod/pkg/loader"
)

const (
	// NameKey is the name of the git repo target.
	NameKey = "name"
	// RemoteKey is the remote address of this git repo.
	RemoteKey = "remote"
	// CommitKey is the full commit SHA of the source to download.
	CommitKey = "commit"
)

var (
	// asserts *GitRepo implements starlark.HasAttrs interface.
	_ starlark.HasAttrs = (*GitRepo)(nil)
	// asserts *GitRepo implements loader.Dependency interface.
	_ loader.Dependency = (*GitRepo)(nil)

	// RequiredFields is the list of required fields to initialize a GitRepo target.
	RequiredFields = []string{NameKey, RemoteKey, CommitKey}
)

// GitRepo represents Isopod module source as remote git repo.
type GitRepo struct {
	*AbstractDependency
	name, remote, commit string
}

// NewGitRepoBuiltin creates a new git_repository built-in.
func NewGitRepoBuiltin() *starlark.Builtin {
	return starlark.NewBuiltin(
		"git_repository",
		func(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			absDep, err := NewAbstractDependency("git_repository", RequiredFields, kwargs)
			if err != nil {
				return nil, err
			}
			name, remote, commit, err := nameRemoteCommit(absDep)
			if err != nil {
				return nil, fmt.Errorf("cannot read params: %v", err)
			}
			gitRepo := &GitRepo{absDep, name, remote, commit}
			loader.Register(gitRepo)
			return gitRepo, nil
		},
	)
}

// Name returns the name of this git repo target.
func (g *GitRepo) Name() string {
	return g.name
}

// LocalDir returns the path to the directory storing the source.
func (g *GitRepo) LocalDir() string {
	return filepath.Join(Workspace, g.name, g.commit)
}

// Fetch is part of the Dependency interface.
// It downloads the source of this dependency.
func (g *GitRepo) Fetch() error {
	script, err := gitCloneShellScript(&GitCloneParams{
		OutputDir:    g.LocalDir(),
		GitRemoteURL: g.remote,
		GitCommitSHA: g.commit,
	})
	if err != nil {
		return err
	}
	if _, err := Shellf(script); err != nil {
		return fmt.Errorf("failed to clone git repo `%v': %v", g.name, err)
	}
	return nil
}

func nameRemoteCommit(absDep *AbstractDependency) (name, remote, commit string, err error) {
	if name, err = stringFromValue(absDep.Attrs[NameKey]); err != nil {
		return
	}
	if remote, err = stringFromValue(absDep.Attrs[RemoteKey]); err != nil {
		return
	}
	if commit, err = stringFromValue(absDep.Attrs[CommitKey]); err != nil {
		return
	}
	return
}

func stringFromValue(v starlark.Value) (string, error) {
	if v == nil {
		return "", errors.New("nil value")
	}
	s, ok := v.(starlark.String)
	if !ok {
		return "", fmt.Errorf("%v is not a starlark string (got a `%s')", v, v.Type())
	}
	return string(s), nil
}

// Shellf execute the given shell command and wait until it finishes. Then
// return the combined stdout and stderr, and error if any.
func Shellf(format string, a ...interface{}) (string, error) {
	s := fmt.Sprintf(format, a...)
	log.V(1).Infof("Executing shell command: %v\n", s)
	bytes, err := exec.Command("sh", "-c", s).CombinedOutput()
	log.V(1).Infof("Shell command `%s' finished:\n%s", s, string(bytes))
	return string(bytes), err
}

// GitCloneParams is used to templatize git clone command.
type GitCloneParams struct {
	OutputDir, GitRemoteURL, GitCommitSHA string
}

// gitCloneBashScript composes a shell script to clone a repo at a given commit sha.
func gitCloneShellScript(params *GitCloneParams) (string, error) {
	script := `
set -e
if [ -d "{{.OutputDir}}" ]; then
  # {{.OutputDir}} already exists, meaning dependency version unchanged.
  exit 0
fi

mkdir -p "{{.OutputDir}}"
cd "{{.OutputDir}}"

git init
git remote add origin "{{.GitRemoteURL}}"

# Try to fetch just the specified commit first, which only works if there is a ref
# pointing at this commit. This is true for commits that were just pushed.
# Otherwise, fetch the entire repo history, which supports checking out arbituary commits.
(git fetch origin "{{.GitCommitSHA}}" && git reset --hard FETCH_HEAD) || \
(git fetch origin && git checkout "{{.GitCommitSHA}}")
`
	tpl, err := template.New("script").Parse(script)
	if err != nil {
		return "", fmt.Errorf("failed to parse git-clone shell script template: %v", err)
	}
	var sb strings.Builder
	tpl.Execute(&sb, params)
	if err != nil {
		return "", fmt.Errorf("failed to render git-clone shell script template: %v", err)
	}
	return sb.String(), nil
}
