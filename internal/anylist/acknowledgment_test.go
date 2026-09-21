package anylist

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

func invalidAcknowledgments() []struct {
	name string
	body []byte
} {
	return []struct {
		name string
		body []byte
	}{
		{name: "empty"},
		{name: "malformed", body: []byte{0xff}},
		{name: "missing operation IDs", body: []byte{0x0a, 0}},
		{name: "different operation", body: protowire.AppendString(protowire.AppendTag(nil, 3, protowire.BytesType), "different-operation-id")},
	}
}

func TestItemAcknowledgmentFailurePreservesCacheAndStopsWrites(t *testing.T) {
	for _, acknowledgment := range invalidAcknowledgments() {
		for _, test := range []struct {
			name   string
			change ItemChange
		}{
			{"compound edit", ItemChange{ID: "item", Name: proto.String("Changed"), Notes: proto.String("Next write")}},
			{"add", ItemChange{ID: "new-item", Create: true, Name: proto.String("New")}},
			{"remove", ItemChange{ID: "item", Remove: true}},
			{"category", ItemChange{ID: "item", CategoryID: proto.String("")}},
			{"quantity", ItemChange{ID: "item", Quantity: proto.String("")}},
		} {
			t.Run(acknowledgment.name+"/"+test.name, func(t *testing.T) {
				client := loadedClient(t, fixtureAccount(), requestStep{path: "/data/shopping-lists/update", response: acknowledgment.body})
				before, err := client.ListState("list")
				if err != nil {
					t.Fatal(err)
				}
				result, err := client.ApplyItem(context.Background(), "list", test.change)
				_ = safeError(t, err, "protocol", true)
				if result.Outcome != "unknown" || result.ID != test.change.ID {
					t.Fatalf("result = %+v", result)
				}
				after, err := client.ListState("list")
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(before, after) {
					t.Fatal("unacknowledged operation changed cached item state")
				}
			})
		}
	}
}

func TestCompoundAcknowledgmentFailureKeepsOnlyAcknowledgedChanges(t *testing.T) {
	client := loadedClient(t, fixtureAccount(),
		acknowledgedStep("/data/shopping-lists/update", nil),
		requestStep{path: "/data/shopping-lists/update"},
	)
	result, err := client.ApplyItem(context.Background(), "list", ItemChange{ID: "item", Name: proto.String("Changed"), Notes: proto.String("Unconfirmed"), Checked: proto.Bool(false)})
	_ = safeError(t, err, "protocol", true)
	if result.Outcome != "unknown" || result.ID != "item" {
		t.Fatalf("result = %+v", result)
	}
	state, err := client.ListState("list")
	if err != nil {
		t.Fatal(err)
	}
	if state.Items[0].Name != "Changed" || state.Items[0].Notes != "whole" || !state.Items[0].Checked {
		t.Fatalf("cache = %+v", state.Items[0])
	}
}

func TestCreateAcknowledgmentFailureRetainsIDAndStopsLifecycle(t *testing.T) {
	for _, acknowledgment := range invalidAcknowledgments() {
		for _, failedStep := range []string{"create", "settings"} {
			t.Run(acknowledgment.name+"/"+failedStep, func(t *testing.T) {
				ids := make(chan string, 1)
				captureID := func(r *http.Request) error {
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
					ids <- string(id)
					return nil
				}
				steps := []requestStep{{path: "/data/shopping-lists/update", check: captureID, response: acknowledgment.body}}
				if failedStep == "settings" {
					steps = []requestStep{acknowledgedStep("/data/shopping-lists/update", captureID), {path: "/data/list-settings/update", response: acknowledgment.body}}
				}
				client := loadedClient(t, fixtureAccount(), steps...)
				result, err := client.CreateList(context.Background(), "New")
				_ = safeError(t, err, "protocol", true)
				var submittedID string
				select {
				case submittedID = <-ids:
				default:
					t.Fatal("create request did not record its list ID")
				}
				if result.Outcome != "unknown" || result.ID != submittedID {
					t.Fatalf("create lost attempted target: %+v", result)
				}
				if client.settings[result.ID] != nil {
					t.Fatal("unacknowledged settings operation advanced the cache")
				}
			})
		}
	}
}

func TestDeleteAcknowledgmentFailurePreservesResidualState(t *testing.T) {
	for _, acknowledgment := range invalidAcknowledgments() {
		for _, failedStep := range []string{"folder", "settings"} {
			t.Run(acknowledgment.name+"/"+failedStep, func(t *testing.T) {
				steps := []requestStep{{path: "/data/list-folders/update", check: checkFolderDelete, response: acknowledgment.body}}
				if failedStep == "settings" {
					steps = []requestStep{acknowledgedStep("/data/list-folders/update", checkFolderDelete), {path: "/data/list-settings/update", check: checkSettingsDelete, response: acknowledgment.body}}
				}
				client := loadedClient(t, fixtureAccount(), steps...)
				result, err := client.DeleteList(context.Background(), "list")
				_ = safeError(t, err, "protocol", true)
				if result.Outcome != "unknown" || result.ID != "list" {
					t.Fatalf("delete lost target: %+v", result)
				}
				_, visible := client.parents["list"]
				if visible != (failedStep == "folder") {
					t.Fatal("folder membership advanced without acknowledgment")
				}
				if client.settings["list"] == nil {
					t.Fatal("unacknowledged settings removal advanced the cache")
				}
			})
		}
	}
}

func TestRenameAcknowledgmentFailureDoesNotAdvanceNameOrRead(t *testing.T) {
	client := loadedClient(t, fixtureAccount(), requestStep{path: "/data/shopping-lists/update"})
	result, err := client.RenameList(context.Background(), "list", "Unconfirmed")
	_ = safeError(t, err, "protocol", true)
	if result.Outcome != "unknown" || result.ID != "list" {
		t.Fatalf("result = %+v", result)
	}
	state, err := client.ListState("list")
	if err != nil {
		t.Fatal(err)
	}
	if state.Name != "Groceries" {
		t.Fatal("unacknowledged rename changed cached list name")
	}
}
