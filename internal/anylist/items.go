package anylist

import (
	"context"
	"sort"
	"strings"
	"unicode"

	pb "github.com/cosgroveb/al/internal/anylistpb"
	"google.golang.org/protobuf/proto"
)

func defaultGroupID(listID string) string {
	return derivedID("f656a81f0e0a419aa45121f4f2eac51b", listID)
}

func assignmentID(groupID string) string {
	return derivedID("08e5c5bdcd694454a1ffd611b6d9abc0", groupID)
}

func (c *Client) activeGroup(listID string) (*pb.PBListCategoryGroup, error) {
	groups := c.groups[listID]
	byID := make(map[string]*pb.PBListCategoryGroup, len(groups))
	for _, group := range groups {
		byID[group.GetIdentifier()] = group
	}
	settings := c.settings[listID]
	if filterID := settings.GetStoreFilterId(); filterID != "" {
		for _, filter := range c.filters[listID] {
			if filter.GetIdentifier() == filterID && byID[filter.GetListCategoryGroupId()] != nil {
				return byID[filter.GetListCategoryGroupId()], nil
			}
		}
	}
	if group := byID[settings.GetListCategoryGroupId()]; group != nil {
		return group, nil
	}
	if group := byID[defaultGroupID(listID)]; group != nil {
		return group, nil
	}
	if len(groups) == 0 {
		return nil, nil
	}
	if len(groups) == 1 {
		return groups[0], nil
	}
	candidates := make([]Candidate, 0, len(groups))
	for _, group := range groups {
		candidates = append(candidates, Candidate{ID: group.GetIdentifier(), Name: group.GetName()})
	}
	return nil, &Error{Code: "category_ambiguous", Message: "active category group is ambiguous; select a category set in AnyList", Candidates: candidates}
}

// ListState resolves categories only for the selected account-visible list.
func (c *Client) ListState(id string) (ListState, error) {
	list, err := c.visibleList(id)
	if err != nil {
		return ListState{}, err
	}
	group, err := c.activeGroup(id)
	if err != nil {
		return ListState{}, err
	}
	state := ListState{List: listValue(list), Items: make([]Item, 0, len(list.Items)),
		Categories: make([]Category, 0, len(group.GetCategories())), CategoryGroupID: group.GetIdentifier()}
	categories := append([]*pb.PBListCategory(nil), group.GetCategories()...)
	sort.SliceStable(categories, func(i, j int) bool { return categories[i].GetSortIndex() < categories[j].GetSortIndex() })
	for _, category := range categories {
		state.Categories = append(state.Categories, Category{ID: category.GetIdentifier(), Name: category.GetName()})
	}
	for _, item := range list.Items {
		state.Items = append(state.Items, itemValue(item, group))
	}
	return state, nil
}

func itemValue(item *pb.ListItem, group *pb.PBListCategoryGroup) Item {
	value := Item{ID: item.GetIdentifier(), Name: item.GetName(), Checked: item.GetChecked(), Notes: item.GetDetails()}
	if quantity := item.QuantityPb; quantity != nil {
		value.QuantityDetails = &Quantity{Amount: quantity.GetAmount(), Unit: quantity.GetUnit(), Raw: quantity.GetRawQuantity()}
		value.Quantity = quantity.GetRawQuantity()
		if value.Quantity == "" {
			value.Quantity = strings.TrimSpace(quantity.GetAmount() + " " + quantity.GetUnit())
		}
	} else {
		value.Quantity = item.GetDeprecatedQuantity()
	}
	value.CategoryID = itemCategoryID(item, group)
	for _, category := range group.GetCategories() {
		if category.GetIdentifier() == value.CategoryID {
			value.CategoryName = category.GetName()
			break
		}
	}
	return value
}

func itemCategoryID(item *pb.ListItem, group *pb.PBListCategoryGroup) string {
	if group == nil {
		return ""
	}
	var assigned *string
	for _, assignment := range item.CategoryAssignments {
		if assignment.GetCategoryGroupId() == group.GetIdentifier() {
			assigned = assignment.CategoryId
		}
	}
	if assigned != nil {
		return *assigned
	}
	match := item.GetCategoryMatchId()
	if match == "" || match == "other" {
		return ""
	}
	for _, category := range group.Categories {
		categoryMatch := category.GetSystemCategory()
		if categoryMatch == "" {
			categoryMatch = categoryMatchID(category.GetName())
		}
		if match == categoryMatch {
			return category.GetIdentifier()
		}
	}
	return ""
}

