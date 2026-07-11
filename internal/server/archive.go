package server

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

const (
	defaultMaxArchiveEntries     = 2000
	defaultMaxUncompressedTotal  = 512 * 1024 * 1024
	defaultMaxUncompressedEntry  = 128 * 1024 * 1024
	defaultMaxExpansionRatio     = 40
	defaultMaxThumbnailPixels    = 4096 * 4096
	defaultMaxJSONBytes          = 8 * 1024 * 1024
)

type ArchiveLimits struct {
	MaxEntries            int
	MaxUncompressedTotal  int64
	MaxUncompressedEntry  int64
	MaxExpansionRatio     int64
	MaxThumbnailPixels    int64
	MaxJSONBytes          int64
}

type ValidatedArchive struct {
	Title         string
	ThumbnailPNG  []byte
	FormatVersion int
}

func defaultArchiveLimits(maxUploadBytes int64) ArchiveLimits {
	limits := ArchiveLimits{
		MaxEntries:           defaultMaxArchiveEntries,
		MaxUncompressedTotal: defaultMaxUncompressedTotal,
		MaxUncompressedEntry: defaultMaxUncompressedEntry,
		MaxExpansionRatio:    defaultMaxExpansionRatio,
		MaxThumbnailPixels:   defaultMaxThumbnailPixels,
		MaxJSONBytes:         defaultMaxJSONBytes,
	}
	if maxUploadBytes > 0 {
		// Keep total uncompressed bound proportional to configured upload size.
		if maxUploadBytes*int64(defaultMaxExpansionRatio) < limits.MaxUncompressedTotal {
			limits.MaxUncompressedTotal = maxUploadBytes * int64(defaultMaxExpansionRatio)
		}
	}
	return limits
}

func ValidateTNoteArchive(path string, limits ArchiveLimits) (*ValidatedArchive, error) {
	if limits.MaxEntries <= 0 {
		limits.MaxEntries = defaultMaxArchiveEntries
	}
	if limits.MaxUncompressedTotal <= 0 {
		limits.MaxUncompressedTotal = defaultMaxUncompressedTotal
	}
	if limits.MaxUncompressedEntry <= 0 {
		limits.MaxUncompressedEntry = defaultMaxUncompressedEntry
	}
	if limits.MaxExpansionRatio <= 0 {
		limits.MaxExpansionRatio = defaultMaxExpansionRatio
	}
	if limits.MaxThumbnailPixels <= 0 {
		limits.MaxThumbnailPixels = defaultMaxThumbnailPixels
	}
	if limits.MaxJSONBytes <= 0 {
		limits.MaxJSONBytes = defaultMaxJSONBytes
	}

	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("invalid zip: %w", err)
	}
	defer zr.Close()

	if len(zr.File) == 0 {
		return nil, errors.New("empty archive")
	}
	if len(zr.File) > limits.MaxEntries {
		return nil, fmt.Errorf("too many archive entries: %d", len(zr.File))
	}

	var compressedTotal int64
	var uncompressedTotal int64
	names := make(map[string]struct{}, len(zr.File))
	var manifestFile, documentFile, thumbFile *zip.File

	for _, f := range zr.File {
		name, err := safeArchiveName(f.Name)
		if err != nil {
			return nil, err
		}
		if _, exists := names[name]; exists {
			return nil, fmt.Errorf("duplicate archive entry %q", name)
		}
		names[name] = struct{}{}
		if f.UncompressedSize64 > uint64(limits.MaxUncompressedEntry) {
			return nil, fmt.Errorf("entry too large: %s", name)
		}
		uncompressedTotal += int64(f.UncompressedSize64)
		if uncompressedTotal > limits.MaxUncompressedTotal {
			return nil, errors.New("archive uncompressed size too large")
		}
		compressedTotal += int64(f.CompressedSize64)
		switch name {
		case "manifest.json":
			manifestFile = f
		case "document.json":
			documentFile = f
		case "thumbnail.png":
			thumbFile = f
		}
	}
	if compressedTotal > 0 && uncompressedTotal/compressedTotal > limits.MaxExpansionRatio {
		return nil, errors.New("archive expansion ratio too high")
	}
	if manifestFile == nil {
		return nil, errors.New("missing manifest.json")
	}
	if documentFile == nil {
		return nil, errors.New("missing document.json")
	}
	if thumbFile == nil {
		return nil, errors.New("thumbnail_required")
	}

	manifestRaw, err := readZipFileLimited(manifestFile, limits.MaxJSONBytes)
	if err != nil {
		return nil, fmt.Errorf("read manifest.json: %w", err)
	}
	documentRaw, err := readZipFileLimited(documentFile, limits.MaxJSONBytes)
	if err != nil {
		return nil, fmt.Errorf("read document.json: %w", err)
	}
	thumbRaw, err := readZipFileLimited(thumbFile, limits.MaxUncompressedEntry)
	if err != nil {
		return nil, fmt.Errorf("read thumbnail.png: %w", err)
	}
	if err := validatePNGThumbnail(thumbRaw, limits.MaxThumbnailPixels); err != nil {
		return nil, err
	}

	var manifest struct {
		FormatVersion int `json:"formatVersion"`
	}
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return nil, fmt.Errorf("invalid manifest.json: %w", err)
	}
	var document struct {
		Title         string `json:"title"`
		FormatVersion int    `json:"formatVersion"`
	}
	if err := json.Unmarshal(documentRaw, &document); err != nil {
		return nil, fmt.Errorf("invalid document.json: %w", err)
	}
	if document.Title == "" {
		document.Title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	version := document.FormatVersion
	if version == 0 {
		version = manifest.FormatVersion
	}
	return &ValidatedArchive{
		Title:         document.Title,
		ThumbnailPNG:  thumbRaw,
		FormatVersion: version,
	}, nil
}

func safeArchiveName(name string) (string, error) {
	name = filepath.ToSlash(strings.TrimSpace(name))
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "../") || strings.Contains(name, "..\\") || name == ".." {
		return "", fmt.Errorf("unsafe archive entry %q", name)
	}
	cleaned := filepath.ToSlash(filepath.Clean(name))
	if cleaned == "." || strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "/") {
		return "", fmt.Errorf("unsafe archive entry %q", name)
	}
	return cleaned, nil
}

func readZipFileLimited(f *zip.File, maxBytes int64) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	limited := io.LimitReader(rc, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, errors.New("entry exceeds size limit")
	}
	return data, nil
}

func validatePNGThumbnail(data []byte, maxPixels int64) error {
	if len(data) < 24 {
		return errors.New("thumbnail is not a valid PNG")
	}
	signature := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	if !bytes.Equal(data[:8], signature) {
		return errors.New("thumbnail is not a valid PNG")
	}
	// IHDR length(4)+type(4)+width(4)+height(4)
	if string(data[12:16]) != "IHDR" {
		return errors.New("thumbnail missing IHDR")
	}
	width := binary.BigEndian.Uint32(data[16:20])
	height := binary.BigEndian.Uint32(data[20:24])
	if width == 0 || height == 0 {
		return errors.New("thumbnail has invalid dimensions")
	}
	if int64(width)*int64(height) > maxPixels {
		return errors.New("thumbnail too large")
	}
	return nil
}
