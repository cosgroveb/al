package anylist

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	pb "github.com/cosgroveb/al/internal/anylistpb"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

func TestItemScalarRequests(t *testing.T) {
	for _, test := range []struct {
		name, handler, value string
		change               ItemChange
		original             *string
		initialChecked       bool
	}{
		{"rename", "set-list-item-name", "Oat milk", ItemChange{ID: "item", Name: proto.String("Oat milk")}, proto.String("Milk"), true},
		{"clear notes", "set-list-item-details", "", ItemChange{ID: "item", Notes: proto.String("")}, proto.String("whole"), true},
		{"check", "set-list-item-checked", "y", ItemChange{ID: "item", Checked: proto.Bool(true)}, nil, false},
		{"uncheck", "set-list-item-checked", "n", ItemChange{ID: "item", Checked: proto.Bool(false)}, nil, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			account := fixtureAccount()
			account.ShoppingListsResponse.NewLists[0].Items[0].Checked = proto.Bool(test.initialChecked)
			client := loadedClient(t, account, acknowledgedStep("/data/shopping-lists/update", func(r *http.Request) error {
				op, err := operation(r, test.handler, "list", "item")
				if err != nil {
					return err
				}
				if err := textField(op, 4, test.value); err != nil {
					return err
				}
				if test.original != nil {
					return errors.Join(exactFields(op, 1, 2, 3, 4, 5), textField(op, 5, *test.original))
				}
				return exactFields(op, 1, 2, 3, 4)
			}))
			result, err := client.ApplyItem(context.Background(), "list", test.change)
			if err != nil {
				t.Fatal(err)
			}
			if result.Outcome != "updated" || result.ID != "item" {
				t.Errorf("result = %+v", result)
			}
			if result.Item.Quantity != "two cartons" || result.Item.CategoryID != "dairy-id" {
				t.Fatal("scalar update changed unrelated metadata")
			}
		})
	}
}

func TestQuantityReplacementRequests(t *testing.T) {
	for _, quantity := range []string{"3 jars", ""} {
		t.Run("quantity="+quantity, func(t *testing.T) {
			client := loadedClient(t, fixtureAccount(), acknowledgedStep("/data/shopping-lists/update", func(r *http.Request) error {
				op, err := operation(r, "set-list-item-quantity-v2", "list", "item")
				if err != nil {
					return err
				}
				item, err := nestedFields(op, 6)
				if err != nil {
					return err
				}
				q, err := nestedFields(item, 21)
				if err != nil {
					return err
				}
				return errors.Join(
					exactFields(op, 1, 2, 3, 6), exactFields(item, 1, 3, 18, 21),
					textField(item, 1, "item"), textField(item, 3, "list"), textField(item, 18, ""),
					exactFields(q, 3), textField(q, 3, quantity),
				)
			}))
			result, err := client.ApplyItem(context.Background(), "list", ItemChange{ID: "item", Quantity: proto.String(quantity)})
			if err != nil {
				t.Fatal(err)
			}
			if result.Item.Quantity != quantity || result.Item.QuantityDetails.Amount != "" || result.Item.QuantityDetails.Unit != "" {
				t.Fatalf("quantity did not replace old amount/unit: %+v", result.Item)
			}
			if result.Item.Notes != "whole" || result.Item.Name != "Milk" {
				t.Fatal("quantity update changed unrelated fields")
			}
		})
	}
}

