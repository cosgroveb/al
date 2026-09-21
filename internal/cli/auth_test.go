package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/cosgroveb/al/internal/anylist"
)

func TestAuthStatusVerifiesEveryInvocation(t *testing.T) {
	for _, test := range []struct {
		name   string
		config Config
	}{
		{"fresh config", Config{}},
		{"saved config", Config{ClientID: "saved-client", DefaultListID: "saved-list"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if test.config.ClientID != "" {
				data, err := json.Marshal(test.config)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			var mu sync.Mutex
			var requests int
			var clientID string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				requests++
				clientID = r.Header.Get("X-AnyLeaf-Client-Identifier")
				if r.Method != http.MethodPost || r.URL.Path != "/auth/token" || r.Header.Get("Authorization") != "" || clientID == "" {
					t.Error("expected one token request with a client ID and no cached authorization")
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if err := r.ParseMultipartForm(1 << 20); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				defer func() { _ = r.MultipartForm.RemoveAll() }()
				if r.FormValue("email") != "fixture@example.test" || r.FormValue("password") != "fixture-password" {
					t.Error("token request did not use the supplied credentials")
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				_, _ = io.WriteString(w, `{"access_token":"fixture-token","user_id":"fixture-user"}`)
			}))
			defer server.Close()
			previousClientID := test.config.ClientID
			for i, jsonOutput := range []bool{false, true} {
				var out, errOut bytes.Buffer
				a := newApp(nil, &out, &errOut, options{
					configPath: path,
					anylist:    anylist.Options{BaseURL: server.URL, HTTPClient: server.Client()},
					getenv: func(name string) string {
						return map[string]string{"ANYLIST_EMAIL": "fixture@example.test", "ANYLIST_PASSWORD": "fixture-password"}[name]
					},
				})
				args := []string{"auth", "status"}
				if jsonOutput {
					args = append(args, "--json")
				}
				if code := a.run(context.Background(), args); code != 0 || errOut.Len() != 0 {
					t.Fatalf("auth status: exit %d, stdout %q, stderr %q", code, out.String(), errOut.String())
				}
				if jsonOutput {
					got := decodeShellOutput(t, &out)
					if !got.OK || got.Error != nil || string(got.Data) != `{"authenticated":true}` {
						t.Fatalf("auth status JSON: %+v, data %s", got, got.Data)
					}
				} else if out.String() != "Authenticated with AnyList.\n" {
					t.Fatalf("auth status output: %q", out.String())
				}
				mu.Lock()
				gotRequests, gotClientID := requests, clientID
				mu.Unlock()
				if gotRequests != i+1 {
					t.Fatalf("request count %d, want %d", gotRequests, i+1)
				}
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var saved Config
				if err := json.Unmarshal(data, &saved); err != nil {
					t.Fatal(err)
				}
				if saved.ClientID == "" || saved.ClientID != gotClientID || previousClientID != "" && saved.ClientID != previousClientID || saved.DefaultListID != test.config.DefaultListID {
					t.Fatalf("auth status changed configuration: %+v, request client %q", saved, gotClientID)
				}
				previousClientID = saved.ClientID
				for _, secret := range []string{"fixture@example.test", "fixture-password", "fixture-token"} {
					if bytes.Contains(data, []byte(secret)) {
						t.Fatal("auth status persisted credentials or a token")
					}
				}
			}
		})
	}
}

