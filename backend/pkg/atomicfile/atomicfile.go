// Package atomicfile provides concurrency-safe file writes using
// flock + tmp-file + atomic-rename.
// Ported from Deneb gateway.
package atomicfile

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

type Options struct {
	Perm    os.FileMode
	DirPerm os.FileMode
	Fsync   bool
	Backup  bool
}

func (o *Options) perm() os.FileMode {
	if o != nil && o.Perm != 0 {
		return o.Perm
	}
	return 0o644
}

func (o *Options) dirPerm() os.FileMode {
	if o != nil && o.DirPerm != 0 {
		return o.DirPerm
	}
	return 0o755
}

func WriteFile(path string, data []byte, opts *Options) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, opts.dirPerm()); err != nil {
		return fmt.Errorf("atomicfile: mkdir %s: %w", dir, err)
	}

	lockPath := path + ".lock"
	lockFd, err := os.OpenFile(lockPath, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("atomicfile: open lock %s: %w", lockPath, err)
	}
	defer lockFd.Close()

	if err := syscall.Flock(int(lockFd.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("atomicfile: flock %s: %w", lockPath, err)
	}
	defer syscall.Flock(int(lockFd.Fd()), syscall.LOCK_UN) //nolint:errcheck

	randBytes := make([]byte, 8)
	if _, err := rand.Read(randBytes); err != nil {
		return fmt.Errorf("atomicfile: random: %w", err)
	}
	tmp := fmt.Sprintf("%s.%d.%s.tmp", path, os.Getpid(), hex.EncodeToString(randBytes))

	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, opts.perm())
	if err != nil {
		return fmt.Errorf("atomicfile: create temp %s: %w", tmp, err)
	}

	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("atomicfile: write temp: %w", err)
	}

	if opts != nil && opts.Fsync {
		if err := f.Sync(); err != nil {
			f.Close()
			os.Remove(tmp)
			return fmt.Errorf("atomicfile: fsync temp: %w", err)
		}
	}

	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("atomicfile: close temp: %w", err)
	}

	if opts != nil && opts.Backup {
		if _, statErr := os.Stat(path); statErr == nil {
			data, _ := os.ReadFile(path)
			if data != nil {
				_ = os.WriteFile(path+".bak", data, 0o600)
			}
		}
	}

	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("atomicfile: rename: %w", err)
	}
	return nil
}
