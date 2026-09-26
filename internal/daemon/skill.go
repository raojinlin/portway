package daemon

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	builtinSkillName         = "portway"
	builtinSkillVersion      = "1.1.0"
	builtinSkillDownloadPath = "/skills/portway.zip"
)

//go:embed builtin/portway
var builtinSkillFS embed.FS

type skillFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type skillBundle struct {
	Name         string      `json:"name"`
	Version      string      `json:"version"`
	SHA256       string      `json:"sha256"`
	Files        []skillFile `json:"files"`
	InstallHint  string      `json:"install_hint"`
	DownloadPath string      `json:"download_path"`
}

func loadBuiltinSkill() (skillBundle, []byte, error) {
	entries := []skillFile{}
	err := fs.WalkDir(builtinSkillFS, "builtin/portway", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		data, err := builtinSkillFS.ReadFile(path)
		if err != nil {
			return err
		}
		relative := strings.TrimPrefix(path, "builtin/portway/")
		entries = append(entries, skillFile{Path: relative, Content: string(data)})
		return nil
	})
	if err != nil {
		return skillBundle{}, nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })

	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	fixedTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: builtinSkillName + "/" + entry.Path, Method: zip.Deflate}
		header.SetModTime(fixedTime)
		header.SetMode(0o600)
		writer, err := zw.CreateHeader(header)
		if err != nil {
			return skillBundle{}, nil, err
		}
		if _, err := writer.Write([]byte(entry.Content)); err != nil {
			return skillBundle{}, nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return skillBundle{}, nil, err
	}
	digest := sha256.Sum256(archive.Bytes())
	bundle := skillBundle{
		Name:         builtinSkillName,
		Version:      builtinSkillVersion,
		SHA256:       hex.EncodeToString(digest[:]),
		Files:        entries,
		InstallHint:  fmt.Sprintf("Download the ZIP from the MCP server origin at %s, extract the %s directory into the agent's skills directory, and validate SKILL.md after installation.", builtinSkillDownloadPath, builtinSkillName),
		DownloadPath: builtinSkillDownloadPath,
	}
	return bundle, archive.Bytes(), nil
}

// BuiltinPortwaySkillArchive returns the same ZIP served by the web API and MCP tool.
func BuiltinPortwaySkillArchive() ([]byte, error) {
	_, archive, err := loadBuiltinSkill()
	return archive, err
}

func (s *Server) handleDownloadSkill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	archive, err := BuiltinPortwaySkillArchive()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "package skill: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="portway-skill.zip"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(archive)
}