func TestLegacyCategoryUnicodeNormalization(t *testing.T) {
	for _, test := range []struct {
		name, match string
	}{
		{"ΚΡΕΑΣ", "κρεας"},
		{"ΣΟΥΣΙ", "σουσι"},
		{"ΟΣ\u0301", "ος"},
		{"  İSTANBUL & Dairy!  ", "istanbul-and-dairy"},
		{"\u0085 Dairy", "-dairy"},
		{"Dairy \u0085", "dairy-"},
		{"\ufeff Dairy", "dairy"},
		{"A𐐀B", "ab"},
		{"Dairy 𝟘", "dairy-"},
		{"A𠀀B", "ab"},
		{"A\u08b3B", "ab"},
		{"A\u180eB", "a-b"},
		{"A\u0345B", "a\u0345b"},
		{"AΣ𐐀", "aσ"},
		{"𐐀Σ", "ς"},
		{"AΣ" + strings.Repeat(".", 31) + "B", "aσb"},
		{"AΣ" + strings.Repeat("\u0301", 31) + "B", "aσb"},
		{"A" + strings.Repeat(".", 1024) + "Σ", "aς"},
		{"A" + strings.Repeat("\u0301", 1024) + "Σ", "aς"},
		{"A\ua7cbB", "aɤb"},
		{"A\ua7dcB", "aƛb"},
		{"AΣ\u0897B", "aσb"},
		{"A\u0897Σ", "aς"},
		{"AΣ\u0295B", "aςʕb"},
		{"A\u0295Σ", "aʕσ"},
		{"AΣ\u1c89", "aσ"},
		{"\u1c89Σ", "ς"},
		{"AΣ\u0345", "aς\u0345"},
		{"\u0345Σ", "\u0345σ"},
		{"AΣ\U0001e030", "aς"},
		{"\U0001e030Σ", "σ"},
	} {
		t.Run(test.name, func(t *testing.T) {
			account := fixtureAccount()
			account.ShoppingListsResponse.NewLists[0].Items[0].CategoryMatchId = proto.String(test.match)
			account.ShoppingListsResponse.ListResponses[0].CategoryGroupResponses[0].CategoryGroup.Categories = []*pb.PBListCategory{
				{Identifier: proto.String("category-id"), Name: proto.String(test.name)},
			}
			client := loadedClient(t, account)
			state, err := client.ListState("list")
			if err != nil {
				t.Fatal(err)
			}
			if item := state.Items[0]; item.CategoryID != "category-id" || item.CategoryName != test.name {
				t.Fatalf("legacy category %q resolved to %q (%q), want category-id (%q)", test.match, item.CategoryID, item.CategoryName, test.name)
			}
		})
	}
}

