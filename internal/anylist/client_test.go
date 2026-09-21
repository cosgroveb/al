package anylist

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	pb "github.com/cosgroveb/al/internal/anylistpb"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

type requestStep struct {
	path         string
	check        func(*http.Request) error
	response     []byte
	responseFunc func() ([]byte, error)
	status       int
	delay        time.Duration
}

func scriptedClient(t *testing.T, steps ...requestStep) *Client {
	t.Helper()
	var mu sync.Mutex
	next := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if next == len(steps) {
			t.Errorf("unexpected request: %s", r.URL.Path)
			w.WriteHeader(500)
			return
		}
		step := steps[next]
		next++
		if r.Method != "POST" || r.URL.Path != step.path {
			t.Errorf("request %d: %s %s, want POST %s", next, r.Method, r.URL.Path, step.path)
			w.WriteHeader(500)
			return
		}
		if r.Header.Get("X-AnyLeaf-API-Version") != "3" || r.Header.Get("X-AnyLeaf-Client-Identifier") != "test-client" {
			t.Error("missing API/client headers")
			w.WriteHeader(500)
			return
		}
		if step.path != "/auth/token" && r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing bearer token")
			w.WriteHeader(500)
			return
		}
		if step.check != nil {
			if err := step.check(r); err != nil {
				t.Error(err)
				w.WriteHeader(500)
				return
			}
		}
		if step.delay > 0 {
			time.Sleep(step.delay)
		}
		status := step.status
		if status == 0 {
			status = http.StatusOK
		}
		response := step.response
		if step.responseFunc != nil {
			var err error
			response, err = step.responseFunc()
			if err != nil {
				t.Error(err)
				w.WriteHeader(500)
				return
			}
		}
		w.WriteHeader(status)
		_, _ = w.Write(response)
	}))
	t.Cleanup(func() {
		server.Close()
		mu.Lock()
		defer mu.Unlock()
		if next != len(steps) {
			t.Errorf("received %d requests, want %d", next, len(steps))
		}
	})
	return NewClient(Options{Email: "test@example.invalid", Password: "test-password", ClientID: "test-client", BaseURL: server.URL, HTTPClient: server.Client()})
}

func authStep() requestStep {
	return requestStep{path: "/auth/token", response: []byte(`{"access_token":"test-token","user_id":"test-user"}`), check: func(r *http.Request) error {
		if err := r.ParseMultipartForm(1024); err != nil {
			return err
		}
		defer func() { _ = r.MultipartForm.RemoveAll() }()
		if r.FormValue("email") != "test@example.invalid" || r.FormValue("password") != "test-password" || len(r.MultipartForm.Value) != 2 {
			return errors.New("incorrect login form")
		}
		if r.Header.Get("Authorization") != "" {
			return errors.New("authentication request included bearer token")
		}
		return nil
	}}
}

func acknowledgedStep(path string, check func(*http.Request) error) requestStep {
	var response []byte
	return requestStep{path: path, check: func(r *http.Request) error {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return err
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		outer, err := requestFields(r, "operations")
		if err != nil {
			return err
		}
		op, err := nestedFields(outer, 1)
		if err != nil {
			return err
		}
		metadata, err := nestedFields(op, 1)
		if err != nil {
			return err
		}
		id, err := one(metadata, 1)
		if err != nil {
			return err
		}
		response = protowire.AppendString(protowire.AppendTag(nil, 3, protowire.BytesType), string(id))
		if check != nil {
			r.Body = io.NopCloser(bytes.NewReader(body))
			return check(r)
		}
		return nil
	}, responseFunc: func() ([]byte, error) { return response, nil }}
}

func marshal(t *testing.T, message proto.Message) []byte {
	t.Helper()
	data, err := proto.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func readStep(t *testing.T, account *pb.PBUserDataResponse) requestStep {
	t.Helper()
	return requestStep{path: "/data/user-data/get", response: marshal(t, account), check: func(r *http.Request) error {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			return err
		}
		if len(data) != 0 {
			return errors.New("full account read should have an empty body")
		}
		return nil
	}}
}

func loadedClient(t *testing.T, account *pb.PBUserDataResponse, steps ...requestStep) *Client {
	t.Helper()
	prefix := []requestStep{authStep(), readStep(t, account)}
	client := scriptedClient(t, append(prefix, steps...)...)
	if _, err := client.Lists(context.Background()); err != nil {
		t.Fatal(err)
	}
	return client
}

