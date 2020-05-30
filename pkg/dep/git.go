package dep

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

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
	outputDir := g.LocalDir()
	if _, err := Shellf(gitCloneShellScript(outputDir, g.remote, g.commit)); err != nil {
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

// gitCloneBashScript is a bash script to fetch Git files of a repo at a given commit sha, to a given path.
func gitCloneShellScript(outputDir, gitRemoteURL, gitCommitSHA string) string {
	variables := fmt.Sprintf(`
	OUTPUT_DIR="%s"
	GIT_REMOTE_URL="%s"
	GIT_COMMIT_SHA="%s"
`, outputDir, gitRemoteURL, gitCommitSHA)
	script := `
set -e
if [ -d "${OUTPUT_DIR}" ]
then
  # ${OUTPUT_DIR} already exists, meaning dependency version unchanged.
  exit 0
fi

mkdir -p "${OUTPUT_DIR}"
cd "${OUTPUT_DIR}"

git init

# GIT_REMOTE_URL is expected to have authentication in it
# it should be in the form of https://token@%repo.domain.git
git remote add origin "${GIT_REMOTE_URL}"

# try to fetch this single commit first
# this works for when there is a ref pointing at this commit
# for most commits that are newly pushed, this is the case
# because there is a head (branch) pointint at it
# if no ref points at that commit sha, fetch the entire repo
# this is common when rebuilding for an arbituary commit sha
(git fetch origin "${GIT_COMMIT_SHA}" && git reset --hard FETCH_HEAD) || \
(git fetch origin && git checkout "${GIT_COMMIT_SHA}")
`
	return strings.Join([]string{variables, script}, "\n")
}
