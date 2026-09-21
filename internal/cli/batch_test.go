package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/cosgroveb/al/internal/anylist"
)

func textPointer(value string) *string { return &value }

func TestReadBatchStrictInput(t *testing.T) {
	tests := []struct {
		name, operation, input string
		count                  int
		valid                  bool
	}{
		{"empty", "add", "[]", 0, true},
		{"names and IDs", "add", `[{"name":"milk","quantity":"2","notes":"","category":""},{"id":"item","quantity":"3"}]`, 2, true},
		{"duplicate creation", "add", `[{"name":"milk","new":true}]`, 1, true},
		{"edit rename", "edit", `[{"name":"milk","newName":"whole milk"}]`, 1, true},
		{"not array", "add", `{"name":"milk"}`, 0, false},
		{"null array", "add", `null`, 0, false},
		{"null record", "add", `[null]`, 0, false},
		{"null metadata", "add", `[{"name":"milk","notes":null}]`, 0, false},
		{"trailing document", "add", `[{"name":"milk"}] []`, 0, false},
		{"trailing garbage", "add", `[] nope`, 0, false},
		{"unterminated", "add", `[{"name":"milk"}`, 0, false},
		{"unknown field", "add", `[{"name":"milk","amount":"2"}]`, 0, false},
		{"wrong operation field", "check", `[{"name":"milk","quantity":"2"}]`, 0, false},
		{"wrong type", "add", `[{"name":"milk","quantity":2}]`, 0, false},
		{"new wrong type", "add", `[{"name":"milk","new":"yes"}]`, 0, false},
		{"both selectors", "add", `[{"name":"milk","id":"item"}]`, 0, false},
		{"no selectors", "add", `[{}]`, 0, false},
		{"empty name", "add", `[{"name":""}]`, 0, false},
		{"empty ID", "check", `[{"id":""}]`, 0, false},
		{"empty category ID", "add", `[{"name":"milk","categoryId":""}]`, 0, false},
		{"both categories", "add", `[{"name":"milk","category":"Dairy","categoryId":"dairy"}]`, 0, false},
		{"duplicate key", "add", `[{"name":"milk","name":"tea"}]`, 0, false},
		{"new with ID", "add", `[{"id":"item","new":false}]`, 0, false},
		{"empty rename", "edit", `[{"name":"milk","newName":""}]`, 0, false},
		{"edit without change", "edit", `[{"id":"item"}]`, 0, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			records, err := readBatch(strings.NewReader(test.input), test.operation)
			if test.valid {
				if err != nil {
					t.Fatal(err)
				}
				if len(records) != test.count {
					t.Fatalf("records=%d want%d", len(records), test.count)
				}
			} else {
				var input *anylist.Error
				if !errors.As(err, &input) || input.Code != "input" {
					t.Fatalf("expected input error, got %v", err)
				}
			}
		})
	}
}

func TestProjectedAddReuse(t *testing.T) {
	state := anylist.ListState{}
	records := []itemInput{{Name: textPointer("milk")}, {Name: textPointer("milk")}, {Name: textPointer("milk"), New: true, newSet: true}}
	plans, _, err := planBatch(state, records, "add")
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 3 || !plans[0].change.Create || plans[1].change != (anylist.ItemChange{ID: plans[0].item.ID}) || !plans[2].change.Create {
		t.Fatalf("creation and reuse: %+v", plans)
	}
	if plans[0].item.ID != plans[1].item.ID || plans[0].item.ID == plans[2].item.ID {
		t.Fatal("planned add reuse did not preserve distinct IDs")
	}
	state.Items = []anylist.Item{{ID: "existing", Name: "milk", Checked: true, Quantity: "2", Notes: "keep"}}
	plans, _, err = planBatch(state, records[:2], "add")
	if err != nil {
		t.Fatal(err)
	}
	if plans[0].item.ID != "existing" || plans[0].item.Checked || plans[0].item.Notes != "keep" || plans[0].change.Quantity != nil || plans[0].change.Checked == nil || *plans[0].change.Checked || plans[1].change != (anylist.ItemChange{ID: "existing"}) || plans[1].item != plans[0].item {
		t.Fatalf("checked reuse: %+v", plans)
	}
}

