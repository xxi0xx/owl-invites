package buildinfo

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeDefaults(t *testing.T) {
	got := Normalize(Info{})
	if got.Version != "dev" || got.Commit != "unknown" || got.BuildState != "unknown" {
		t.Fatalf("unexpected defaults: %+v", got)
	}
}

func TestWriteTextAndJSON(t *testing.T) {
	info := Info{Version: "v3.2.1", Commit: "abc123", BuildState: "clean"}

	var text bytes.Buffer
	if err := Write(&text, info, false); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Owl Invites v3.2.1", "commit: abc123", "build: clean"} {
		if !strings.Contains(text.String(), want) {
			t.Fatalf("text output %q does not contain %q", text.String(), want)
		}
	}

	var encoded bytes.Buffer
	if err := Write(&encoded, info, true); err != nil {
		t.Fatal(err)
	}
	var decoded Info
	if err := json.Unmarshal(encoded.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded != info {
		t.Fatalf("JSON round trip: got %+v, want %+v", decoded, info)
	}
}

func TestRunVersionCommand(t *testing.T) {
	var output bytes.Buffer
	handled, err := RunVersionCommand([]string{"version", "--json"}, &output)
	if err != nil || !handled {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	if !json.Valid(output.Bytes()) {
		t.Fatalf("invalid JSON: %q", output.String())
	}

	handled, err = RunVersionCommand([]string{"admin"}, &output)
	if err != nil || handled {
		t.Fatalf("non-version command handled=%v err=%v", handled, err)
	}

	handled, err = RunVersionCommand([]string{"version", "--yaml"}, &output)
	if err == nil || !handled {
		t.Fatalf("invalid option handled=%v err=%v", handled, err)
	}
}
