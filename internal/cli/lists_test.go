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
	"sync/atomic"
	"testing"

	"github.com/cosgroveb/al/internal/anylist"
	pb "github.com/cosgroveb/al/internal/anylistpb"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

func listReadServer(t *testing.T, lists []anylist.List) *httptest.Server {
	t.Helper()
	root := &pb.PBListFolder{Identifier: proto.String("root")}
	account := &pb.PBUserDataResponse{
		ShoppingListsResponse: &pb.ShoppingListsResponse{},
		ListFoldersResponse: &pb.PBListFoldersResponse{
			RootFolderId: proto.String("root"), ListFolders: []*pb.PBListFolder{root},
		},
	}
	for _, list := range lists {
		account.ShoppingListsResponse.NewLists = append(account.ShoppingListsResponse.NewLists,
			&pb.ShoppingList{Identifier: proto.String(list.ID), Name: proto.String(list.Name)})
		root.Items = append(root.Items, &pb.PBListFolderItem{Identifier: proto.String(list.ID), ItemType: proto.Int32(0)})
	}
	data, err := proto.Marshal(account)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/token":
			_, _ = io.WriteString(w, `{"access_token":"fixture-token","user_id":"fixture-user"}`)
		case "/data/user-data/get":
			_, _ = w.Write(data)
		default:
			t.Errorf("read-only command attempted %s", r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func runListCommand(t *testing.T, server *httptest.Server, configPath string, args ...string) (int, shellOutput) {
	t.Helper()
	var out, errOut bytes.Buffer
	a := newApp(nil, &out, &errOut, options{
		configPath: configPath,
		anylist:    anylist.Options{BaseURL: server.URL, HTTPClient: server.Client()},
		getenv: func(name string) string {
			if name == "ANYLIST_EMAIL" {
				return "fixture@example.test"
			}
			if name == "ANYLIST_PASSWORD" {
				return "fixture-password"
			}
			return ""
		},
	})
	code := a.run(context.Background(), append(args, "--json"))
	if errOut.Len() != 0 {
		t.Fatalf("unexpected stderr: %q", errOut.String())
	}
	return code, decodeShellOutput(t, &out)
}

func TestListReadCollectionsAndConfigPreservation(t *testing.T) {
	for _, lists := range [][]anylist.List{{}, {{ID: "one", Name: "Groceries"}}, {{ID: "one", Name: "Groceries"}, {ID: "two", Name: "Errands"}}} {
		server := listReadServer(t, lists)
		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, []byte(`{"clientId":"stable-client","defaultListId":"saved-default"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		code, result := runListCommand(t, server, path, "lists")
		if code != 0 || !result.OK {
			t.Fatalf("result: code=%d %+v", code, result)
		}
		var data struct {
			Lists []anylist.List `json:"lists"`
		}
		if err := json.Unmarshal(result.Data, &data); err != nil {
			t.Fatal(err)
		}
		if data.Lists == nil || len(data.Lists) != len(lists) {
			t.Fatalf("lists = %+v, want %+v", data.Lists, lists)
		}
		fresh := newApp(nil, nil, nil, options{configPath: path})
		if err := fresh.loadConfig(); err != nil {
			t.Fatal(err)
		}
		if fresh.config != (Config{ClientID: "stable-client", DefaultListID: "saved-default"}) {
			t.Fatalf("read changed config: %+v", fresh.config)
		}
	}
}

func TestDefaultListValidationSavesID(t *testing.T) {
	server := listReadServer(t, []anylist.List{{ID: "list-one", Name: "Groceries"}})
	path := filepath.Join(t.TempDir(), "config.json")
	code, result := runListCommand(t, server, path, "config", "set", "default-list", "Groceries")
	if code != 0 || !result.OK {
		t.Fatalf("result: code=%d %+v", code, result)
	}
	fresh := newApp(nil, nil, nil, options{configPath: path})
	if err := fresh.loadConfig(); err != nil {
		t.Fatal(err)
	}
	if fresh.config.DefaultListID != "list-one" || fresh.config.ClientID == "" {
		t.Fatalf("saved config = %+v", fresh.config)
	}
	code, result = runListCommand(t, server, path, "config", "set", "default-list", "--list-id", "missing")
	if code != 1 || result.Error == nil || result.Error.Code != "not_found" {
		t.Fatalf("missing default: code=%d %+v", code, result)
	}
	unchanged := newApp(nil, nil, nil, options{configPath: path})
	if err := unchanged.loadConfig(); err != nil {
		t.Fatal(err)
	}
	if unchanged.config != fresh.config {
		t.Fatalf("invalid selection changed config: %+v", unchanged.config)
	}
}

func TestListLifecycleAmbiguityUsesSupportedIDFlag(t *testing.T) {
	server := listReadServer(t, []anylist.List{{ID: "one", Name: "Groceries"}, {ID: "two", Name: "Groceries"}})
	for _, args := range [][]string{
		{"list", "rename", "Groceries", "Shopping"},
		{"list", "delete", "Groceries"},
		{"list", "share", "Groceries", "person@example.test"},
	} {
		code, result := runListCommand(t, server, filepath.Join(t.TempDir(), "config.json"), args...)
		if code != 1 || result.Error == nil || result.Error.Code != "ambiguous" || len(result.Error.Candidates) != 2 {
			t.Fatalf("result: code=%d %+v", code, result)
		}
		if !strings.Contains(result.Error.Message, "--id one") || strings.Contains(result.Error.Message, "--list-id") {
			t.Fatalf("unsupported selector in %q", result.Error.Message)
		}
	}
}

func TestListIDFlagWorksBeforeAndAfterPositionals(t *testing.T) {
	server := listReadServer(t, []anylist.List{{ID: "one", Name: "Groceries"}})
	for _, args := range [][]string{
		{"list", "rename", "--id", "one", "Groceries"},
		{"list", "rename", "Groceries", "--id", "one"},
	} {
		code, result := runListCommand(t, server, filepath.Join(t.TempDir(), "config.json"), args...)
		if code != 0 || !result.OK {
			t.Fatalf("result: code=%d %+v", code, result)
		}
		var data struct {
			Results []anylist.MutationResult `json:"results"`
		}
		if err := json.Unmarshal(result.Data, &data); err != nil {
			t.Fatal(err)
		}
		if len(data.Results) != 1 || data.Results[0].ID != "one" || data.Results[0].Outcome != "unchanged" {
			t.Fatalf("results = %+v", data.Results)
		}
	}
}

func TestListDeleteResidualSettingsByID(t *testing.T) {
	account := &pb.PBUserDataResponse{
		ShoppingListsResponse: &pb.ShoppingListsResponse{},
		ListFoldersResponse: &pb.PBListFoldersResponse{
			RootFolderId: proto.String("root"),
			ListFolders:  []*pb.PBListFolder{{Identifier: proto.String("root")}},
		},
		ListSettingsResponse: &pb.PBListSettingsList{Settings: []*pb.PBListSettings{{
			Identifier: proto.String("settings"), UserId: proto.String("fixture-user"), ListId: proto.String("residual-list"),
		}}},
	}
	before, err := proto.Marshal(account)
	if err != nil {
		t.Fatal(err)
	}
	account.ListSettingsResponse.Settings = nil
	after, err := proto.Marshal(account)
	if err != nil {
		t.Fatal(err)
	}
	var reads, updates atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/token":
			_, _ = io.WriteString(w, `{"access_token":"fixture-token","user_id":"fixture-user"}`)
		case "/data/user-data/get":
			reads.Add(1)
			data := before
			if updates.Load() > 0 {
				data = after
			}
			_, _ = w.Write(data)
		case "/data/list-settings/update":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			defer func() { _ = r.MultipartForm.RemoveAll() }()
			var operations pb.PBListSettingsOperationList
			if err := proto.Unmarshal([]byte(r.FormValue("operations")), &operations); err != nil || len(operations.Operations) != 1 {
				t.Errorf("invalid settings operation: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			op := operations.Operations[0]
			if op.GetMetadata().GetHandlerId() != "remove-list-settings" || op.GetUpdatedSettings().GetListId() != "residual-list" || op.GetUpdatedSettings().GetIdentifier() != "settings" {
				t.Errorf("unexpected recovery operation: %v", op)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			updates.Add(1)
			_, _ = w.Write(protowire.AppendString(protowire.AppendTag(nil, 3, protowire.BytesType), op.GetMetadata().GetOperationId()))
		default:
			t.Errorf("recovery attempted unexpected endpoint %s", r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(server.Close)
	code, result := runListCommand(t, server, filepath.Join(t.TempDir(), "config.json"), "list", "delete", "--id", "residual-list")
	if code != 0 || !result.OK {
		t.Fatalf("explicit-ID recovery: exit %d, error %+v", code, result.Error)
	}
	var data struct {
		Results []anylist.MutationResult `json:"results"`
	}
	if err := json.Unmarshal(result.Data, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Results) != 1 || data.Results[0].ID != "residual-list" || data.Results[0].Outcome != "removed" || updates.Load() != 1 || reads.Load() != 2 {
		t.Fatalf("recovery results %+v, settings updates %d, account reads %d", data.Results, updates.Load(), reads.Load())
	}
}