func categoryMatchID(name string) string {
	name = categoryLower(name)
	// ECMAScript trim includes BOM but excludes NEL and U+180E.
	name = strings.TrimFunc(name, func(r rune) bool {
		switch r {
		case '\t', '\v', '\f', '\ufeff', '\n', '\r', '\u2028', '\u2029':
			return true
		}
		return unicode.Is(unicode.Zs, r)
	})
	name = strings.Replace(name, "&", " and ", 1)
	var result strings.Builder
	separator := false
	for _, r := range name {
		if unicode.Is(categorySeparators, r) {
			if !separator {
				result.WriteByte('-')
			}
			separator = true
		} else if unicode.Is(categoryLettersAndDigits, r) {
			result.WriteRune(r)
			separator = false
		}
	}
	return result.String()
}

func setAssignment(item *pb.ListItem, groupID, categoryID string) {
	id := assignmentID(groupID)
	for _, assignment := range item.CategoryAssignments {
		if assignment.GetIdentifier() == id {
			assignment.CategoryGroupId = proto.String(groupID)
			assignment.CategoryId = proto.String(categoryID)
			return
		}
	}
	item.CategoryAssignments = append(item.CategoryAssignments, &pb.PBListItemCategoryAssignment{
		Identifier: proto.String(id), CategoryGroupId: proto.String(groupID), CategoryId: proto.String(categoryID)})
}

