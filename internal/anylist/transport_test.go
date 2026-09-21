package anylist

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/cosgroveb/al/internal/anylistpb"
	"google.golang.org/protobuf/proto"
)

func TestAuthenticationFailuresAreSafeAndNotRetried(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body, code string
	}{
		{"rejected credentials", 401, "secret authentication details", "authentication"},
		{"permission", 403, "secret permission details", "permission"},
		{"server failure", 500, "secret server details", "operation_failed"},
		{"unexpected success", 201, `{"access_token":"test-token","user_id":"test-user"}`, "operation_failed"},
		{"empty response", 200, "", "protocol"},
		{"malformed JSON", 200, "secret invalid body", "protocol"},
		{"missing user", 200, `{"access_token":"test-token"}`, "protocol"},
		{"missing token", 200, `{"user_id":"test-user"}`, "protocol"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			step := authStep()
			step.status = test.status
			step.response = []byte(test.body)
			client := scriptedClient(t, step)
			_, err := client.Lists(context.Background())
			_ = safeError(t, err, test.code, false)
		})
	}
}

func TestMalformedAccountReadsFailSafely(t *testing.T) {
	noShopping := fixtureAccount()
	noShopping.ShoppingListsResponse = nil
	noFolders := fixtureAccount()
	noFolders.ListFoldersResponse = nil
	noRoot := fixtureAccount()
	noRoot.ListFoldersResponse.RootFolderId = nil
	missingRoot := fixtureAccount()
	missingRoot.ListFoldersResponse.RootFolderId = proto.String("missing-root")
	cases := []struct {
		name string
		body []byte
	}{
		{"empty", nil},
		{"invalid wire", []byte{0xff}},
		{"missing required list identifier", []byte{0x0a, 0x02, 0x0a, 0x00}},
		{"no shopping response", marshal(t, noShopping)},
		{"no folders response", marshal(t, noFolders)},
		{"no root ID", marshal(t, noRoot)},
		{"unresolved root ID", marshal(t, missingRoot)},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			client := scriptedClient(t, authStep(), requestStep{path: "/data/user-data/get", response: test.body})
			_, err := client.Lists(context.Background())
			_ = safeError(t, err, "protocol", false)
		})
	}
}

func TestRejectedAccountReadsAreNotRetried(t *testing.T) {
	for _, test := range []struct {
		status int
		code   string
	}{{401, "authentication"}, {403, "permission"}, {500, "operation_failed"}} {
		t.Run(http.StatusText(test.status), func(t *testing.T) {
			client := scriptedClient(t, authStep(), requestStep{path: "/data/user-data/get", status: test.status, response: []byte("secret response body")})
			_, err := client.Lists(context.Background())
			got := safeError(t, err, test.code, false)
			if got.HTTPStatus != test.status {
				t.Fatalf("HTTP status%d want%d", got.HTTPStatus, test.status)
			}
		})
	}
}

func TestHTTPWriteCertaintyAndNoRetry(t *testing.T) {
	cases := []struct {
		status  int
		code    string
		unknown bool
	}{
		{201, "operation_failed", true}, {204, "operation_failed", true}, {302, "operation_failed", true}, {307, "operation_failed", true},
		{400, "operation_failed", false}, {401, "authentication", false}, {403, "permission", false}, {429, "operation_failed", false},
		{500, "operation_failed", true}, {503, "operation_failed", true},
	}
	for _, test := range cases {
		t.Run(http.StatusText(test.status), func(t *testing.T) {
			client := loadedClient(t, fixtureAccount(), requestStep{path: "/data/shopping-lists/update", status: test.status, response: []byte("secret response body")})
			name := "changed"
			result, err := client.ApplyItem(context.Background(), "list", ItemChange{ID: "item", Name: &name})
			got := safeError(t, err, test.code, test.unknown)
			if got.HTTPStatus != test.status || result.ID != "item" {
				t.Fatalf("error HTTP%d result%+v", got.HTTPStatus, result)
			}
			want := "failed"
			if test.unknown {
				want = "unknown"
			}
			if result.Outcome != want {
				t.Fatalf("outcome%s want%s", result.Outcome, want)
			}
		})
	}
}