func TestCategoryAssignmentPreservesOtherFields(t *testing.T) {
	for _, test := range []struct {
		name, categoryID string
		existing         bool
	}{
		{"append category", "produce-id", false},
		{"append clearing", "", false},
		{"replace category", "produce-id", true},
		{"clear category", "", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			categoryID := test.categoryID
			account := fixtureAccount()
			item := account.ShoppingListsResponse.NewLists[0].Items[0]
			unknown := protowire.AppendString(protowire.AppendTag(nil, 30, protowire.BytesType), "upc")
			item.ProtoReflect().SetUnknown(unknown)
			otherAssignment := &pb.PBListItemCategoryAssignment{Identifier: proto.String("other-assignment"), CategoryGroupId: proto.String("other-group"), CategoryId: proto.String("other-category")}
			otherAssignment.ProtoReflect().SetUnknown(protowire.AppendString(protowire.AppendTag(nil, 8, protowire.BytesType), "future assignment field"))
			item.CategoryAssignments = []*pb.PBListItemCategoryAssignment{otherAssignment}
			if test.existing {
				item.CategoryAssignments = append(item.CategoryAssignments, &pb.PBListItemCategoryAssignment{
					Identifier:      proto.String("93994a3ca5f257079d3067ed79cbb395"),
					CategoryGroupId: proto.String("group"), CategoryId: proto.String("dairy-id"),
				})
			}
			otherAssignmentData := marshal(t, otherAssignment)
			client := loadedClient(t, account, acknowledgedStep("/data/shopping-lists/update", func(r *http.Request) error {
				op, err := operation(r, "update-list-item-category-assignment", "list", "item")
				if err != nil {
					return err
				}
				wireItem, err := nestedFields(op, 6)
				if err != nil {
					return err
				}
				if len(wireItem[20]) != 2 {
					return errors.New("expected two category assignments")
				}
				if !bytes.Equal(wireItem[20][0], otherAssignmentData) {
					return errors.New("other group's assignment changed")
				}
				assignment, err := fields(wireItem[20][1])
				if err != nil {
					return err
				}
				return errors.Join(
					exactFields(op, 1, 2, 3, 6), textField(wireItem, 4, "Milk"), textField(wireItem, 5, "whole"),
					textField(wireItem, 11, "dairy"), textField(wireItem, 13, "dairy"), textField(wireItem, 30, "upc"),
					exactFields(assignment, 1, 2, 3), textField(assignment, 1, "93994a3ca5f257079d3067ed79cbb395"),
					textField(assignment, 2, "group"), textField(assignment, 3, categoryID),
				)
			}))
			result, err := client.ApplyItem(context.Background(), "list", ItemChange{ID: "item", CategoryID: proto.String(categoryID)})
			if err != nil {
				t.Fatal(err)
			}
			if result.Outcome != "updated" || result.Item == nil || result.Item.CategoryID != categoryID {
				t.Fatalf("category update = %+v, want updated with category %q", result, categoryID)
			}
			assignments := client.lists["list"].Items[0].CategoryAssignments
			if len(assignments) != 2 || assignments[1].GetIdentifier() != "93994a3ca5f257079d3067ed79cbb395" || assignments[1].GetCategoryId() != categoryID {
				t.Fatalf("cached category assignments = %v", assignments)
			}
			fallbackGroup := &pb.PBListCategoryGroup{Identifier: proto.String("fallback-group"), Categories: []*pb.PBListCategory{{Identifier: proto.String("fallback-dairy"), SystemCategory: proto.String("dairy")}}}
			if itemCategoryID(client.lists["list"].Items[0], fallbackGroup) != "fallback-dairy" {
				t.Fatal("active-group assignment changed another group's legacy fallback")
			}
		})
	}
}

func TestDeterministicCategoryAssignmentID(t *testing.T) {
	// Public fixture from anylist_rs 0698dc9, operations.rs:811-814.
	if got := assignmentID("65564675a0de5a5fa6a69272df260fcc"); got != "47868d70669a5a078f8bc4e40dc07cab" {
		t.Errorf("UUIDv5 = %s", got)
	}
	// Independently computed with Python's standard uuid.uuid5 implementation.
	if got := defaultGroupID("list"); got != "26f2b12d17ba5143a5271e05fc4bd579" {
		t.Errorf("default group UUIDv5 = %s", got)
	}
}

func TestAddAndRemoveItemRequests(t *testing.T) {
	account := fixtureAccount()
	account.ShoppingListsResponse.NewLists[0].NewListItemPosition = proto.Int32(1)
	client := loadedClient(t, account,
		acknowledgedStep("/data/shopping-lists/update", func(r *http.Request) error {
			op, err := operation(r, "add-shopping-list-item", "list", "new-item")
			if err != nil {
				return err
			}
			item, err := nestedFields(op, 6)
			if err != nil {
				return err
			}
			quantity, err := nestedFields(item, 21)
			if err != nil {
				return err
			}
			assignment, err := nestedFields(item, 20)
			if err != nil {
				return err
			}
			list, err := nestedFields(op, 7)
			if err != nil {
				return err
			}
			return errors.Join(
				exactFields(op, 1, 2, 3, 6, 7), textField(item, 1, "new-item"), textField(item, 3, "list"),
				textField(item, 4, "Apples"), textField(item, 5, "green"), textField(item, 12, "test-user"),
				bytesField(item, 6, []byte{0}), textField(item, 18, ""), textField(quantity, 3, "three"),
				textField(assignment, 2, "group"), textField(assignment, 3, "produce-id"),
				exactFields(list, 1, 18), bytesField(list, 18, []byte{1}),
			)
		}),
		acknowledgedStep("/data/shopping-lists/update", func(r *http.Request) error {
			op, err := operation(r, "remove-shopping-list-item", "list", "new-item")
			if err != nil {
				return err
			}
			item, err := nestedFields(op, 6)
			if err != nil {
				return err
			}
			return errors.Join(exactFields(op, 1, 2, 3, 6), textField(item, 4, "Apples"))
		}),
	)
	result, err := client.ApplyItem(context.Background(), "list", ItemChange{ID: "new-item", Create: true, Name: proto.String("Apples"), Quantity: proto.String("three"), Notes: proto.String("green"), CategoryID: proto.String("produce-id")})
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "new-item" || result.Outcome != "added" {
		t.Fatalf("add result = %+v", result)
	}
	state, err := client.ListState("list")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Items) != 2 || state.Items[0].ID != "new-item" {
		t.Fatal("successful add not reflected in cache")
	}
	result, err = client.ApplyItem(context.Background(), "list", ItemChange{ID: "new-item", Remove: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != "removed" {
		t.Fatal("missing remove outcome")
	}
	state, err = client.ListState("list")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Items) != 1 || state.Items[0].ID != "item" {
		t.Fatal("successful remove not reflected in cache")
	}
}

