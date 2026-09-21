package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cosgroveb/al/internal/anylist"
)

func TestLocalCommandsDoNotAuthenticate(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		stdin string
		code  int
		json  bool
	}{
		{"help", []string{"--help"}, "", 0, false},
		{"command help", []string{"list", "rename", "--help"}, "", 0, false},
		{"auth", []string{"auth"}, "", 0, false},
		{"auth help", []string{"auth", "--help"}, "", 0, false},
		{"auth status help", []string{"auth", "status", "--help"}, "", 0, false},
		{"json auth", []string{"--json", "auth"}, "", 0, true},
		{"json auth help", []string{"auth", "--help", "--json"}, "", 0, true},
		{"json auth status help", []string{"auth", "status", "--json", "--help"}, "", 0, true},
		{"version", []string{"--version"}, "", 0, false},
		{"json help", []string{"--json", "--help"}, "", 0, true},
		{"config", []string{"config", "show", "--json"}, "", 0, true},
		{"unknown command", []string{"unknown", "--json"}, "", 2, true},
		{"unknown list command", []string{"list", "unknown", "--json"}, "", 2, true},
		{"unknown config command", []string{"config", "unknown", "--json"}, "", 2, true},
		{"unknown auth command", []string{"auth", "unknown", "--json"}, "", 2, true},
		{"extra auth status argument", []string{"auth", "status", "extra", "--json"}, "", 2, true},
		{"unknown auth status flag", []string{"auth", "status", "--bogus", "--json"}, "", 2, true},
		{"unknown flag", []string{"lists", "--bogus", "--json"}, "", 2, true},
		{"flag from another command", []string{"lists", "--notes", "--json"}, "", 2, true},
		{"invalid json flag", []string{"lists", "--json=invalid"}, "", 2, true},
		{"extra list argument", []string{"lists", "extra", "--json"}, "", 2, true},
		{"empty create", []string{"list", "create", "", "--json"}, "", 2, true},
		{"conflicting rename", []string{"list", "rename", "old", "new", "--id", "id", "--json"}, "", 2, true},
		{"empty delete ID", []string{"list", "delete", "--id=", "--json"}, "", 2, true},
		{"invalid recipient", []string{"list", "share", "Groceries", "invalid", "--json"}, "", 2, true},
		{"empty config ID", []string{"config", "set", "default-list", "--list-id=", "--json"}, "", 2, true},
		{"empty batch", []string{"add", "--stdin", "--json"}, "[]", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Error("local command contacted a network endpoint")
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()
			var out, errOut bytes.Buffer
			a := newApp(strings.NewReader(tt.stdin), &out, &errOut, options{
				configPath: filepath.Join(t.TempDir(), "config.json"),
				anylist:    anylist.Options{BaseURL: server.URL, HTTPClient: server.Client()},
				getenv: func(name string) string {
					if name == "ANYLIST_EMAIL" || name == "ANYLIST_PASSWORD" {
						t.Error("local command read credentials")
					}
					return ""
				},
			})
			if code := a.run(context.Background(), tt.args); code != tt.code {
				t.Fatalf("exit = %d, want %d; stdout=%q stderr=%q", code, tt.code, out.String(), errOut.String())
			}
			if tt.json {
				got := decodeShellOutput(t, &out)
				if got.OK != (tt.code == 0) {
					t.Fatalf("ok = %v, exit = %d", got.OK, tt.code)
				}
				if tt.code != 0 && (got.Error == nil || got.Error.Code != "input") {
					t.Fatalf("error = %+v", got.Error)
				}
				if errOut.Len() != 0 {
					t.Fatalf("JSON diagnostics on stderr: %q", errOut.String())
				}
			} else if out.Len() == 0 {
				t.Fatal("successful local command produced no output")
			}
		})
	}
}

func TestJSONFlagRespectsValuesAndPositionalEscaping(t *testing.T) {
	tests := []struct {
		args []string
		json bool
	}{
		{[]string{"--json", "lists"}, true},
		{[]string{"lists", "--json"}, true},
		{[]string{"list", "create", "--", "--json"}, false},
		{[]string{"add", "--notes", "--json", "milk"}, false},
		{[]string{"add", "--notes=--json", "milk", "--json"}, true},
		{[]string{"lists", "--json", "--json=false"}, false},
	}
	for _, tt := range tests {
		var out, errOut bytes.Buffer
		a := newApp(strings.NewReader(""), &out, &errOut, options{
			configPath: filepath.Join(t.TempDir(), "config.json"),
			getenv:     func(string) string { return "" },
		})
		if code := a.run(context.Background(), tt.args); code != 1 {
			t.Fatalf("args %q: exit = %d, want 1; stdout=%q stderr=%q", tt.args, code, out.String(), errOut.String())
		}
		if tt.json {
			got := decodeShellOutput(t, &out)
			if got.Error == nil || got.Error.Code != "configuration" || errOut.Len() != 0 {
				t.Fatalf("args %q: error=%+v stderr=%q", tt.args, got.Error, errOut.String())
			}
		} else if out.Len() != 0 || errOut.Len() == 0 {
			t.Fatalf("args %q: stdout=%q stderr=%q", tt.args, out.String(), errOut.String())
		}
	}
}

type shellOutput struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *anylist.Error  `json:"error"`
}

func decodeShellOutput(t *testing.T, in io.Reader) shellOutput {
	t.Helper()
	decoder := json.NewDecoder(in)
	var got shellOutput
	if err := decoder.Decode(&got); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		t.Fatalf("output must contain one JSON document, trailing decode: %v", err)
	}
	return got
}
