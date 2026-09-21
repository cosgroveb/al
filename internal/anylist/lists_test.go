package anylist

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"net/http"
	"testing"
	"time"

	pb "github.com/cosgroveb/al/internal/anylistpb"
	"google.golang.org/protobuf/proto"
)

func TestCreateListRequestsAndVerification(t *testing.T) {
	account := fixtureAccount()
	var createdID, groupID string
	client := loadedClient(t, account,
		acknowledgedStep("/data/shopping-lists/update", func(r *http.Request) error {
			outer, err := requestFields(r, "operations")
			if err != nil {
				return err
			}
			op, err := nestedFields(outer, 1)
			if err != nil {
				return err
			}
			id, err := one(op, 2)
			if err != nil {
				return err
			}
			createdID = string(id)
			if len(createdID) != 32 {
				return errors.New("new list ID must be generated before writing")
			}
			listData, err := one(op, 7)
			if err != nil {
				return err
			}
			list, err := fields(listData)
			if err != nil {
				return err
			}
			groupData, err := one(op, 19)
			if err != nil {
				return err
			}
			group, err := fields(groupData)
			if err != nil {
				return err
			}
			id, err = one(group, 1)
			if err != nil {
				return err
			}
			groupID = string(id)
			if groupID != defaultGroupID(createdID) {
				return errors.New("incorrect default category group ID")
			}
			category, err := nestedFields(group, 5)
			if err != nil {
				return err
			}
			otherID, err := one(group, 8)
			if err != nil {
				return err
			}
			if err := errors.Join(
				exactFields(outer, 1), exactFields(op, 1, 2, 7, 8, 19), checkMetadata(op, "new-shopping-list"), textField(op, 8, "root"),
				exactFields(list, 1, 2, 3, 5, 10, 16, 17, 18), textField(list, 1, createdID),
				textField(list, 3, "Disposable"), textField(list, 5, "test-user"), bytesField(list, 10, []byte{1}),
				bytesField(list, 16, []byte{1}), bytesField(list, 17, []byte{0}), bytesField(list, 18, []byte{0}),
				exactFields(group, 1, 3, 5, 8), textField(group, 3, createdID), exactFields(category, 1, 3, 4, 5, 6, 7, 9),
				textField(category, 1, string(otherID)), textField(category, 3, groupID), textField(category, 4, createdID),
				textField(category, 5, "Other"), textField(category, 6, "other"), textField(category, 7, "other"),
				bytesField(category, 9, []byte{0}),
			); err != nil {
				return err
			}
			var created pb.ShoppingList
			if err := proto.Unmarshal(listData, &created); err != nil {
				return err
			}
			var categories pb.PBListCategoryGroup
			if err := proto.Unmarshal(groupData, &categories); err != nil {
				return err
			}
			account.ShoppingListsResponse.NewLists = append(account.ShoppingListsResponse.NewLists, &created)
			account.ShoppingListsResponse.ListResponses = append(account.ShoppingListsResponse.ListResponses, &pb.PBListResponse{ListId: proto.String(createdID), CategoryGroupResponses: []*pb.PBListCategoryGroupResponse{{CategoryGroup: &categories}}})
			account.ListFoldersResponse.ListFolders[0].Items = append(account.ListFoldersResponse.ListFolders[0].Items, &pb.PBListFolderItem{Identifier: proto.String(createdID), ItemType: proto.Int32(0)})
			return nil
		}),
		acknowledgedStep("/data/list-settings/update", func(r *http.Request) error {
			outer, err := requestFields(r, "operations")
			if err != nil {
				return err
			}
			op, err := nestedFields(outer, 1)
			if err != nil {
				return err
			}
			settingsData, err := one(op, 2)
			if err != nil {
				return err
			}
			settings, err := fields(settingsData)
			if err != nil {
				return err
			}
			hash := md5.Sum([]byte("test-user-" + createdID))
			if err := errors.Join(
				exactFields(outer, 1), exactFields(op, 1, 2), checkMetadata(op, "set-list-category-group-id"),
				exactFields(settings, 1, 2, 3, 27), textField(settings, 1, hex.EncodeToString(hash[:])),
				textField(settings, 2, "test-user"), textField(settings, 3, createdID), textField(settings, 27, groupID),
			); err != nil {
				return err
			}
			var setting pb.PBListSettings
			if err := proto.Unmarshal(settingsData, &setting); err != nil {
				return err
			}
			account.ListSettingsResponse.Settings = append(account.ListSettingsResponse.Settings, &setting)
			return nil
		}),
		requestStep{path: "/data/user-data/get", responseFunc: func() ([]byte, error) { return proto.Marshal(account) }},
	)
	result, err := client.CreateList(context.Background(), "Disposable")
	if err != nil {
		t.Fatal(err)
	}
	if result.ID == "" || result.List == nil || result.List.ID != result.ID || result.List.Name != "Disposable" || result.Outcome != "created" {
		t.Fatalf("create result = %+v", result)
	}
	state, err := client.ListState(result.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Categories) != 1 || state.Categories[0].Name != "Other" || state.Items == nil || len(state.Items) != 0 {
		t.Fatalf("created state = %+v", state)
	}
}

