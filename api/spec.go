// Package apidoc embeds the reviewable Owl Invites OpenAPI contract.
package apidoc

import (
	"embed"
	"net/http"
)

//go:embed openapi.json
var files embed.FS

func ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	spec, err := files.ReadFile("openapi.json")
	if err != nil {
		http.Error(w, "API contract unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.oai.openapi+json;version=3.1")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(spec)
}
