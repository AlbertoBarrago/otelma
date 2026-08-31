package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirSize_SumsRegularFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("12345"), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("1234567"), 0o644); err != nil {
		t.Fatalf("write sub/b.txt: %v", err)
	}

	got, err := DirSize(dir)
	if err != nil {
		t.Fatalf("DirSize failed: %v", err)
	}
	if want := uint64(5 + 7); got != want {
		t.Fatalf("DirSize = %d, want %d", got, want)
	}
}

func TestDirSize_FollowsSymlinks(t *testing.T) {
	dir := t.TempDir()
	realDir := t.TempDir()
	target := filepath.Join(realDir, "blob")
	if err := os.WriteFile(target, []byte("0123456789"), 0o644); err != nil {
		t.Fatalf("write blob: %v", err)
	}
	link := filepath.Join(dir, "linked.bin")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	got, err := DirSize(dir)
	if err != nil {
		t.Fatalf("DirSize failed: %v", err)
	}
	if got != 10 {
		t.Fatalf("DirSize should follow the symlink to its 10-byte target, got %d", got)
	}
}

func TestDirChecksum_DeterministicAndSensitiveToContent(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	for _, dir := range []string{dir1, dir2} {
		if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("world"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	sum1, err := DirChecksum(dir1)
	if err != nil {
		t.Fatalf("DirChecksum(dir1) failed: %v", err)
	}
	sum2, err := DirChecksum(dir2)
	if err != nil {
		t.Fatalf("DirChecksum(dir2) failed: %v", err)
	}
	if sum1 != sum2 {
		t.Fatalf("expected identical checksums for identical directory contents, got %s vs %s", sum1, sum2)
	}

	if err := os.WriteFile(filepath.Join(dir2, "b.txt"), []byte("WORLD"), 0o644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	sum2Changed, err := DirChecksum(dir2)
	if err != nil {
		t.Fatalf("DirChecksum(dir2) after change failed: %v", err)
	}
	if sum1 == sum2Changed {
		t.Fatal("expected checksum to change when a file's content changes")
	}
}