func TestProjectedRenameAndRemove(t *testing.T) {
	state := anylist.ListState{Items: []anylist.Item{{ID: "milk-id", Name: "milk"}}}
	plans, _, err := planBatch(state, []itemInput{{Name: textPointer("milk"), NewName: textPointer("whole milk")}, {Name: textPointer("whole milk"), Notes: textPointer("keep cold")}}, "edit")
	if err != nil {
		t.Fatal(err)
	}
	if plans[1].item.ID != "milk-id" || plans[1].item.Name != "whole milk" || plans[1].item.Notes != "keep cold" {
		t.Fatalf("projected rename: %+v", plans)
	}
	if state.Items[0].Name != "milk" {
		t.Fatal("planning changed input snapshot")
	}
	plans, failed, err := planBatch(state, []itemInput{{Name: textPointer("milk")}, {Name: textPointer("milk")}}, "remove")
	if err == nil || failed != 1 || len(plans) != 1 {
		t.Fatalf("remove-then-select: plans%d failed%d err%v", len(plans), failed, err)
	}
}

func TestPreflightRejectsAmbiguousCheckedAndUncheckedNames(t *testing.T) {
	state := anylist.ListState{Items: []anylist.Item{{ID: "one", Name: "milk"}, {ID: "two", Name: "milk", Checked: true}}}
	_, failed, err := planBatch(state, []itemInput{{Name: textPointer("milk")}}, "add")
	var selection *anylist.Error
	if failed != 0 || !errors.As(err, &selection) || selection.Code != "ambiguous" || len(selection.Candidates) != 2 {
		t.Fatalf("ambiguity: failed%d err%+v", failed, err)
	}
	plans, _, err := planBatch(state, []itemInput{{ID: textPointer("two")}}, "add")
	if err != nil || plans[0].item.Checked {
		t.Fatalf("ID selection failed: %v", err)
	}
}

func TestMetadataClearingPreservesOmissions(t *testing.T) {
	state := anylist.ListState{CategoryGroupID: "group", Categories: []anylist.Category{{ID: "cat", Name: "Dairy"}}, Items: []anylist.Item{{ID: "item", Name: "milk", Quantity: "2", Notes: "keep", CategoryID: "cat", CategoryName: "Dairy"}}}
	plans, _, err := planBatch(state, []itemInput{{ID: textPointer("item"), Notes: textPointer("")}, {ID: textPointer("item"), Quantity: textPointer(""), Category: textPointer("")}}, "edit")
	if err != nil {
		t.Fatal(err)
	}
	if plans[0].item.Quantity != "2" || plans[0].item.CategoryID != "cat" || plans[0].item.Notes != "" || plans[0].change.Quantity != nil {
		t.Fatalf("omissions: %+v", plans[0])
	}
	if plans[1].item.Quantity != "" || plans[1].item.CategoryID != "" || plans[1].item.CategoryName != "" || plans[1].change.CategoryID == nil {
		t.Fatalf("clearing: %+v", plans[1])
	}
}

func TestSelectorsAreExactAndReturnCandidates(t *testing.T) {
	lists := []anylist.List{{ID: "one", Name: "Groceries"}, {ID: "two", Name: "Groceries"}}
	if _, err := selectList(lists, "groceries", "", ""); err == nil {
		t.Fatal("case-insensitive name match")
	}
	_, err := selectList(lists, "Groceries", "", "")
	var selection *anylist.Error
	if !errors.As(err, &selection) || len(selection.Candidates) != 2 {
		t.Fatalf("list ambiguity: %v", err)
	}
	if list, err := selectList(lists, "", "", "two"); err != nil || list.ID != "two" {
		t.Fatalf("default ID: %v", err)
	}
	if _, err := selectList(lists, "", "", "missing"); err == nil {
		t.Fatal("stale default silently accepted")
	}
	categories := []anylist.Category{{ID: "one", Name: "Dairy"}, {ID: "two", Name: "Dairy"}}
	if _, err := selectCategory(categories, textPointer("Dairy"), nil); !errors.As(err, &selection) || len(selection.Candidates) != 2 {
		t.Fatalf("category ambiguity: %v", err)
	}
	if id, err := selectCategory(categories, nil, textPointer("two")); err != nil || *id != "two" {
		t.Fatalf("category ID: %v", err)
	}
}
