package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/cosgroveb/al/internal/anylist"
)

type itemInput struct {
	Name       *string
	ID         *string
	NewName    *string
	Quantity   *string
	Notes      *string
	Category   *string
	CategoryID *string
	New        bool
	newSet     bool
}

func readBatchContext(ctx context.Context, r io.Reader, operation string) ([]itemInput, error) {
	canceled := &anylist.Error{Code: "canceled", Message: "command canceled while reading stdin"}
	if ctx.Err() != nil {
		return nil, canceled
	}
	type result struct {
		records []itemInput
		err     error
	}
	done := make(chan result, 1)
	// Closing inherited stdin may not interrupt Read. Let the command return
	// while that read remains blocked until process exit or new input.
	go func() {
		records, err := readBatch(r, operation)
		done <- result{records: records, err: err}
	}()
	select {
	case <-ctx.Done():
		if input, ok := r.(io.Closer); ok {
			go func() { _ = input.Close() }()
		}
		return nil, canceled
	case result := <-done:
		if ctx.Err() != nil {
			return nil, canceled
		}
		return result.records, result.err
	}
}

func readBatch(r io.Reader, operation string) ([]itemInput, error) {
	decoder := json.NewDecoder(r)
	token, err := decoder.Token()
	if err != nil || token != json.Delim('[') {
		return nil, inputError("stdin must contain one JSON array")
	}
	records := []itemInput{}
	for decoder.More() {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, inputError(fmt.Sprintf("record %d: invalid JSON", len(records)))
		}
		record, err := decodeRecord(raw, operation)
		if err != nil {
			return nil, inputError(fmt.Sprintf("record %d: %s", len(records), err))
		}
		records = append(records, record)
	}
	if _, err := decoder.Token(); err != nil {
		return nil, inputError("stdin must contain a complete JSON array")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, inputError("stdin must contain exactly one JSON array with no trailing data")
	}
	return records, nil
}

func decodeRecord(raw []byte, operation string) (itemInput, error) {
	record := itemInput{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return record, inputError("each record must be an object")
	}
	seen := map[string]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return record, inputError("invalid JSON object")
		}
		key, ok := token.(string)
		if !ok {
			return record, inputError("invalid object field")
		}
		if seen[key] {
			return record, inputError("duplicate field in record")
		}
		seen[key] = true
		var rawValue json.RawMessage
		if err := decoder.Decode(&rawValue); err != nil {
			return record, inputError("invalid field value")
		}
		if bytes.Equal(bytes.TrimSpace(rawValue), []byte("null")) {
			return record, inputError("null field values are invalid; omit a field or use an empty string to clear metadata")
		}
		var field **string
		switch key {
		case "name":
			field = &record.Name
		case "id":
			field = &record.ID
		case "quantity":
			if operation == "add" || operation == "edit" {
				field = &record.Quantity
			}
		case "notes":
			if operation == "add" || operation == "edit" {
				field = &record.Notes
			}
		case "category":
			if operation == "add" || operation == "edit" {
				field = &record.Category
			}
		case "categoryId":
			if operation == "add" || operation == "edit" {
				field = &record.CategoryID
			}
		case "newName":
			if operation == "edit" {
				field = &record.NewName
			}
		case "new":
			if operation == "add" {
				if err := json.Unmarshal(rawValue, &record.New); err != nil {
					return record, inputError("new must be a boolean")
				}
				record.newSet = true
				continue
			}
		}
		if field == nil {
			return record, inputError("unknown or unsupported field for this operation")
		}
		var value string
		if err := json.Unmarshal(rawValue, &value); err != nil {
			return record, inputError("record fields must be strings, except new")
		}
		*field = &value
	}
	if _, err := decoder.Token(); err != nil {
		return record, inputError("invalid JSON object")
	}
	return record, validateItemInput(record, operation)
}

func validateItemInput(record itemInput, operation string) error {
	if (record.Name == nil) == (record.ID == nil) {
		return inputError("provide exactly one item name or ID")
	}
	if record.Name != nil && *record.Name == "" {
		return inputError("item name must not be empty")
	}
	if record.ID != nil && *record.ID == "" {
		return inputError("item ID must not be empty")
	}
	if record.NewName != nil && *record.NewName == "" {
		return inputError("new item name must not be empty")
	}
	if record.CategoryID != nil && *record.CategoryID == "" {
		return inputError("category ID must not be empty; use an empty category name to clear it")
	}
	if record.Category != nil && record.CategoryID != nil {
		return inputError("category and categoryId are mutually exclusive")
	}
	if record.newSet && record.ID != nil {
		return inputError("new and an item ID are mutually exclusive")
	}
	if operation == "edit" && record.NewName == nil && record.Quantity == nil && record.Notes == nil && record.Category == nil && record.CategoryID == nil {
		return inputError("edit requires --name, --quantity, --notes, or a category change")
	}
	return nil
}

