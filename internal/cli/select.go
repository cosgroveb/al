package cli

import (
	"fmt"

	"github.com/cosgroveb/al/internal/anylist"
)

func selectList(lists []anylist.List, name, id, defaultID string) (anylist.List, error) {
	if name != "" && id != "" {
		return anylist.List{}, inputError("--list and --list-id are mutually exclusive")
	}
	if name == "" && id == "" {
		id = defaultID
		if id == "" {
			return anylist.List{}, &anylist.Error{Code: "configuration", Message: "select a list with --list or --list-id, or run al config set default-list"}
		}
	}
	candidates := []anylist.Candidate{}
	for _, list := range lists {
		if id != "" && list.ID == id {
			return list, nil
		}
		if id == "" && list.Name == name {
			candidates = append(candidates, anylist.Candidate{ID: list.ID, Name: list.Name})
		}
	}
	if len(candidates) > 1 {
		return anylist.List{}, ambiguous("list", "--list-id", candidates)
	}
	if len(candidates) == 1 {
		return anylist.List{ID: candidates[0].ID, Name: candidates[0].Name}, nil
	}
	return anylist.List{}, &anylist.Error{Code: "not_found", Message: "list is missing or inaccessible; use an exact list name or ID"}
}

func selectItem(items []anylist.Item, name, id string) (anylist.Item, error) {
	candidates := []anylist.Candidate{}
	var selected anylist.Item
	for _, item := range items {
		if id != "" && item.ID == id {
			return item, nil
		}
		if id == "" && item.Name == name {
			selected = item
			candidates = append(candidates, anylist.Candidate{ID: item.ID, Name: item.Name})
		}
	}
	if len(candidates) > 1 {
		return anylist.Item{}, ambiguous("item", "--id", candidates)
	}
	if len(candidates) == 1 {
		return selected, nil
	}
	return anylist.Item{}, &anylist.Error{Code: "not_found", Message: "item not found; use an exact name or --id"}
}

func selectCategory(categories []anylist.Category, name, id *string) (*string, error) {
	if name != nil && id != nil {
		return nil, inputError("category and categoryId are mutually exclusive")
	}
	if name == nil && id == nil {
		return nil, nil
	}
	if name != nil && *name == "" {
		value := ""
		return &value, nil
	}
	candidates := []anylist.Candidate{}
	for _, category := range categories {
		if id != nil && category.ID == *id {
			value := category.ID
			return &value, nil
		}
		if name != nil && category.Name == *name {
			candidates = append(candidates, anylist.Candidate{ID: category.ID, Name: category.Name})
		}
	}
	if len(candidates) > 1 {
		return nil, ambiguous("category", "--category-id", candidates)
	}
	if len(candidates) == 1 {
		value := candidates[0].ID
		return &value, nil
	}
	return nil, &anylist.Error{Code: "not_found", Message: "category not found in the list's active category group"}
}

func ambiguous(kind, flag string, candidates []anylist.Candidate) *anylist.Error {
	return &anylist.Error{Code: "ambiguous", Message: fmt.Sprintf("multiple %s names match; select one with %s %s", kind, flag, candidates[0].ID), Candidates: candidates}
}