func TestWriteRedirectIsNotFollowed(t *testing.T) {
	var redirected, updates atomic.Int32
	account := marshal(t, fixtureAccount())
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/token":
			_, _ = io.WriteString(w, `{"access_token":"test-token","user_id":"test-user"}`)
		case "/data/user-data/get":
			_, _ = w.Write(account)
		case "/data/shopping-lists/update":
			updates.Add(1)
			w.Header().Set("Location", server.URL+"/redirected")
			w.WriteHeader(307)
		case "/redirected":
			redirected.Add(1)
			w.WriteHeader(200)
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	client := NewClient(Options{Email: "test@example.invalid", Password: "test-password", ClientID: "test-client", BaseURL: server.URL, HTTPClient: server.Client()})
	if _, err := client.Lists(context.Background()); err != nil {
		t.Fatal(err)
	}
	name := "changed"
	_, err := client.ApplyItem(context.Background(), "list", ItemChange{ID: "item", Name: &name})
	_ = safeError(t, err, "operation_failed", true)
	if redirected.Load() != 0 || updates.Load() != 1 {
		t.Fatalf("updates%d redirected%d", updates.Load(), redirected.Load())
	}
}

func TestCanceledRequestBeforeSendMakesNoWrite(t *testing.T) {
	client := loadedClient(t, fixtureAccount())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	name := "changed"
	result, err := client.ApplyItem(ctx, "list", ItemChange{ID: "item", Name: &name})
	_ = safeError(t, err, "transport", false)
	if result.Outcome != "failed" || result.ID != "item" {
		t.Fatalf("result%+v", result)
	}
}

func TestCanceledAndTimedOutWritesHaveUnknownOutcome(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		name := "canceled"
		if timeout {
			name = "timed out"
		}
		t.Run(name, func(t *testing.T) {
			entered := make(chan struct{}, 1)
			release := make(chan struct{})
			var updates atomic.Int32
			account := marshal(t, fixtureAccount())
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/auth/token":
					_, _ = io.WriteString(w, `{"access_token":"test-token","user_id":"test-user"}`)
				case "/data/user-data/get":
					_, _ = w.Write(account)
				case "/data/shopping-lists/update":
					updates.Add(1)
					entered <- struct{}{}
					<-release
				default:
					t.Errorf("unexpected endpoint %s", r.URL.Path)
					w.WriteHeader(500)
				}
			}))
			defer server.Close()
			defer close(release)
			client := NewClient(Options{Email: "test@example.invalid", Password: "test-password", ClientID: "test-client", BaseURL: server.URL, HTTPClient: server.Client()})
			if _, err := client.Lists(context.Background()); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			if timeout {
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), 500*time.Millisecond)
			}
			defer cancel()
			type completed struct {
				result MutationResult
				err    error
			}
			done := make(chan completed, 1)
			go func() {
				name := "changed"
				result, err := client.ApplyItem(ctx, "list", ItemChange{ID: "item", Name: &name})
				done <- completed{result, err}
			}()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("mutation did not reach local server")
			}
			if !timeout {
				cancel()
			}
			var result completed
			select {
			case result = <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("request ignored context cancellation")
			}
			_ = safeError(t, result.err, "transport", true)
			if result.result.Outcome != "unknown" || result.result.ID != "item" || updates.Load() != 1 {
				t.Fatalf("result%+v writes%d", result.result, updates.Load())
			}
		})
	}
}

func TestMissingCredentialsDoNotContactService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request without credentials")
		w.WriteHeader(500)
	}))
	defer server.Close()
	client := NewClient(Options{BaseURL: server.URL, HTTPClient: server.Client(), ClientID: "test-client"})
	_, err := client.Lists(context.Background())
	_ = safeError(t, err, "configuration", false)
}

func TestEmptyAccountIsAnEmptyCollection(t *testing.T) {
	account := &pb.PBUserDataResponse{ShoppingListsResponse: &pb.ShoppingListsResponse{}, ListFoldersResponse: &pb.PBListFoldersResponse{ListDataId: proto.String("data"), RootFolderId: proto.String("root"), ListFolders: []*pb.PBListFolder{{Identifier: proto.String("root")}}}}
	client := scriptedClient(t, authStep(), readStep(t, account))
	lists, err := client.Lists(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if lists == nil || len(lists) != 0 {
		t.Fatalf("empty list read=%+v", lists)
	}
}