func fixtureAccount() *pb.PBUserDataResponse {
	return &pb.PBUserDataResponse{
		ShoppingListsResponse: &pb.ShoppingListsResponse{NewLists: []*pb.ShoppingList{{Identifier: proto.String("list"), Name: proto.String("Groceries"), Items: []*pb.ListItem{
			{Identifier: proto.String("item"), ListId: proto.String("list"), Name: proto.String("Milk"), Checked: proto.Bool(true), Details: proto.String("whole"),
				QuantityPb: &pb.PBItemQuantity{Amount: proto.String("2"), Unit: proto.String("cartons"), RawQuantity: proto.String("two cartons")}, DeprecatedQuantity: proto.String("old quantity"),
				Category: proto.String("dairy"), CategoryMatchId: proto.String("dairy")},
		}}}, ListResponses: []*pb.PBListResponse{{ListId: proto.String("list"), CategoryGroupResponses: []*pb.PBListCategoryGroupResponse{
			{CategoryGroup: &pb.PBListCategoryGroup{Identifier: proto.String("group"), ListId: proto.String("list"), Name: proto.String("Main"), Categories: []*pb.PBListCategory{
				{Identifier: proto.String("dairy-id"), Name: proto.String("Dairy"), SystemCategory: proto.String("dairy"), SortIndex: proto.Int32(1)},
				{Identifier: proto.String("produce-id"), Name: proto.String("Produce"), SystemCategory: proto.String("produce"), SortIndex: proto.Int32(0)},
			}}},
		}}}},
		ListFoldersResponse: &pb.PBListFoldersResponse{ListDataId: proto.String("list-data"), RootFolderId: proto.String("root"), ListFolders: []*pb.PBListFolder{
			{Identifier: proto.String("root"), Items: []*pb.PBListFolderItem{{Identifier: proto.String("list"), ItemType: proto.Int32(0)}}},
		}},
		ListSettingsResponse: &pb.PBListSettingsList{Settings: []*pb.PBListSettings{{Identifier: proto.String("settings"), UserId: proto.String("test-user"), ListId: proto.String("list"),
			Timestamp: proto.Float64(123), ListCategoryGroupId: proto.String("group")}}},
	}
}

// Inspect numeric wire fields independently of the generated request schema.
type wireFields map[protowire.Number][][]byte

func fields(data []byte) (wireFields, error) {
	result := make(wireFields)
	for len(data) > 0 {
		number, kind, n := protowire.ConsumeTag(data)
		if n < 0 {
			return nil, errors.New("invalid protobuf tag")
		}
		data = data[n:]
		var value []byte
		if kind == protowire.BytesType {
			value, n = protowire.ConsumeBytes(data)
		} else {
			n = protowire.ConsumeFieldValue(number, kind, data)
			if n >= 0 {
				value = data[:n]
			}
		}
		if n < 0 {
			return nil, errors.New("invalid protobuf field")
		}
		result[number] = append(result[number], value)
		data = data[n:]
	}
	return result, nil
}

func one(values wireFields, number protowire.Number) ([]byte, error) {
	if len(values[number]) != 1 {
		return nil, fmt.Errorf("field %d count = %d, want 1", number, len(values[number]))
	}
	return values[number][0], nil
}

func nestedFields(values wireFields, number protowire.Number) (wireFields, error) {
	data, err := one(values, number)
	if err != nil {
		return nil, err
	}
	return fields(data)
}

func textField(values wireFields, number protowire.Number, want string) error {
	data, err := one(values, number)
	if err != nil {
		return err
	}
	if got := string(data); got != want {
		return fmt.Errorf("field %d = %q, want %q", number, got, want)
	}
	return nil
}

func bytesField(values wireFields, number protowire.Number, want []byte) error {
	data, err := one(values, number)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, want) {
		return fmt.Errorf("field %d = %x, want %x", number, data, want)
	}
	return nil
}

func exactFields(values wireFields, numbers ...protowire.Number) error {
	if len(values) != len(numbers) {
		return fmt.Errorf("field count = %d, want %d (fields %v)", len(values), len(numbers), numbers)
	}
	for _, number := range numbers {
		if _, ok := values[number]; !ok {
			return fmt.Errorf("missing field %d", number)
		}
	}
	return nil
}