type plannedItem struct {
	change anylist.ItemChange
	item   anylist.Item
}

// planBatch resolves every record against projected state without mutation requests.
func planBatch(state anylist.ListState, records []itemInput, operation string) ([]plannedItem, int, error) {
	items := append([]anylist.Item(nil), state.Items...)
	plans := make([]plannedItem, 0, len(records))
	for index, record := range records {
		plan, err := planItem(items, state.Categories, state.CategoryGroupID, record, operation)
		if err != nil {
			return plans, index, err
		}
		plans = append(plans, plan)
		found := -1
		for i := range items {
			if items[i].ID == plan.item.ID {
				found = i
				break
			}
		}
		if operation == "remove" {
			items = append(items[:found], items[found+1:]...)
		} else if found < 0 {
			items = append(items, plan.item)
		} else {
			items[found] = plan.item
		}
	}
	return plans, -1, nil
}

func planItem(items []anylist.Item, categories []anylist.Category, groupID string, record itemInput, operation string) (plannedItem, error) {
	var item anylist.Item
	var err error
	name, id := "", ""
	if record.Name != nil {
		name = *record.Name
	}
	if record.ID != nil {
		id = *record.ID
	}
	create := operation == "add" && record.New
	if !create {
		item, err = selectItem(items, name, id)
		if err != nil {
			var selectionError *anylist.Error
			if operation == "add" && id == "" && errors.As(err, &selectionError) && selectionError.Code == "not_found" {
				create = true
			} else {
				return plannedItem{}, err
			}
		}
	}
	if create {
		item = anylist.Item{ID: anylist.NewID(), Name: name}
	}
	plan := plannedItem{change: anylist.ItemChange{ID: item.ID, Create: create}, item: item}
	if create {
		plan.change.Name = &name
	}
	if operation == "remove" {
		plan.change.Remove = true
		return plan, nil
	}
	if operation == "check" || operation == "uncheck" || operation == "add" {
		checked := operation == "check"
		if item.Checked != checked {
			plan.change.Checked = &checked
			plan.item.Checked = checked
		}
	}
	if record.NewName != nil && *record.NewName != item.Name {
		plan.change.Name = record.NewName
		plan.item.Name = *record.NewName
	}
	if record.Quantity != nil {
		plan.change.Quantity = record.Quantity
		plan.item.Quantity = *record.Quantity
		plan.item.QuantityDetails = &anylist.Quantity{Raw: *record.Quantity}
	}
	if record.Notes != nil {
		plan.change.Notes = record.Notes
		plan.item.Notes = *record.Notes
	}
	categoryID, err := selectCategory(categories, record.Category, record.CategoryID)
	if err != nil {
		return plannedItem{}, err
	}
	if categoryID != nil {
		if groupID == "" {
			return plannedItem{}, &anylist.Error{Code: "not_found", Message: "list has no active category group"}
		}
		plan.change.CategoryID = categoryID
		plan.item.CategoryID = *categoryID
		plan.item.CategoryName = ""
		for _, category := range categories {
			if category.ID == *categoryID {
				plan.item.CategoryName = category.Name
				break
			}
		}
	}
	return plan, nil
}

func executeBatch(ctx context.Context, client *anylist.Client, listID string, plans []plannedItem) ([]anylist.MutationResult, error) {
	results := make([]anylist.MutationResult, len(plans))
	for i, plan := range plans {
		index := i
		results[i] = anylist.MutationResult{Index: &index, ID: plan.item.ID, Outcome: "skipped"}
	}
	for i, plan := range plans {
		result, err := client.ApplyItem(ctx, listID, plan.change)
		result.Index = results[i].Index
		if err != nil {
			results[i] = result
			return results, err
		}
		results[i] = result
	}
	return results, nil
}

func preflightResults(records []itemInput, plans []plannedItem, failed int) []anylist.MutationResult {
	results := make([]anylist.MutationResult, len(records))
	for i, record := range records {
		index := i
		results[i] = anylist.MutationResult{Index: &index, Outcome: "skipped"}
		if record.ID != nil {
			results[i].ID = *record.ID
		}
		if i < len(plans) {
			results[i].ID = plans[i].item.ID
		}
		if i == failed {
			results[i].Outcome = "failed"
		}
	}
	return results
}
