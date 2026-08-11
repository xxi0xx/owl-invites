// Package buildinfo exposes the source identity embedded in shipped binaries.
package buildinfo

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// These values are replaced by scripts/build-go.sh at build time.
var (
	Version    = "dev"
	Commit     = "unknown"
	BuildState = "unknown"
)

// Info is the stable, non-secret build identity contract.
type Info struct {
	Version    string `json:"version"`
	Commit     string `json:"commit"`
	BuildState string `json:"buildState"`
}

// Current returns a complete identity even for an unversioned developer build.
func Current() Info {
	return Normalize(Info{Version: Version, Commit: Commit, BuildState: BuildState})
}

// Normalize makes partially injected build information explicit.
func Normalize(info Info) Info {
	if info.Version == "" {
		info.Version = "dev"
	}
	if info.Commit == "" {
		info.Commit = "unknown"
	}
	if info.BuildState == "" {
		info.BuildState = "unknown"
	}
	return info
}

// Write renders build identity for an operator or automation.
func Write(w io.Writer, info Info, asJSON bool) error {
	info = Normalize(info)
	if asJSON {
		return json.NewEncoder(w).Encode(info)
	}
	_, err := fmt.Fprintf(w, "Owl Invites %s\ncommit: %s\nbuild: %s\n", info.Version, info.Commit, info.BuildState)
	return err
}

// RunVersionCommand handles the shared `version [--json]` binary contract.
func RunVersionCommand(args []string, w io.Writer) (bool, error) {
	if len(args) == 0 || args[0] != "version" {
		return false, nil
	}
	if len(args) > 2 || (len(args) == 2 && args[1] != "--json") {
		return true, errors.New("usage: version [--json]")
	}
	return true, Write(w, Current(), len(args) == 2)
}
