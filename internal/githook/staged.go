package githook

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"unicode/utf8"

	"github.com/rchehd/phpwarden/internal/projectpath"
)

type Snapshot struct {
	Root      string
	Files     []string
	index     [32]byte
	indexFile string
}

func Read(ctx context.Context, directory string) (Snapshot, error) {
	top, err := root(ctx, directory)
	if err != nil {
		return Snapshot{}, err
	}
	indexFile := os.Getenv("GIT_INDEX_FILE")
	if indexFile != "" && !filepath.IsAbs(indexFile) {
		indexFile = filepath.Join(top, indexFile)
	}
	conflicts, err := gitWithIndex(ctx, top, indexFile, "ls-files", "--unmerged", "-z")
	if err != nil {
		return Snapshot{}, err
	}
	if len(conflicts) > 0 {
		return Snapshot{}, fmt.Errorf("resolve unmerged files before running pre-commit")
	}
	index, err := gitWithIndex(ctx, top, indexFile, "ls-files", "--stage", "-z")
	if err != nil {
		return Snapshot{}, err
	}
	changed, err := gitWithIndex(ctx, top, indexFile, "diff", "--cached", "--name-only", "--diff-filter=ACMRT", "--no-renames", "--no-ext-diff", "--no-textconv", "-z", "--")
	if err != nil {
		return Snapshot{}, err
	}
	result := Snapshot{Root: top, index: sha256.Sum256(index), indexFile: indexFile}
	for _, name := range bytes.Split(changed, []byte{0}) {
		if len(name) == 0 {
			continue
		}
		if !utf8.Valid(name) {
			return Snapshot{}, fmt.Errorf("staged filename is not valid UTF-8")
		}
		if err := projectpath.Validate(string(name)); err != nil {
			return Snapshot{}, err
		}
		result.Files = append(result.Files, string(name))
	}
	return result, nil
}

func (snapshot Snapshot) Check(ctx context.Context, selected []string) error {
	index, err := gitWithIndex(ctx, snapshot.Root, snapshot.indexFile, "ls-files", "--stage", "-z")
	if err != nil {
		return err
	}
	if sha256.Sum256(index) != snapshot.index {
		return fmt.Errorf("Git index changed during pre-commit; run git commit again")
	}
	unstaged, err := gitWithIndex(ctx, snapshot.Root, snapshot.indexFile, "diff-files", "--name-only", "--no-ext-diff", "--ignore-submodules=all", "-z", "--")
	if err != nil {
		return err
	}
	for _, name := range bytes.Split(unstaged, []byte{0}) {
		if len(name) > 0 && slices.Contains(selected, string(name)) {
			return fmt.Errorf("selected staged file %q also has unstaged changes; stage its remaining changes or restore its working-tree version to the staged content before committing", string(name))
		}
	}
	return nil
}