func (c *Client) ApplyItem(ctx context.Context, listID string, change ItemChange) (MutationResult, error) {
	result := MutationResult{ID: change.ID}
	list, err := c.visibleList(listID)
	if err != nil {
		return failedMutation(result, err, false)
	}
	if change.ID == "" || change.Create && change.Remove {
		return failedMutation(result, &Error{Code: "input", Message: "invalid item change"}, false)
	}
	group, err := c.activeGroup(listID)
	if err != nil {
		return failedMutation(result, err, false)
	}
	if change.CategoryID != nil {
		if group == nil {
			return failedMutation(result, &Error{Code: "not_found", Message: "list has no active category group"}, false)
		}
		found := *change.CategoryID == ""
		for _, category := range group.Categories {
			if category.GetIdentifier() == *change.CategoryID {
				found = true
			}
		}
		if !found {
			return failedMutation(result, &Error{Code: "not_found", Message: "category is missing from the active group"}, false)
		}
	}
	index := -1
	for i, item := range list.Items {
		if item.GetIdentifier() == change.ID {
			index = i
			break
		}
	}
	if change.Create {
		if index >= 0 {
			return failedMutation(result, &Error{Code: "conflict", Message: "item identifier already exists"}, false)
		}
		if change.Name == nil || *change.Name == "" {
			return failedMutation(result, &Error{Code: "input", Message: "item name cannot be empty"}, false)
		}
		item := &pb.ListItem{Identifier: proto.String(change.ID), ListId: proto.String(listID), Name: proto.String(*change.Name),
			Checked: proto.Bool(false), UserId: proto.String(c.userID)}
		if change.Checked != nil {
			item.Checked = proto.Bool(*change.Checked)
		}
		if change.Notes != nil {
			item.Details = proto.String(*change.Notes)
		}
		if change.Quantity != nil {
			item.QuantityPb = &pb.PBItemQuantity{RawQuantity: proto.String(*change.Quantity)}
			item.DeprecatedQuantity = proto.String("")
		}
		if change.CategoryID != nil {
			setAssignment(item, group.GetIdentifier(), *change.CategoryID)
		}
		meta := c.metadata("add-shopping-list-item")
		op := &pb.PBListOperation{Metadata: meta, ListId: proto.String(listID), ListItemId: proto.String(change.ID), ListItem: item}
		if list.GetNewListItemPosition() == int32(pb.ShoppingList_Top) {
			op.List = &pb.ShoppingList{Identifier: proto.String(listID), NewListItemPosition: proto.Int32(int32(pb.ShoppingList_Top))}
		}
		if err = c.updateList(ctx, op); err != nil {
			return failedMutation(result, err, false)
		}
		if list.GetNewListItemPosition() == int32(pb.ShoppingList_Top) {
			list.Items = append([]*pb.ListItem{item}, list.Items...)
		} else {
			list.Items = append(list.Items, item)
		}
		value := itemValue(item, group)
		result.Item, result.Outcome = &value, "added"
		return result, nil
	}
	if index < 0 {
		return failedMutation(result, &Error{Code: "not_found", Message: "item is missing or inaccessible"}, false)
	}
	original := list.Items[index]
	if change.Remove {
		meta := c.metadata("remove-shopping-list-item")
		if err = c.updateList(ctx, &pb.PBListOperation{Metadata: meta, ListId: proto.String(listID), ListItemId: proto.String(change.ID),
			ListItem: proto.Clone(original).(*pb.ListItem)}); err != nil {
			return failedMutation(result, err, false)
		}
		list.Items = append(list.Items[:index], list.Items[index+1:]...)
		result.Outcome = "removed"
		return result, nil
	}
	if change.Name != nil && *change.Name == "" {
		return failedMutation(result, &Error{Code: "input", Message: "item name cannot be empty"}, false)
	}

	// Build the complete sequence before sending. Each snapshot includes earlier
	// field changes, so a later full-item category operation cannot undo them.
	type edit struct {
		operation *pb.PBListOperation
		item      *pb.ListItem
	}
	edits := make([]edit, 0, 5)
	current := proto.Clone(original).(*pb.ListItem)
	appendEdit := func(handler string, op *pb.PBListOperation) {
		op.Metadata, op.ListId, op.ListItemId = c.metadata(handler), proto.String(listID), proto.String(change.ID)
		edits = append(edits, edit{operation: op, item: proto.Clone(current).(*pb.ListItem)})
	}
	if change.Name != nil && current.GetName() != *change.Name {
		old := current.GetName()
		current.Name = proto.String(*change.Name)
		appendEdit("set-list-item-name", &pb.PBListOperation{UpdatedValue: current.Name, OriginalValue: proto.String(old)})
	}
	if change.Quantity != nil {
		quantity := &pb.PBItemQuantity{RawQuantity: proto.String(*change.Quantity)}
		if !proto.Equal(current.QuantityPb, quantity) || current.GetDeprecatedQuantity() != "" {
			current.QuantityPb, current.DeprecatedQuantity = quantity, proto.String("")
			partial := &pb.ListItem{Identifier: proto.String(change.ID), ListId: proto.String(listID), QuantityPb: quantity, DeprecatedQuantity: proto.String("")}
			appendEdit("set-list-item-quantity-v2", &pb.PBListOperation{ListItem: partial})
		}
	}
	if change.Notes != nil && current.GetDetails() != *change.Notes {
		old := current.GetDetails()
		current.Details = proto.String(*change.Notes)
		appendEdit("set-list-item-details", &pb.PBListOperation{UpdatedValue: current.Details, OriginalValue: proto.String(old)})
	}
	if change.Checked != nil && current.GetChecked() != *change.Checked {
		current.Checked = proto.Bool(*change.Checked)
		value := "n"
		if *change.Checked {
			value = "y"
		}
		appendEdit("set-list-item-checked", &pb.PBListOperation{UpdatedValue: proto.String(value)})
	}
	if change.CategoryID != nil {
		before := proto.Clone(current).(*pb.ListItem)
		setAssignment(current, group.GetIdentifier(), *change.CategoryID)
		if !proto.Equal(before, current) {
			appendEdit("update-list-item-category-assignment", &pb.PBListOperation{ListItem: proto.Clone(current).(*pb.ListItem)})
		}
	}
	for i, edit := range edits {
		if err = c.updateList(ctx, edit.operation); err != nil {
			return failedMutation(result, err, i > 0)
		}
		list.Items[index] = edit.item
	}
	result.Outcome = "updated"
	if len(edits) == 0 {
		result.Outcome = "unchanged"
	}
	value := itemValue(list.Items[index], group)
	result.Item = &value
	return result, nil
}