func TestCreateListPartialFailureRetainsID(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		delay  time.Duration
		code   string
	}{
		{"rejection", 403, 0, "permission"},
		{"timeout", 200, 50 * time.Millisecond, "transport"},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := loadedClient(t, fixtureAccount(),
				acknowledgedStep("/data/shopping-lists/update", nil),
				requestStep{path: "/data/list-settings/update", status: test.status, delay: test.delay},
			)
			if test.delay > 0 {
				client.http.Timeout = 10 * time.Millisecond
			}
			result, err := client.CreateList(context.Background(), "Disposable")
			_ = safeError(t, err, test.code, true)
			if len(result.ID) != 32 || result.Outcome != "unknown" {
				t.Fatalf("partial create lost target ID: %+v", result)
			}
		})
	}
}

func TestCreateListFirstWriteFailureRetainsID(t *testing.T) {
	client := loadedClient(t, fixtureAccount(), requestStep{path: "/data/shopping-lists/update", status: 403})
	result, err := client.CreateList(context.Background(), "Disposable")
	_ = safeError(t, err, "permission", false)
	if len(result.ID) != 32 || result.Outcome != "failed" {
		t.Fatalf("failed create lost target ID: %+v", result)
	}
}

func TestRenameListRequestAndVerification(t *testing.T) {
	before := fixtureAccount()
	after := proto.Clone(before).(*pb.PBUserDataResponse)
	after.ShoppingListsResponse.NewLists[0].Name = proto.String("Renamed")
	client := loadedClient(t, before,
		acknowledgedStep("/data/shopping-lists/update", func(r *http.Request) error {
			op, err := operation(r, "rename-list", "list", "")
			if err != nil {
				return err
			}
			return errors.Join(exactFields(op, 1, 2, 4), textField(op, 4, "Renamed"))
		}),
		readStep(t, after),
	)
	result, err := client.RenameList(context.Background(), "list", "Renamed")
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != "renamed" || result.ID != "list" || result.List.Name != "Renamed" {
		t.Fatalf("rename result = %+v", result)
	}
	state, err := client.ListState("list")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Items) != 1 || state.Items[0].Name != "Milk" {
		t.Fatal("rename lost existing items")
	}
}

func checkFolderDelete(r *http.Request) error {
	outer, err := requestFields(r, "operations")
	if err != nil {
		return err
	}
	op, err := nestedFields(outer, 1)
	if err != nil {
		return err
	}
	item, err := nestedFields(op, 4)
	if err != nil {
		return err
	}
	return errors.Join(
		exactFields(outer, 1), exactFields(op, 1, 2, 4, 5), checkMetadata(op, "delete-folder-items"),
		textField(op, 2, "list-data"), textField(op, 5, "root"), exactFields(item, 1, 2),
		textField(item, 1, "list"), bytesField(item, 2, []byte{0}),
	)
}

func checkSettingsDelete(r *http.Request) error {
	outer, err := requestFields(r, "operations")
	if err != nil {
		return err
	}
	op, err := nestedFields(outer, 1)
	if err != nil {
		return err
	}
	settings, err := nestedFields(op, 2)
	if err != nil {
		return err
	}
	return errors.Join(
		exactFields(outer, 1), exactFields(op, 1, 2), checkMetadata(op, "remove-list-settings"),
		exactFields(settings, 1, 2, 3, 4), textField(settings, 1, "settings"), textField(settings, 2, "test-user"),
		textField(settings, 3, "list"), bytesField(settings, 4, []byte{0, 0, 0, 0, 0, 0xc0, 0x5e, 0x40}),
	)
}

func removedAccount() *pb.PBUserDataResponse {
	account := fixtureAccount()
	account.ListFoldersResponse.ListFolders[0].Items = nil
	account.ListSettingsResponse.Settings = nil
	// The object remains in list data after folder removal. It is not visible.
	return account
}

func TestDeleteListRequestsAndVisibilityVerification(t *testing.T) {
	client := loadedClient(t, fixtureAccount(),
		acknowledgedStep("/data/list-folders/update", checkFolderDelete),
		acknowledgedStep("/data/list-settings/update", checkSettingsDelete),
		readStep(t, removedAccount()),
	)
	result, err := client.DeleteList(context.Background(), "list")
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "list" || result.Outcome != "removed" {
		t.Fatalf("delete result = %+v", result)
	}
	_, err = client.ListState("list")
	_ = safeError(t, err, "not_found", false)
}

