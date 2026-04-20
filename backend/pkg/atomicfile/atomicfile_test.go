package atomicfile

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileBasic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	data := []byte("hello world")

	if err := WriteFile(path, data, nil); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("content = %q, want %q", got, data)
	}
}

func TestWriteFileCreatesDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "deep", "file.txt")

	if err := WriteFile(path, []byte("nested"), nil); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "nested" {
		t.Errorf("content = %q, want nested", got)
	}
}

func TestWriteFileOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "overwrite.txt")

	if err := WriteFile(path, []byte("first"), nil); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(path, []byte("second"), nil); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "second" {
		t.Errorf("content = %q, want second", got)
	}
}

func TestWriteFileWithFsync(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fsync.txt")
	opts := &Options{Fsync: true}

	if err := WriteFile(path, []byte("synced"), opts); err != nil {
		t.Fatalf("WriteFile with Fsync: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "synced" {
		t.Errorf("content = %q, want synced", got)
	}
}

func TestWriteFileWithBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "backup.txt")
	opts := &Options{Backup: true}

	if err := WriteFile(path, []byte("original"), opts); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(path, []byte("updated"), opts); err != nil {
		t.Fatal(err)
	}

	// Check backup exists with original content
	bak, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf("backup file: %v", err)
	}
	if string(bak) != "original" {
		t.Errorf("backup = %q, want original", bak)
	}

	// Check main file has new content
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "updated" {
		t.Errorf("content = %q, want updated", got)
	}
}

func TestWriteFileCustomPerm(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "perm.txt")
	opts := &Options{Perm: 0o600}

	if err := WriteFile(path, []byte("secret"), opts); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("perm = %o, want 600", perm)
	}
}

func TestWriteFileNoTmpLeftOver(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clean.txt")

	if err := WriteFile(path, []byte("data"), nil); err != nil {
		t.Fatal(err)
	}

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		name := e.Name()
		if name != "clean.txt" && name != "clean.txt.lock" {
			t.Errorf("unexpected leftover file: %s", name)
		}
	}
}
