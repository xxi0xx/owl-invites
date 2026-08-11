// Package backup implements verified operator backups for Owl Invites state.
package backup

import "time"

const (
	FormatVersion      = 1
	databaseFilename   = "database.sqlite"
	uploadsDirectory   = "uploads"
	manifestFilename   = "manifest.json"
	uploadsConsistency = "filesystem snapshot after database; not transactionally atomic"
)

// Manifest is the versioned, non-secret description of a backup bundle.
type Manifest struct {
	FormatVersion     int             `json:"formatVersion"`
	CreatedAt         time.Time       `json:"createdAt"`
	Source            SourceMetadata  `json:"source"`
	Database          FileArtifact    `json:"database"`
	SecretFingerprint string          `json:"secretFingerprint"`
	Uploads           UploadsSnapshot `json:"uploads"`
}

type SourceMetadata struct {
	DatabaseType       string `json:"databaseType"`
	SchemaVersion      uint   `json:"schemaVersion"`
	SchemaDirty        bool   `json:"schemaDirty"`
	ApplicationVersion string `json:"applicationVersion"`
	Commit             string `json:"commit"`
	BuildState         string `json:"buildState"`
}

type FileArtifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type UploadsSnapshot struct {
	Included    bool           `json:"included"`
	Consistency string         `json:"consistency"`
	FileCount   int            `json:"fileCount"`
	TotalBytes  int64          `json:"totalBytes"`
	Files       []FileArtifact `json:"files"`
}
