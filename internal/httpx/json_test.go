package httpx

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeJSONRejectsUnknownFieldsAndTrailingValues(t *testing.T) {
	tests := []string{
		`{"name":"ok","extra":true}`,
		`{"name":"ok"} {"name":"second"}`,
	}
	for _, body := range tests {
		req := httptest.NewRequest("POST", "/", strings.NewReader(body))
		var dst struct {
			Name string `json:"name"`
		}
		require.Error(t, DecodeJSON(req, &dst))
	}
}

func TestDecodeJSONAcceptsSingleKnownObject(t *testing.T) {
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"Owl Invites"}`))
	var dst struct {
		Name string `json:"name"`
	}
	require.NoError(t, DecodeJSON(req, &dst))
	require.Equal(t, "Owl Invites", dst.Name)
}
