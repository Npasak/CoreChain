package blocksdk

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const PackageExtension = ".Blz"

// Manifest describes a .Blz block package.
type Manifest struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Description    string            `json:"description"`
	Version        string            `json:"version"`
	MinCoreVersion string            `json:"min_core_version,omitempty"`
	Icon           string            `json:"icon"`
	Color          string            `json:"color"`
	Entrypoint     string            `json:"entrypoint"` // e.g. "block.exe" on Windows, "block" on Unix
	CPUThreads     int               `json:"cpu_threads"` // GOMAXPROCS per block worker (0 = auto)
	Capabilities   []string          `json:"capabilities,omitempty"`
	Connectors     []ConnectorSpec   `json:"connectors,omitempty"`
	SubscribedTopics []string        `json:"subscribed_topics,omitempty"`
	DefaultEnabled bool              `json:"default_enabled"`
	LocalOnly      bool              `json:"local_only,omitempty"`
	Order          int               `json:"order"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// ConnectorSpec declares an RPC action exposed by a block on the Connector Bus.
type ConnectorSpec struct {
	Action      string `json:"action"`
	Description string `json:"description"`
}

// ReadManifest reads manifest.json from a directory.
func ReadManifest(dir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("read manifest.json: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest.json: %w", err)
	}
	if m.ID == "" {
		return nil, fmt.Errorf("manifest.id is required")
	}
	if m.Entrypoint == "" {
		m.Entrypoint = "block.exe"
	}
	return &m, nil
}

// PackBlz packages a block directory (containing manifest.json & block binary) into a .Blz archive.
func PackBlz(srcDir, outBlzPath string) (string, error) {
	manifest, err := ReadManifest(srcDir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(outBlzPath), 0755); err != nil {
		return "", err
	}

	outFile, err := os.Create(outBlzPath)
	if err != nil {
		return "", err
	}
	defer outFile.Close()

	zw := zip.NewWriter(outFile)
	err = filepath.Walk(srcDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		// Normalize to forward slashes inside zip
		rel = filepath.ToSlash(rel)
		// Skip temporary/db files if any
		if strings.HasSuffix(rel, ".db") || strings.HasSuffix(rel, ".db-wal") || strings.HasSuffix(rel, ".db-shm") {
			return nil
		}

		hdr, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		hdr.Name = rel
		hdr.Method = zip.Deflate

		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(w, f)
		return err
	})
	if err != nil {
		_ = zw.Close()
		return "", err
	}
	if err := zw.Close(); err != nil {
		return "", err
	}
	_ = outFile.Close()

	hash, err := FileSHA256(outBlzPath)
	if err != nil {
		return "", err
	}
	_ = manifest
	return hash, nil
}

// UnpackBlz extracts a .Blz package into destDir safely (preventing ZipSlip).
func UnpackBlz(blzPath, destDir string) (*Manifest, error) {
	r, err := zip.OpenReader(blzPath)
	if err != nil {
		return nil, fmt.Errorf("open .Blz package: %w", err)
	}
	defer r.Close()

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return nil, err
	}
	cleanDest := filepath.Clean(destDir) + string(os.PathSeparator)

	for _, f := range r.File {
		target := filepath.Join(destDir, filepath.FromSlash(f.Name))
		if !strings.HasPrefix(filepath.Clean(target)+string(os.PathSeparator), cleanDest) && filepath.Clean(target) != filepath.Clean(destDir) {
			return nil, fmt.Errorf("illegal path in .Blz: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return nil, err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return nil, err
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			_ = rc.Close()
			return nil, err
		}
		_, copyErr := io.Copy(out, rc)
		_ = out.Close()
		_ = rc.Close()
		if copyErr != nil {
			return nil, copyErr
		}
	}

	return ReadManifest(destDir)
}

// FileSHA256 calculates the hex SHA-256 checksum of a file.
func FileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
