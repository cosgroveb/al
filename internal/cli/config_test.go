package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigShowDoesNotCreateConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "al", "config.json")
	var out, errOut bytes.Buffer
	a := newApp(strings.NewReader(""), &out, &errOut, options{configPath: path, getenv: func(string) string { return "" }})
	if code := a.run(context.Background(), []string{"config", "show", "--json"}); code != 0 {
		t.Fatalf("exit = %d, output = %s", code, out.String())
	}
	got := decodeShellOutput(t, &out)
	var data struct {
		Config Config `json:"config"`
		Path   string `json:"path"`
	}
	if err := json.Unmarshal(got.Data, &data); err != nil {
		t.Fatal(err)
	}
	if data.Config != (Config{}) || data.Path != path {
		t.Fatalf("config = %+v, path = %q", data.Config, data.Path)
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatalf("show created a configuration directory: %v", err)
	}
}

func TestConfigSavePreservesDefaultAndUsesAtomicReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"clientId":"old-client","defaultListId":"saved-list"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	original, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = original.Close() }()
	a := newApp(nil, nil, nil, options{configPath: path})
	if err := a.loadConfig(); err != nil {
		t.Fatal(err)
	}
	a.config.ClientID = "new-client"
	if err := a.saveConfig(); err != nil {
		t.Fatal(err)
	}
	fresh := newApp(nil, nil, nil, options{configPath: path})
	if err := fresh.loadConfig(); err != nil {
		t.Fatal(err)
	}
	if fresh.config != (Config{ClientID: "new-client", DefaultListID: "saved-list"}) {
		t.Fatalf("saved config = %+v", fresh.config)
	}
	var previous Config
	if err := json.NewDecoder(original).Decode(&previous); err != nil {
		t.Fatal(err)
	}
	if previous.ClientID != "old-client" {
		t.Fatal("save modified the existing file instead of replacing it")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config permissions = %o", info.Mode().Perm())
	}
	files, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".config-*"))
	if err != nil || len(files) != 0 {
		t.Fatalf("temporary config files = %v, error = %v", files, err)
	}
}

func TestMalformedConfigHasSafeStructuredError(t *testing.T) {
	for _, content := range []string{"null", "{} {}", `{"password":"fixture-secret"}`, `{"clientId":false}`} {
		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		var out, errOut bytes.Buffer
		a := newApp(nil, &out, &errOut, options{configPath: path, getenv: func(string) string { return "" }})
		if code := a.run(context.Background(), []string{"config", "show", "--json"}); code != 1 {
			t.Fatalf("config %q: exit = %d", content, code)
		}
		if strings.Contains(out.String(), "fixture-secret") || errOut.Len() != 0 {
			t.Fatalf("unsafe output: %s, stderr: %q", out.String(), errOut.String())
		}
		got := decodeShellOutput(t, &out)
		if got.Error == nil || got.Error.Code != "configuration" {
			t.Fatalf("error = %+v", got.Error)
		}
	}
}
