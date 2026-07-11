package server

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func writeZip(t *testing.T, path string, files map[string][]byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

func samplePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func validArchiveFiles(t *testing.T) map[string][]byte {
	t.Helper()
	return map[string][]byte{
		"manifest.json": []byte(`{"formatVersion":5}`),
		"document.json": []byte(`{"title":"demo","formatVersion":5,"pages":[],"elements":[]}`),
		"thumbnail.png": samplePNG(t, 64, 64),
	}
}

func TestValidateTNoteArchiveAcceptsValidPackage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ok.tnote")
	writeZip(t, path, validArchiveFiles(t))
	got, err := ValidateTNoteArchive(path, defaultArchiveLimits(10<<20))
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "demo" || len(got.ThumbnailPNG) == 0 {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestValidateTNoteArchiveRejectsTraversal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.tnote")
	files := validArchiveFiles(t)
	files["../evil.txt"] = []byte("x")
	writeZip(t, path, files)
	if _, err := ValidateTNoteArchive(path, defaultArchiveLimits(10<<20)); err == nil {
		t.Fatal("expected traversal rejection")
	}
}

func TestValidateTNoteArchiveRequiresThumbnail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-thumb.tnote")
	files := validArchiveFiles(t)
	delete(files, "thumbnail.png")
	writeZip(t, path, files)
	_, err := ValidateTNoteArchive(path, defaultArchiveLimits(10<<20))
	if err == nil || err.Error() != "thumbnail_required" {
		t.Fatalf("error = %v, want thumbnail_required", err)
	}
}

func TestValidateTNoteArchiveRejectsFakePNG(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fake.tnote")
	files := validArchiveFiles(t)
	files["thumbnail.png"] = []byte("not-a-png")
	writeZip(t, path, files)
	if _, err := ValidateTNoteArchive(path, defaultArchiveLimits(10<<20)); err == nil {
		t.Fatal("expected fake png rejection")
	}
}

func TestValidatePNGThumbnailRejectsHugeDimensions(t *testing.T) {
	// Craft a minimal PNG header with absurd dimensions without allocating the image.
	var buf bytes.Buffer
	buf.Write([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A})
	// length=13, type=IHDR
	_ = binary.Write(&buf, binary.BigEndian, uint32(13))
	buf.WriteString("IHDR")
	_ = binary.Write(&buf, binary.BigEndian, uint32(100000))
	_ = binary.Write(&buf, binary.BigEndian, uint32(100000))
	if err := validatePNGThumbnail(buf.Bytes(), defaultMaxThumbnailPixels); err == nil {
		t.Fatal("expected oversized thumbnail rejection")
	}
}
