package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/cosgroveb/al/internal/anylist"
	pb "github.com/cosgroveb/al/internal/anylistpb"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

type pendingInput struct {
	*io.PipeReader
	started chan struct{}
	once    sync.Once
}

func (r *pendingInput) Read(data []byte) (int, error) {
	r.once.Do(func() { close(r.started) })
	return r.PipeReader.Read(data)
}

func TestStdinCancellationClosesPendingReader(t *testing.T) {
	reader, writer := io.Pipe()
	defer func() { _ = reader.Close() }()
	defer func() { _ = writer.Close() }()
	input := &pendingInput{PipeReader: reader, started: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var stdout, stderr bytes.Buffer
	a := newApp(input, &stdout, &stderr, options{
		configPath: filepath.Join(t.TempDir(), "config.json"),
		getenv:     func(string) string { return "" },
	})
	done := make(chan int, 1)
	go func() { done <- a.run(ctx, []string{"add", "--stdin", "--json"}) }()
	select {
	case <-input.started:
	case <-time.After(2 * time.Second):
		t.Fatal("command did not start reading stdin")
	}
	cancel()
	var code int
	select {
	case code = <-done:
	case <-time.After(2 * time.Second):
		_ = reader.Close()
		<-done
		t.Fatal("canceled command left stdin reader blocked")
	}
	var result commandEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if code != 1 || result.Error == nil || result.Error.Code != "canceled" || stderr.Len() != 0 {
		t.Fatalf("cancellation: code %d, error %v, stderr %q", code, result.Error, stderr.String())
	}
}

type commandServer struct {
	server      *httptest.Server
	mu          sync.Mutex
	auth, reads int
	operations  []*pb.PBListOperation
	acknowledge func(*pb.PBListOperation, int) []byte
}

func newCommandServer(t *testing.T, items []*pb.ListItem, failAt int) *commandServer {
	t.Helper()
	state := &commandServer{}
	account := &pb.PBUserDataResponse{
		ShoppingListsResponse: &pb.ShoppingListsResponse{
			NewLists: []*pb.ShoppingList{{Identifier: proto.String("list"), Name: proto.String("Groceries"), Items: items}},
			ListResponses: []*pb.PBListResponse{{ListId: proto.String("list"), CategoryGroupResponses: []*pb.PBListCategoryGroupResponse{{CategoryGroup: &pb.PBListCategoryGroup{
				Identifier: proto.String("group"), ListId: proto.String("list"), Categories: []*pb.PBListCategory{{Identifier: proto.String("dairy"), Name: proto.String("Dairy"), SystemCategory: proto.String("dairy")}},
			}}}}},
		},
		ListFoldersResponse:  &pb.PBListFoldersResponse{ListDataId: proto.String("list-data"), RootFolderId: proto.String("root"), ListFolders: []*pb.PBListFolder{{Identifier: proto.String("root"), Items: []*pb.PBListFolderItem{{Identifier: proto.String("list"), ItemType: proto.Int32(0)}}}}},
		ListSettingsResponse: &pb.PBListSettingsList{Settings: []*pb.PBListSettings{{Identifier: proto.String("settings"), UserId: proto.String("test-user"), ListId: proto.String("list"), ListCategoryGroupId: proto.String("group")}}},
	}
	data, err := proto.Marshal(account)
	if err != nil {
		t.Fatal(err)
	}
	state.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		defer state.mu.Unlock()
		switch r.URL.Path {
		case "/auth/token":
			state.auth++
			_, _ = io.WriteString(w, `{"access_token":"fixture-token","user_id":"test-user"}`)
		case "/data/user-data/get":
			state.reads++
			_, _ = w.Write(data)
		case "/data/shopping-lists/update":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			defer func() { _ = r.MultipartForm.RemoveAll() }()
			operations := new(pb.PBListOperationList)
			if err := proto.Unmarshal([]byte(r.FormValue("operations")), operations); err != nil || len(operations.Operations) != 1 {
				t.Errorf("invalid operation wrapper: %v", err)
				w.WriteHeader(400)
				return
			}
			state.operations = append(state.operations, operations.Operations[0])
			if len(state.operations) == failAt {
				w.WriteHeader(http.StatusForbidden)
				_, _ = io.WriteString(w, "upstream private diagnostic")
				return
			}
			w.WriteHeader(http.StatusOK)
			operation := operations.Operations[0]
			acknowledgment := protowire.AppendString(protowire.AppendTag(nil, 3, protowire.BytesType), operation.GetMetadata().GetOperationId())
			if state.acknowledge != nil {
				acknowledgment = state.acknowledge(operation, len(state.operations))
			}
			_, _ = w.Write(acknowledgment)
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	t.Cleanup(state.server.Close)
	return state
}

func (s *commandServer) counts() (int, int, []*pb.PBListOperation) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.auth, s.reads, append([]*pb.PBListOperation(nil), s.operations...)
}

type commandEnvelope struct {
	OK   bool `json:"ok"`
	Data struct {
		List    anylist.List             `json:"list"`
		Items   []anylist.Item           `json:"items"`
		Results []anylist.MutationResult `json:"results"`
	} `json:"data"`
	Error *anylist.Error `json:"error"`
}

func runItemCommand(t *testing.T, server *commandServer, input string, args ...string) (int, commandEnvelope) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	app := newApp(bytes.NewBufferString(input), &stdout, &stderr, options{
		configPath: filepath.Join(t.TempDir(), "config.json"),
		anylist:    anylist.Options{BaseURL: server.server.URL, HTTPClient: server.server.Client()},
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
	code := app.run(context.Background(), append([]string{"--json"}, args...))
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
	var result commandEnvelope
	decoder := json.NewDecoder(&stdout)
	if err := decoder.Decode(&result); err != nil {
		t.Fatal(err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		t.Fatalf("extra output after JSON: %v", err)
	}
	return code, result
}

func TestCommandRepeatedNamesCreateOneItem(t *testing.T) {
	server := newCommandServer(t, nil, 0)
	code, result := runItemCommand(t, server, `[{"name":"milk"},{"name":"milk"},{"name":"milk","new":true}]`, "add", "--stdin", "--list-id", "list")
	if code != 0 || !result.OK || len(result.Data.Results) != 3 {
		t.Fatalf("result: code%d %+v", code, result)
	}
	outcomes := result.Data.Results
	if outcomes[0].Outcome != "added" || outcomes[1].Outcome != "unchanged" || outcomes[2].Outcome != "added" || outcomes[0].ID != outcomes[1].ID || outcomes[0].ID == outcomes[2].ID {
		t.Fatalf("outcomes: %+v", outcomes)
	}
	_, _, operations := server.counts()
	if len(operations) != 2 {
		t.Fatalf("writes%d want2", len(operations))
	}
	for i, op := range operations {
		if op.GetMetadata().GetHandlerId() != "add-shopping-list-item" || op.GetListId() != "list" || op.GetListItem().GetName() != "milk" || op.GetListItemId() == "" {
			t.Fatalf("write%d: invalid add payload", i)
		}
	}
}

func TestCommandInvalidFinalRecordMakesZeroWrites(t *testing.T) {
	t.Run("syntax", func(t *testing.T) {
		server := newCommandServer(t, nil, 0)
		code, result := runItemCommand(t, server, `[{"name":"milk"},{"name":"tea","quantity":3}]`, "add", "--stdin", "--list-id", "list")
		auth, reads, ops := server.counts()
		if code != 2 || result.Error == nil || result.Error.Code != "input" || auth != 0 || reads != 0 || len(ops) != 0 {
			t.Fatalf("code%d auth%d reads%d writes%d err%v", code, auth, reads, len(ops), result.Error)
		}
	})
	t.Run("projected selection", func(t *testing.T) {
		server := newCommandServer(t, []*pb.ListItem{{Identifier: proto.String("milk"), ListId: proto.String("list"), Name: proto.String("milk")}}, 0)
		code, result := runItemCommand(t, server, `[{"name":"milk"},{"name":"milk"}]`, "remove", "--stdin", "--list-id", "list")
		_, _, ops := server.counts()
		if code != 1 || len(ops) != 0 || len(result.Data.Results) != 2 || result.Data.Results[0].Outcome != "skipped" || result.Data.Results[1].Outcome != "failed" {
			t.Fatalf("preflight result: %+v writes%d code%d", result, len(ops), code)
		}
	})
}

func TestCommandBatchStopsOnFailure(t *testing.T) {
	server := newCommandServer(t, nil, 2)
	code, result := runItemCommand(t, server, `[{"name":"one"},{"name":"two"},{"name":"three"}]`, "add", "--stdin", "--list-id", "list")
	_, _, ops := server.counts()
	if code != 1 || len(ops) != 2 || result.Error == nil || result.Error.Code != "permission" || len(result.Data.Results) != 3 {
		t.Fatalf("batch result: code%d %+v writes%d", code, result, len(ops))
	}
	for i, want := range []string{"added", "failed", "skipped"} {
		got := result.Data.Results[i]
		if got.Outcome != want || got.Index == nil || *got.Index != i || got.ID == "" {
			t.Fatalf("result%d: %+v", i, got)
		}
	}
}

func TestCommandMultiFieldFailureIsUnknown(t *testing.T) {
	server := newCommandServer(t, []*pb.ListItem{{Identifier: proto.String("milk"), ListId: proto.String("list"), Name: proto.String("milk")}}, 2)
	code, result := runItemCommand(t, server, `[{"id":"milk","quantity":"2","notes":"cold"},{"id":"milk","notes":"later"}]`, "edit", "--stdin", "--list-id", "list")
	_, _, ops := server.counts()
	if code != 1 || len(ops) != 2 || len(result.Data.Results) != 2 || result.Data.Results[0].Outcome != "unknown" || result.Data.Results[1].Outcome != "skipped" {
		t.Fatalf("compound result: code%d %+v writes%d", code, result, len(ops))
	}
}

func TestCommandCheckedReusePreservesMetadata(t *testing.T) {
	item := &pb.ListItem{Identifier: proto.String("milk-id"), ListId: proto.String("list"), Name: proto.String("milk"), Checked: proto.Bool(true), Details: proto.String("keep cold"), QuantityPb: &pb.PBItemQuantity{RawQuantity: proto.String("two cartons")}}
	server := newCommandServer(t, []*pb.ListItem{item}, 0)
	code, result := runItemCommand(t, server, "", "add", "milk", "--list", "Groceries")
	_, _, ops := server.counts()
	if code != 0 || len(ops) != 1 || ops[0].GetMetadata().GetHandlerId() != "set-list-item-checked" || ops[0].GetUpdatedValue() != "n" {
		t.Fatalf("reuse writes%d result%+v code%d", len(ops), result, code)
	}
	got := result.Data.Results[0].Item
	if got == nil || got.ID != "milk-id" || got.Checked || got.Notes != "keep cold" || got.Quantity != "two cartons" {
		t.Fatalf("reused item: %+v", got)
	}
}

func TestCommandSingleItemMutationFlags(t *testing.T) {
	for _, test := range []struct {
		name, flag, value, handler, category string
	}{
		{"rename", "--name", "oat milk", "set-list-item-name", "dairy"},
		{"quantity", "--quantity", "3 cartons", "set-list-item-quantity-v2", "dairy"},
		{"notes", "--notes", "keep cold", "set-list-item-details", "dairy"},
		{"clear quantity", "--quantity", "", "set-list-item-quantity-v2", "dairy"},
		{"clear notes", "--notes", "", "set-list-item-details", "dairy"},
		{"category name", "--category", "Dairy", "update-list-item-category-assignment", ""},
		{"category ID", "--category-id", "dairy", "update-list-item-category-assignment", ""},
		{"clear category", "--category", "", "update-list-item-category-assignment", "dairy"},
	} {
		t.Run(test.name, func(t *testing.T) {
			item := &pb.ListItem{
				Identifier: proto.String("milk-id"), ListId: proto.String("list"), Name: proto.String("milk"),
				Details: proto.String("whole"), QuantityPb: &pb.PBItemQuantity{RawQuantity: proto.String("two cartons")},
				Checked: proto.Bool(true),
			}
			if test.category != "" {
				item.CategoryAssignments = []*pb.PBListItemCategoryAssignment{{
					Identifier:      proto.String("93994a3ca5f257079d3067ed79cbb395"),
					CategoryGroupId: proto.String("group"), CategoryId: proto.String(test.category),
				}}
			}
			server := newCommandServer(t, []*pb.ListItem{item}, 0)
			code, result := runItemCommand(t, server, "", "edit", "--id", "milk-id", "--list-id", "list", test.flag, test.value)
			_, _, operations := server.counts()
			if code != 0 || !result.OK || result.Error != nil || len(result.Data.Results) != 1 || len(operations) != 1 {
				t.Fatalf("single-item %s: exit %d, error %+v, writes %d, results %+v", test.flag, code, result.Error, len(operations), result.Data.Results)
			}
			got, operation := result.Data.Results[0], operations[0]
			if operation.GetMetadata().GetHandlerId() != test.handler || operation.GetListId() != "list" || operation.GetListItemId() != "milk-id" {
				t.Fatalf("single-item %s sent wrong operation: %v", test.flag, operation)
			}
			if got.Outcome != "updated" || got.Item == nil || got.ID != "milk-id" {
				t.Fatalf("single-item %s result: %+v", test.flag, got)
			}
			wantName, wantQuantity, wantNotes, wantCategory := "milk", "two cartons", "whole", test.category
			switch test.flag {
			case "--name":
				wantName = test.value
			case "--quantity":
				wantQuantity = test.value
			case "--notes":
				wantNotes = test.value
			case "--category", "--category-id":
				wantCategory = ""
				if test.value != "" {
					wantCategory = "dairy"
				}
			}
			if got.Item.ID != "milk-id" || got.Item.Name != wantName || got.Item.Quantity != wantQuantity || got.Item.Notes != wantNotes || got.Item.CategoryID != wantCategory || !got.Item.Checked {
				t.Fatalf("single-item %s changed wrong fields: %+v", test.flag, got.Item)
			}
		})
	}
}

func TestCommandRenameThenSelectProjectedName(t *testing.T) {
	server := newCommandServer(t, []*pb.ListItem{{Identifier: proto.String("milk-id"), ListId: proto.String("list"), Name: proto.String("milk")}}, 0)
	code, result := runItemCommand(t, server, `[{"name":"milk","newName":"whole milk"},{"name":"whole milk","notes":"cold"}]`, "edit", "--stdin", "--list-id", "list")
	_, _, ops := server.counts()
	if code != 0 || len(ops) != 2 || ops[0].GetMetadata().GetHandlerId() != "set-list-item-name" || ops[1].GetMetadata().GetHandlerId() != "set-list-item-details" || ops[0].GetListItemId() != "milk-id" || ops[1].GetListItemId() != "milk-id" {
		t.Fatalf("rename/select result%+v writes%d code%d", result, len(ops), code)
	}
}

func TestCommandItemFiltersOnlyAffectReads(t *testing.T) {
	items := []*pb.ListItem{{Identifier: proto.String("one"), ListId: proto.String("list"), Name: proto.String("one"), Checked: proto.Bool(false)}, {Identifier: proto.String("two"), ListId: proto.String("list"), Name: proto.String("two"), Checked: proto.Bool(true)}}
	for _, test := range []struct {
		name  string
		flags []string
		want  int
		id    string
	}{{"unchecked", nil, 1, "one"}, {"checked", []string{"--checked"}, 1, "two"}, {"all", []string{"--all"}, 2, ""}} {
		t.Run(test.name, func(t *testing.T) {
			server := newCommandServer(t, items, 0)
			args := append([]string{"items", "--list-id", "list"}, test.flags...)
			code, result := runItemCommand(t, server, "", args...)
			if code != 0 || len(result.Data.Items) != test.want {
				t.Fatalf("read result: code%d %+v", code, result)
			}
			if test.id != "" && result.Data.Items[0].ID != test.id {
				t.Fatalf("filtered item: %+v", result.Data.Items)
			}
		})
	}
}

func TestCommandUnacknowledgedWriteStopsBatch(t *testing.T) {
	for _, test := range []struct {
		name     string
		response []byte
	}{
		{"empty", nil},
		{"malformed", []byte{0xff}},
		{"missing acknowledgment", []byte{0x0a, 0x00}},
		{"wrong operation", protowire.AppendString(protowire.AppendTag(nil, 3, protowire.BytesType), "different-operation")},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := newCommandServer(t, nil, 0)
			server.mu.Lock()
			server.acknowledge = func(operation *pb.PBListOperation, index int) []byte {
				if index == 2 {
					return test.response
				}
				return protowire.AppendString(protowire.AppendTag(nil, 3, protowire.BytesType), operation.GetMetadata().GetOperationId())
			}
			server.mu.Unlock()
			code, result := runItemCommand(t, server, `[{"name":"one"},{"name":"two"},{"name":"three"}]`, "add", "--stdin", "--list-id", "list")
			_, _, operations := server.counts()
			if code != 1 || result.Error == nil || result.Error.Code != "protocol" || len(operations) != 2 || len(result.Data.Results) != 3 {
				t.Fatalf("unacknowledged batch: code%d error%v writes%d results%+v", code, result.Error, len(operations), result.Data.Results)
			}
			for i, want := range []string{"added", "unknown", "skipped"} {
				got := result.Data.Results[i]
				if got.Outcome != want || got.ID == "" || got.Index == nil || *got.Index != i {
					t.Fatalf("result%d: %+v", i, got)
				}
			}
		})
	}
}