func TestUnchangedItemDoesNotWrite(t *testing.T) {
	client := loadedClient(t, fixtureAccount())
	for _, change := range []ItemChange{{ID: "item"}, {ID: "item", Name: proto.String("Milk"), Notes: proto.String("whole"), Checked: proto.Bool(true)}} {
		result, err := client.ApplyItem(context.Background(), "list", change)
		if err != nil {
			t.Fatal(err)
		}
		if result.Outcome != "unchanged" {
			t.Errorf("outcome = %s", result.Outcome)
		}
	}
}

func TestCompoundItemFailurePreservesOnlySuccessfulFields(t *testing.T) {
	client := loadedClient(t, fixtureAccount(),
		acknowledgedStep("/data/shopping-lists/update", func(r *http.Request) error {
			_, err := operation(r, "set-list-item-name", "list", "item")
			return err
		}),
		requestStep{path: "/data/shopping-lists/update", status: 403, check: func(r *http.Request) error {
			_, err := operation(r, "set-list-item-details", "list", "item")
			return err
		}},
	)
	result, err := client.ApplyItem(context.Background(), "list", ItemChange{ID: "item", Name: proto.String("Oat milk"), Notes: proto.String("unsweetened"), Checked: proto.Bool(false)})
	_ = safeError(t, err, "permission", true)
	if result.Outcome != "unknown" || result.ID != "item" {
		t.Errorf("partial result = %+v", result)
	}
	state, err := client.ListState("list")
	if err != nil {
		t.Fatal(err)
	}
	if state.Items[0].Name != "Oat milk" || state.Items[0].Notes != "whole" || !state.Items[0].Checked {
		t.Fatalf("cache = %+v", state.Items[0])
	}
}

func TestCompoundCategoryWriteIncludesPriorChanges(t *testing.T) {
	client := loadedClient(t, fixtureAccount(),
		acknowledgedStep("/data/shopping-lists/update", func(r *http.Request) error {
			_, err := operation(r, "set-list-item-name", "list", "item")
			return err
		}),
		acknowledgedStep("/data/shopping-lists/update", func(r *http.Request) error {
			op, err := operation(r, "update-list-item-category-assignment", "list", "item")
			if err != nil {
				return err
			}
			item, err := nestedFields(op, 6)
			if err != nil {
				return err
			}
			return textField(item, 4, "Apple milk")
		}),
	)
	result, err := client.ApplyItem(context.Background(), "list", ItemChange{ID: "item", Name: proto.String("Apple milk"), CategoryID: proto.String("produce-id")})
	if err != nil {
		t.Fatal(err)
	}
	if result.Item.Name != "Apple milk" || result.Item.CategoryID != "produce-id" {
		t.Fatalf("result = %+v", result)
	}
}