func TestDeleteListPartialFailureAndExplicitIDRecovery(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		delay  time.Duration
		code   string
	}{
		{"rejection", 403, 0, "permission"},
		{"timeout", 200, 50 * time.Millisecond, "transport"},
	} {
		t.Run(test.name, func(t *testing.T) {
			residual := fixtureAccount()
			residual.ListFoldersResponse.ListFolders[0].Items = nil
			client := loadedClient(t, fixtureAccount(),
				acknowledgedStep("/data/list-folders/update", checkFolderDelete),
				requestStep{path: "/data/list-settings/update", check: checkSettingsDelete, status: test.status, delay: test.delay},
				readStep(t, residual),
				acknowledgedStep("/data/list-settings/update", checkSettingsDelete),
				readStep(t, removedAccount()),
			)
			if test.delay > 0 {
				client.http.Timeout = 10 * time.Millisecond
			}
			result, err := client.DeleteList(context.Background(), "list")
			_ = safeError(t, err, test.code, true)
			if result.ID != "list" || result.Outcome != "unknown" {
				t.Fatalf("partial delete lost target: %+v", result)
			}
			client.http.Timeout = time.Second
			lists, err := client.Lists(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(lists) != 0 {
				t.Fatal("orphaned list leaked into visible results")
			}
			result, err = client.DeleteList(context.Background(), "list")
			if err != nil {
				t.Fatal(err)
			}
			if result.Outcome != "removed" || result.ID != "list" {
				t.Fatal("residual delete failed")
			}
		})
	}
}

func TestLifecycleVerificationFailureIsUnknown(t *testing.T) {
	for _, action := range []string{"rename", "delete"} {
		t.Run(action, func(t *testing.T) {
			steps := []requestStep{acknowledgedStep("/data/shopping-lists/update", nil)}
			if action == "delete" {
				steps = []requestStep{acknowledgedStep("/data/list-folders/update", nil), acknowledgedStep("/data/list-settings/update", nil)}
			}
			steps = append(steps, readStep(t, fixtureAccount()))
			client := loadedClient(t, fixtureAccount(), steps...)
			var result MutationResult
			var err error
			if action == "rename" {
				result, err = client.RenameList(context.Background(), "list", "New")
			} else {
				result, err = client.DeleteList(context.Background(), "list")
			}
			_ = safeError(t, err, "verification", true)
			if result.ID != "list" || result.Outcome != "unknown" {
				t.Fatal("verification failure lost target/outcome")
			}
		})
	}
}

func TestShareListResponseValidation(t *testing.T) {
	for _, test := range []struct {
		name, outcome, code string
		response            *pb.PBShareListOperationResponse
		body                []byte
		unknown             bool
	}{
		{name: "confirmed", outcome: "shared", response: &pb.PBShareListOperationResponse{StatusCode: proto.Int32(0), SharedUser: &pb.PBEmailUserIDPair{Email: proto.String("friend@example.invalid"), UserId: proto.String("friend-id")}}},
		{name: "invitation", outcome: "invited", response: &pb.PBShareListOperationResponse{StatusCode: proto.Int32(0), SharedUser: &pb.PBEmailUserIDPair{Email: proto.String("friend@example.invalid")}}},
		{name: "application rejection", outcome: "failed", code: "operation_failed", response: &pb.PBShareListOperationResponse{StatusCode: proto.Int32(2), ErrorMessage: proto.String("secret upstream body")}},
		{name: "missing status", outcome: "unknown", code: "protocol", unknown: true, response: &pb.PBShareListOperationResponse{SharedUser: &pb.PBEmailUserIDPair{Email: proto.String("friend@example.invalid"), UserId: proto.String("friend-id")}}},
		{name: "conflicting status", outcome: "unknown", code: "protocol", unknown: true, response: &pb.PBShareListOperationResponse{StatusCode: proto.Int32(0), ErrorMessage: proto.String("secret upstream body"), SharedUser: &pb.PBEmailUserIDPair{Email: proto.String("friend@example.invalid"), UserId: proto.String("friend-id")}}},
		{name: "missing user", outcome: "unknown", code: "protocol", unknown: true, response: &pb.PBShareListOperationResponse{StatusCode: proto.Int32(0)}},
		{name: "wrong email", outcome: "unknown", code: "protocol", unknown: true, response: &pb.PBShareListOperationResponse{StatusCode: proto.Int32(0), SharedUser: &pb.PBEmailUserIDPair{Email: proto.String("other@example.invalid")}}},
		{name: "empty", outcome: "unknown", code: "protocol", unknown: true},
		{name: "malformed", outcome: "unknown", code: "protocol", unknown: true, body: []byte{0xff}},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := test.body
			if test.response != nil {
				body = marshal(t, test.response)
			}
			client := loadedClient(t, fixtureAccount(), requestStep{path: "/data/shopping-lists/share-list", response: body, check: func(r *http.Request) error {
				op, err := requestFields(r, "operation")
				if err != nil {
					return err
				}
				return errors.Join(
					exactFields(op, 1, 2, 4), checkMetadata(op, "share-shopping-list"),
					textField(op, 2, "list"), textField(op, 4, "friend@example.invalid"),
				)
			}})
			result, err := client.ShareList(context.Background(), "list", "friend@example.invalid")
			if test.code != "" {
				_ = safeError(t, err, test.code, test.unknown)
			} else if err != nil {
				t.Fatal(err)
			}
			if result.ID != "list" || result.Outcome != test.outcome {
				t.Fatalf("sharing result = %+v", result)
			}
		})
	}
}
