package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// DirChecksum returns a sha256 over the sorted relative path and content of
// every regular file under dir, following symlinks (the Hugging Face cache
// stores files as symlinks into a content-addressed blob store). This is
// the multi-file analogue of Checksum, for backends like mlx whose models
// are a directory of files (safetensors, tokenizer, config) rather than a
// single GGUF file.
func DirChecksum(dir string) (string, error) {
	files, err := listFiles(dir)
	if err != nil {
		return "", err
	}

	h := sha256.New()
	for _, f := range files {
		rel, err := filepath.Rel(dir, f)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s\n", rel)
		if err := hashFileInto(h, f); err != nil {
			return "", fmt.Errorf("hash %s: %w", f, err)
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// DirSize returns the total size in bytes of every regular file under dir,
// following symlinks.
func DirSize(dir string) (uint64, error) {
	files, err := listFiles(dir)
	if err != nil {
		return 0, err
	}

	var total uint64
	for _, f := range files {
		info, err := os.Stat(f) // os.Stat follows symlinks
		if err != nil {
			return 0, fmt.Errorf("stat %s: %w", f, err)
		}
		total += uint64(info.Size())
	}
	return total, nil
}

// listFiles returns every regular file under dir (symlinks included, not
// followed for the walk itself — only their target is read/stat'd by the
// caller), sorted for deterministic ordering.
func listFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", dir, err)
	}
	sort.Strings(files)
	return files, nil
}

func hashFileInto(h io.Writer, path string) error {
	f, err := os.Open(path) // os.Open follows symlinks
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(h, f)
	return err
}