func requestFields(r *http.Request, name string) (wireFields, error) {
	data, err := multipartPayload(r, name)
	if err != nil {
		return nil, err
	}
	return fields(data)
}

func multipartPayload(r *http.Request, name string) ([]byte, error) {
	media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "multipart/form-data" {
		return nil, errors.New("expected multipart form")
	}
	reader := multipart.NewReader(r.Body, params["boundary"])
	part, err := reader.NextPart()
	if err != nil {
		return nil, err
	}
	if part.FormName() != name || part.FileName() != "" || part.Header.Get("Content-Type") != "" {
		return nil, errors.New("incorrect protobuf form part")
	}
	data, err := io.ReadAll(part)
	if err != nil {
		return nil, err
	}
	if _, err := reader.NextPart(); err != io.EOF {
		return nil, errors.New("expected exactly one form part")
	}
	return data, nil
}

func operation(r *http.Request, handler, listID, itemID string) (wireFields, error) {
	outer, err := requestFields(r, "operations")
	if err != nil {
		return nil, err
	}
	if err := exactFields(outer, 1); err != nil {
		return nil, err
	}
	op, err := nestedFields(outer, 1)
	if err != nil {
		return nil, err
	}
	if err := errors.Join(checkMetadata(op, handler), textField(op, 2, listID)); err != nil {
		return nil, err
	}
	if itemID != "" {
		if err := textField(op, 3, itemID); err != nil {
			return nil, err
		}
	}
	return op, nil
}

func checkMetadata(op wireFields, handler string) error {
	meta, err := nestedFields(op, 1)
	if err != nil {
		return err
	}
	data, err := one(meta, 1)
	if err != nil {
		return err
	}
	id := string(data)
	if len(id) != 32 || id[12] != '4' || !strings.ContainsRune("89ab", rune(id[16])) {
		return fmt.Errorf("invalid operation ID %q", id)
	}
	return errors.Join(exactFields(meta, 1, 2, 3), textField(meta, 2, handler), textField(meta, 3, "test-user"))
}

func safeError(t *testing.T, err error, code string, unknown bool) *Error {
	t.Helper()
	var serviceError *Error
	if !errors.As(err, &serviceError) {
		t.Fatalf("expected safe AnyList error, got %v", err)
	}
	if serviceError.Code != code || serviceError.Unknown != unknown {
		t.Errorf("error = %+v, want code %s unknown %v", serviceError, code, unknown)
	}
	if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "test-password") || strings.Contains(err.Error(), "test-token") {
		t.Fatal("error leaked upstream content or credentials")
	}
	return serviceError
}

