package handler

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
)

const (
	maxUploadSize = 50 << 20 // 50 MB
	uploadDir     = "uploads"
)

// Upload handles multipart file uploads. It stores the file locally and returns
// the relative URL path that can be saved into a file-type field.
//
// POST /api/upload
// Content-Type: multipart/form-data; field name = "file"
//
// Response: { "data": { "url": "/api/uploads/abc123.pdf", "name": "original.pdf", "size": 12345 } }
func Upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		writeError(w, http.StatusBadRequest, "file too large (max 50MB)")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()

	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		handleErr(w, r, fmt.Errorf("create upload dir: %w", err))
		return
	}

	// Generate a random filename to avoid collisions, preserving the original extension.
	ext := filepath.Ext(header.Filename)
	randBytes := make([]byte, 16)
	if _, err := rand.Read(randBytes); err != nil {
		handleErr(w, r, fmt.Errorf("generate filename: %w", err))
		return
	}
	storedName := hex.EncodeToString(randBytes) + sanitizeExt(ext)
	destPath := filepath.Join(uploadDir, storedName)

	dst, err := os.Create(destPath)
	if err != nil {
		handleErr(w, r, fmt.Errorf("create file: %w", err))
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		dst.Close()
		os.Remove(destPath)
		handleErr(w, r, fmt.Errorf("write file: %w", err))
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"url":  "/api/uploads/" + storedName,
		"name": header.Filename,
		"size": header.Size,
	})
}

// ServeUpload serves a single uploaded file by its stored filename.
// It rejects any path containing separators or dot-dot sequences to prevent traversal.
func ServeUpload(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "filename")

	// Block path traversal: no slashes, backslashes, or ".." allowed.
	if name == "" || strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		http.NotFound(w, r)
		return
	}

	// Resolve and verify the file is inside the uploads directory.
	abs, err := filepath.Abs(filepath.Join(uploadDir, name))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	base, _ := filepath.Abs(uploadDir)
	if !strings.HasPrefix(abs, base+string(filepath.Separator)) {
		http.NotFound(w, r)
		return
	}

	http.ServeFile(w, r, abs)
}

// sanitizeExt ensures the extension only contains safe characters.
func sanitizeExt(ext string) string {
	ext = strings.ToLower(ext)
	cleaned := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' {
			return r
		}
		return -1
	}, ext)
	if cleaned == "" || cleaned == "." {
		return ""
	}
	if cleaned[0] != '.' {
		cleaned = "." + cleaned
	}
	return cleaned
}
