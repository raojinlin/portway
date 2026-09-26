package daemon

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuiltinSkillBundleAndDownload(t *testing.T) {
	bundle, archive, err := loadBuiltinSkill()
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Name != "portway" || len(bundle.Files) != 2 || bundle.Files[0].Path != "SKILL.md" {
		t.Fatalf("unexpected bundle: %+v", bundle)
	}
	if bundle.DownloadPath != builtinSkillDownloadPath || !strings.Contains(bundle.InstallHint, builtinSkillDownloadPath) {
		t.Fatalf("skill is missing agent installation metadata: %+v", bundle)
	}
	digest := sha256.Sum256(archive)
	if bundle.SHA256 != hex.EncodeToString(digest[:]) {
		t.Fatal("skill checksum mismatch")
	}
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil || len(zr.File) != len(bundle.Files) {
		t.Fatalf("invalid archive: %v", err)
	}

	w := httptest.NewRecorder()
	(&Server{}).handleDownloadSkill(w, httptest.NewRequest(http.MethodGet, "/api/skills/portway/download", nil))
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("download: %d %s", w.Code, w.Body.String())
	}
	downloaded, _ := io.ReadAll(w.Result().Body)
	if !bytes.Equal(downloaded, archive) {
		t.Fatal("download differs from MCP skill package")
	}
}