func TestListsVisibilityAndSelectedCategoryResolution(t *testing.T) {
	account := fixtureAccount()
	account.ShoppingListsResponse.NewLists[0].Timestamp = proto.Float64(1700000000.25)
	account.ShoppingListsResponse.NewLists[0].SharedUsers = []*pb.PBEmailUserIDPair{{UserId: proto.String("other-user")}}
	account.ShoppingListsResponse.NewLists[0].Items[0].ServerModTime = proto.Float64(1700000001.5)
	account.ShoppingListsResponse.NewLists = append(account.ShoppingListsResponse.NewLists,
		&pb.ShoppingList{Identifier: proto.String("nested"), Name: proto.String("Nested")},
		&pb.ShoppingList{Identifier: proto.String("orphan"), Name: proto.String("Orphan")})
	account.ListFoldersResponse.ListFolders[0].Items = append(account.ListFoldersResponse.ListFolders[0].Items,
		&pb.PBListFolderItem{Identifier: proto.String("folder"), ItemType: proto.Int32(1)})
	account.ListFoldersResponse.ListFolders = append(account.ListFoldersResponse.ListFolders, &pb.PBListFolder{Identifier: proto.String("folder"), Items: []*pb.PBListFolderItem{
		{Identifier: proto.String("nested"), ItemType: proto.Int32(0)}, {Identifier: proto.String("root"), ItemType: proto.Int32(1)}, {Identifier: proto.String("missing"), ItemType: proto.Int32(0)},
	}})
	account.ShoppingListsResponse.ListResponses = append(account.ShoppingListsResponse.ListResponses, &pb.PBListResponse{ListId: proto.String("nested"), CategoryGroupResponses: []*pb.PBListCategoryGroupResponse{
		{CategoryGroup: &pb.PBListCategoryGroup{Identifier: proto.String("one")}}, {CategoryGroup: &pb.PBListCategoryGroup{Identifier: proto.String("two")}},
	}})
	client := scriptedClient(t, authStep(), readStep(t, account))
	lists, err := client.Lists(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(lists, []List{{ID: "list", Name: "Groceries", Shared: true, ModifiedAt: "2023-11-14T22:13:20.25Z"}, {ID: "nested", Name: "Nested"}}) {
		t.Fatalf("visible lists = %+v", lists)
	}
	state, err := client.ListState("list")
	if err != nil {
		t.Fatal(err)
	}
	if state.CategoryGroupID != "group" || state.Items[0].CategoryID != "dairy-id" || state.Items[0].Quantity != "two cartons" || state.Items[0].QuantityDetails.Unit != "cartons" || state.Items[0].ModifiedAt != "2023-11-14T22:13:21.5Z" {
		t.Errorf("mapped state = %+v", state)
	}
	if state.Categories[0].Name != "Produce" {
		t.Fatal("categories are not ordered by sortIndex")
	}
	_, err = client.ListState("nested")
	_ = safeError(t, err, "category_ambiguous", false)
	_, err = client.ListState("orphan")
	_ = safeError(t, err, "not_found", false)
}

func TestActiveCategoryGroupPrecedence(t *testing.T) {
	for _, test := range []struct {
		name, filterGroup, settingsGroup, want string
		defaultExists                          bool
	}{
		{"store filter", "filtered", "selected", "filtered", true},
		{"missing filter group", "missing", "selected", "selected", true},
		{"default group", "missing", "missing", "default", true},
		{"only group", "missing", "missing", "selected", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			account := fixtureAccount()
			response := account.ShoppingListsResponse.ListResponses[0]
			response.CategoryGroupResponses = []*pb.PBListCategoryGroupResponse{{CategoryGroup: &pb.PBListCategoryGroup{Identifier: proto.String("selected")}}}
			if test.defaultExists {
				response.CategoryGroupResponses = append(response.CategoryGroupResponses,
					&pb.PBListCategoryGroupResponse{CategoryGroup: &pb.PBListCategoryGroup{Identifier: proto.String(defaultGroupID("list"))}},
					&pb.PBListCategoryGroupResponse{CategoryGroup: &pb.PBListCategoryGroup{Identifier: proto.String("filtered")}})
			}
			response.StoreFilters = []*pb.PBStoreFilter{{Identifier: proto.String("filter"), ListCategoryGroupId: proto.String(test.filterGroup)}}
			account.ListSettingsResponse.Settings[0].StoreFilterId = proto.String("filter")
			account.ListSettingsResponse.Settings[0].ListCategoryGroupId = proto.String(test.settingsGroup)
			client := loadedClient(t, account)
			state, err := client.ListState("list")
			if err != nil {
				t.Fatal(err)
			}
			want := test.want
			if want == "default" {
				want = defaultGroupID("list")
			}
			if state.CategoryGroupID != want {
				t.Errorf("group = %s, want %s", state.CategoryGroupID, want)
			}
		})
	}
}

func TestQuantityReadForms(t *testing.T) {
	for _, test := range []struct {
		name     string
		quantity *pb.PBItemQuantity
		want     string
	}{
		{"legacy", nil, "old quantity"},
		{"modern empty", &pb.PBItemQuantity{RawQuantity: proto.String("")}, ""},
		{"amount and unit", &pb.PBItemQuantity{Amount: proto.String("2"), Unit: proto.String("kg")}, "2 kg"},
	} {
		t.Run(test.name, func(t *testing.T) {
			account := fixtureAccount()
			account.ShoppingListsResponse.NewLists[0].Items[0].QuantityPb = test.quantity
			client := loadedClient(t, account)
			state, err := client.ListState("list")
			if err != nil {
				t.Fatal(err)
			}
			if state.Items[0].Quantity != test.want {
				t.Errorf("quantity = %q, want %q", state.Items[0].Quantity, test.want)
			}
		})
	}
}