func TestAuthStatusFailures(t *testing.T) {
	for _, test := range []struct {
		name, missing, body, code, message string
		status                             int
		canceled                           bool
	}{
		{"missing email", "ANYLIST_EMAIL", "", "configuration", "set ANYLIST_EMAIL and ANYLIST_PASSWORD for commands that use AnyList", 0, false},
		{"missing password", "ANYLIST_PASSWORD", "", "configuration", "set ANYLIST_EMAIL and ANYLIST_PASSWORD for commands that use AnyList", 0, false},
		{"rejected credentials", "", "fixture-private-body fixture-password fixture-token", "authentication", "AnyList rejected authentication", http.StatusUnauthorized, false},
		{"malformed response", "", "fixture-private-body fixture-password fixture-token", "protocol", "AnyList returned an invalid authentication response", http.StatusOK, false},
		{"missing token", "", `{"user_id":"fixture-user"}`, "protocol", "AnyList returned an invalid authentication response", http.StatusOK, false},
		{"missing user", "", `{"access_token":"fixture-token"}`, "protocol", "AnyList returned an invalid authentication response", http.StatusOK, false},
		{"canceled request", "", "", "transport", "AnyList request was canceled before sending", 0, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, jsonOutput := range []bool{false, true} {
				var requests atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests.Add(1)
					if test.status == 0 || r.Method != http.MethodPost || r.URL.Path != "/auth/token" {
						t.Error("unexpected request from auth status")
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					w.WriteHeader(test.status)
					_, _ = io.WriteString(w, test.body)
				}))
				t.Cleanup(server.Close)
				var out, errOut bytes.Buffer
				a := newApp(nil, &out, &errOut, options{
					configPath: filepath.Join(t.TempDir(), "config.json"),
					anylist:    anylist.Options{BaseURL: server.URL, HTTPClient: server.Client()},
					getenv: func(name string) string {
						if name == test.missing {
							return ""
						}
						return map[string]string{"ANYLIST_EMAIL": "fixture@example.test", "ANYLIST_PASSWORD": "fixture-password"}[name]
					},
				})
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if test.canceled {
					cancel()
				}
				args := []string{"auth", "status"}
				if jsonOutput {
					args = append(args, "--json")
				}
				if code := a.run(ctx, args); code != 1 {
					t.Fatalf("auth status: exit %d, stdout %q, stderr %q", code, out.String(), errOut.String())
				}
				wantRequests := int32(1)
				if test.missing != "" || test.canceled {
					wantRequests = 0
				}
				if got := requests.Load(); got != wantRequests {
					t.Fatalf("request count %d, want %d", got, wantRequests)
				}
				for _, secret := range []string{"fixture@example.test", "fixture-password", "fixture-token", "fixture-private-body"} {
					if strings.Contains(out.String()+errOut.String(), secret) {
						t.Fatal("auth status output leaked credentials, token, or response body")
					}
				}
				if jsonOutput {
					got := decodeShellOutput(t, &out)
					if got.OK || string(got.Data) != "null" || got.Error == nil || got.Error.Code != test.code || got.Error.Message != test.message || errOut.Len() != 0 {
						t.Fatalf("auth status JSON: %+v, data %s, stderr %q", got, got.Data, errOut.String())
					}
				} else if out.Len() != 0 || errOut.String() != "error: "+test.message+"\n" {
					t.Fatalf("auth status stdout %q, stderr %q", out.String(), errOut.String())
				}
			}
		})
	}
}

func TestRootHelpExplainsAuthentication(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		var out, errOut bytes.Buffer
		a := newApp(nil, &out, &errOut, options{getenv: func(string) string { return "" }})
		args := []string{"--help"}
		if jsonOutput {
			args = append(args, "--json")
		}
		if code := a.run(context.Background(), args); code != 0 || errOut.Len() != 0 {
			t.Fatalf("help: exit %d, stderr %q", code, errOut.String())
		}
		help := out.String()
		if jsonOutput {
			got := decodeShellOutput(t, &out)
			var data struct {
				Text string `json:"text"`
			}
			if !got.OK || got.Error != nil {
				t.Fatalf("help JSON: %+v", got)
			}
			if err := json.Unmarshal(got.Data, &data); err != nil {
				t.Fatal(err)
			}
			help = data.Text
		}
		for _, want := range []string{"export ANYLIST_EMAIL", "export ANYLIST_PASSWORD", "al auth status", "Passwords and tokens stay in process memory."} {
			if !strings.Contains(help, want) {
				t.Fatalf("help lacks %q: %s", want, help)
			}
		}
	}
}
