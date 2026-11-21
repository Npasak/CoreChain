package blocksdk

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func TestPackAndUnpackBlzZstd(t *testing.T) {
	tmp := t.TempDir()
	srcDir := filepath.Join(tmp, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}

	manifestJSON := `{"id":"testblock","name":"Test Block","version":"1.0.0","entrypoint":"block.exe"}`
	if err := os.WriteFile(filepath.Join(srcDir, "manifest.json"), []byte(manifestJSON), 0644); err != nil {
		t.Fatal(err)
	}
	payload := []byte("hello corechain zstd payload hello corechain zstd payload")
	if err := os.WriteFile(filepath.Join(srcDir, "block.exe"), payload, 0755); err != nil {
		t.Fatal(err)
	}

	blzPath := filepath.Join(tmp, "testblock.Blz")
	hash, err := PackBlz(srcDir, blzPath)
	if err != nil {
		t.Fatalf("PackBlz failed: %v", err)
	}
	if len(hash) != 64 {
		t.Fatalf("expected 64-char sha256 hex, got %q", hash)
	}

	// Verify ZIP entries use Zstd (method 93)
	zr, err := zip.OpenReader(blzPath)
	if err != nil {
		t.Fatalf("zip.OpenReader failed: %v", err)
	}
	for _, f := range zr.File {
		if f.Method != zstd.ZipMethodWinZip {
			t.Errorf("entry %s method = %d, want %d (Zstd)", f.Name, f.Method, zstd.ZipMethodWinZip)
		}
	}
	_ = zr.Close()

	// Verify UnpackBlz extracts cleanly
	destDir := filepath.Join(tmp, "dest")
	m, err := UnpackBlz(blzPath, destDir)
	if err != nil {
		t.Fatalf("UnpackBlz failed: %v", err)
	}
	if m.ID != "testblock" || m.Version != "1.0.0" {
		t.Fatalf("unexpected manifest: %+v", m)
	}
	got, err := os.ReadFile(filepath.Join(destDir, "block.exe"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("extracted payload mismatch: got %q, want %q", got, payload)
	}
}

func TestUnpackBlzLegacyDeflate(t *testing.T) {
	tmp := t.TempDir()
	blzPath := filepath.Join(tmp, "legacy.Blz")

	f, err := os.Create(blzPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.CreateHeader(&zip.FileHeader{Name: "manifest.json", Method: zip.Deflate})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(`{"id":"legacy","name":"Legacy","version":"0.1.0","entrypoint":"block.exe"}`)); err != nil {
		t.Fatal(err)
	}
	_ = zw.Close()
	_ = f.Close()

	m, err := UnpackBlz(blzPath, filepath.Join(tmp, "out"))
	if err != nil {
		t.Fatalf("UnpackBlz legacy deflate failed: %v", err)
	}
	if m.ID != "legacy" {
		t.Fatalf("unexpected manifest ID: %q", m.ID)
	}
}
