package anylist

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"math"
	"strings"
	"time"

	pb "github.com/cosgroveb/al/internal/anylistpb"
	"google.golang.org/protobuf/proto"
)

// Lists refreshes account data and returns lists reachable from its root folder.
func (c *Client) Lists(ctx context.Context) ([]List, error) {
	if err := c.Authenticate(ctx); err != nil {
		return nil, err
	}
	data, err := c.request(ctx, "/data/user-data/get", "", nil, false)
	if err != nil {
		return nil, err
	}
	account := new(pb.PBUserDataResponse)
	if len(data) == 0 || proto.Unmarshal(data, account) != nil || account.ShoppingListsResponse == nil || account.ListFoldersResponse == nil {
		return nil, &Error{Code: "protocol", Message: "AnyList returned invalid account data"}
	}
	lists := make(map[string]*pb.ShoppingList)
	for _, list := range account.ShoppingListsResponse.NewLists {
		lists[list.GetIdentifier()] = list
	}
	for _, list := range account.ShoppingListsResponse.ModifiedLists {
		lists[list.GetIdentifier()] = list
	}
	folders := make(map[string]*pb.PBListFolder)
	for _, folder := range account.ListFoldersResponse.ListFolders {
		folders[folder.GetIdentifier()] = folder
	}
	root := account.ListFoldersResponse.GetRootFolderId()
	if root == "" || folders[root] == nil {
		return nil, &Error{Code: "protocol", Message: "AnyList account data is missing the root folder"}
	}
	visible := make([]string, 0)
	parents := make(map[string]string)
	visited := make(map[string]bool)
	var visit func(string) error
	visit = func(id string) error {
		if visited[id] {
			return nil
		}
		visited[id] = true
		for _, item := range folders[id].GetItems() {
			child := item.GetIdentifier()
			switch item.GetItemType() {
			case int32(pb.PBListFolderItem_FolderType):
				if err := visit(child); err != nil {
					return err
				}
			case int32(pb.PBListFolderItem_ListType):
				if lists[child] == nil {
					continue
				}
				if parent, ok := parents[child]; ok {
					if parent != id {
						return &Error{Code: "protocol", Message: "AnyList returned conflicting list folder membership"}
					}
					continue
				}
				parents[child] = id
				visible = append(visible, child)
			}
		}
		return nil
	}
	if err := visit(root); err != nil {
		return nil, err
	}
	groups := make(map[string][]*pb.PBListCategoryGroup)
	filters := make(map[string][]*pb.PBStoreFilter)
	for _, response := range account.ShoppingListsResponse.ListResponses {
		id := response.GetListId()
		for _, group := range response.CategoryGroupResponses {
			if group.CategoryGroup != nil {
				groups[id] = append(groups[id], group.CategoryGroup)
			}
		}
		filters[id] = append(filters[id], response.StoreFilters...)
	}
	settings := make(map[string]*pb.PBListSettings)
	for _, entry := range account.GetListSettingsResponse().GetSettings() {
		if entry.GetUserId() == "" || entry.GetUserId() == c.userID {
			settings[entry.GetListId()] = entry
		}
	}
	c.account, c.lists, c.parents = account, lists, parents
	c.groups, c.filters, c.settings = groups, filters, settings
	result := make([]List, 0, len(visible))
	for _, id := range visible {
		result = append(result, listValue(lists[id]))
	}
	return result, nil
}

func listValue(list *pb.ShoppingList) List {
	return List{
		ID:         list.GetIdentifier(),
		Name:       list.GetName(),
		Shared:     len(list.GetSharedUsers()) > 0,
		ModifiedAt: serviceTime(list.GetTimestamp()),
	}
}

func serviceTime(timestamp float64) string {
	if timestamp <= 0 {
		return ""
	}
	seconds, fraction := math.Modf(timestamp)
	return time.Unix(int64(seconds), int64(fraction*float64(time.Second))).UTC().Format(time.RFC3339Nano)
}

func (c *Client) visibleList(id string) (*pb.ShoppingList, error) {
	if c.account == nil {
		return nil, &Error{Code: "configuration", Message: "load lists before selecting a list"}
	}
	if _, ok := c.parents[id]; !ok || c.lists[id] == nil {
		return nil, &Error{Code: "not_found", Message: "list is missing or inaccessible"}
	}
	return c.lists[id], nil
}

func (c *Client) CreateList(ctx context.Context, name string) (MutationResult, error) {
	result := MutationResult{}
	if name == "" {
		return failedMutation(result, &Error{Code: "input", Message: "list name cannot be empty"}, false)
	}
	if c.account == nil {
		return failedMutation(result, &Error{Code: "configuration", Message: "load lists before creating a list"}, false)
	}
	id := NewID()
	result.ID = id
	otherID := NewID()
	meta := c.metadata("new-shopping-list")
	groupID := defaultGroupID(id)
	list := &pb.ShoppingList{Identifier: proto.String(id), Name: proto.String(name),
		Timestamp: proto.Float64(float64(time.Now().Unix())), Creator: proto.String(c.userID),
		LogicalClockTime: proto.Uint64(1), AllowsMultipleListCategoryGroups: proto.Bool(true),
		ListItemSortOrder: proto.Int32(int32(pb.ShoppingList_Manual)), NewListItemPosition: proto.Int32(int32(pb.ShoppingList_Bottom))}
	group := &pb.PBListCategoryGroup{Identifier: proto.String(groupID), ListId: proto.String(id),
		DefaultCategoryId: proto.String(otherID), Categories: []*pb.PBListCategory{{Identifier: proto.String(otherID),
			CategoryGroupId: proto.String(groupID), ListId: proto.String(id), Name: proto.String("Other"),
			Icon: proto.String("other"), SystemCategory: proto.String("other"), SortIndex: proto.Int32(0)}}}
	op := &pb.PBListOperation{Metadata: meta, ListId: proto.String(id), List: list,
		ListFolderId: proto.String(c.account.ListFoldersResponse.GetRootFolderId()), UpdatedCategoryGroup: group}
	if err := c.updateList(ctx, op); err != nil {
		return failedMutation(result, err, false)
	}
	hash := md5.Sum([]byte(c.userID + "-" + id))
	settings := &pb.PBListSettings{Identifier: proto.String(hex.EncodeToString(hash[:])), UserId: proto.String(c.userID),
		ListId: proto.String(id), ListCategoryGroupId: proto.String(groupID)}
	if err := c.updateSettings(ctx, "set-list-category-group-id", settings); err != nil {
		return failedMutation(result, err, true)
	}
	if _, err := c.Lists(ctx); err != nil {
		return failedMutation(result, err, true)
	}
	created, err := c.visibleList(id)
	if err != nil || created.GetName() != name || c.settings[id].GetListCategoryGroupId() != groupID {
		return failedMutation(result, &Error{Code: "verification", Message: "could not verify list creation and active category settings"}, true)
	}
	value := listValue(created)
	result.List, result.Outcome = &value, "created"
	return result, nil
}

func (c *Client) RenameList(ctx context.Context, id, name string) (MutationResult, error) {
	result := MutationResult{ID: id}
	list, err := c.visibleList(id)
	if err != nil {
		return failedMutation(result, err, false)
	}
	if name == "" {
		return failedMutation(result, &Error{Code: "input", Message: "list name cannot be empty"}, false)
	}
	if list.GetName() == name {
		value := listValue(list)
		result.List, result.Outcome = &value, "unchanged"
		return result, nil
	}
	meta := c.metadata("rename-list")
	if err = c.updateList(ctx, &pb.PBListOperation{Metadata: meta, ListId: proto.String(id), UpdatedValue: proto.String(name)}); err != nil {
		return failedMutation(result, err, false)
	}
	list.Name = proto.String(name)
	if _, err = c.Lists(ctx); err != nil {
		return failedMutation(result, err, true)
	}
	list, err = c.visibleList(id)
	if err != nil || list.GetName() != name {
		return failedMutation(result, &Error{Code: "verification", Message: "could not verify the renamed list"}, true)
	}
	value := listValue(list)
	result.List, result.Outcome = &value, "renamed"
	return result, nil
}

// DeleteList removes account-visible folder membership and per-list settings.
// Residual settings can be removed by ID even after folder removal succeeded.
func (c *Client) DeleteList(ctx context.Context, id string) (MutationResult, error) {
	result := MutationResult{ID: id}
	if c.account == nil {
		return failedMutation(result, &Error{Code: "configuration", Message: "load lists before deleting a list"}, false)
	}
	parent, visible := c.parents[id]
	settings := c.settings[id]
	if !visible && settings == nil {
		return failedMutation(result, &Error{Code: "not_found", Message: "list and residual settings are missing or inaccessible"}, false)
	}
	if list := c.lists[id]; list != nil {
		value := listValue(list)
		result.List = &value
	}
	wrote := false
	if visible {
		listDataID := c.account.ListFoldersResponse.GetListDataId()
		if listDataID == "" {
			return failedMutation(result, &Error{Code: "protocol", Message: "AnyList account data is missing the list data identifier"}, false)
		}
		meta := c.metadata("delete-folder-items")
		op := &pb.PBListFolderOperation{Metadata: meta, ListDataId: proto.String(listDataID),
			OriginalParentFolderId: proto.String(parent), FolderItems: []*pb.PBListFolderItem{{Identifier: proto.String(id), ItemType: proto.Int32(int32(pb.PBListFolderItem_ListType))}}}
		err := c.sendUpdate(ctx, "/data/list-folders/update", meta.GetOperationId(), &pb.PBListFolderOperationList{Operations: []*pb.PBListFolderOperation{op}})
		if err != nil {
			return failedMutation(result, err, false)
		}
		wrote = true
		delete(c.parents, id)
	}
	if settings != nil {
		payload := &pb.PBListSettings{Identifier: settings.Identifier, UserId: settings.UserId, ListId: settings.ListId, Timestamp: settings.Timestamp}
		if err := c.updateSettings(ctx, "remove-list-settings", payload); err != nil {
			return failedMutation(result, err, wrote)
		}
		wrote = true
		delete(c.settings, id)
	}
	if _, err := c.Lists(ctx); err != nil {
		return failedMutation(result, err, wrote)
	}
	_, stillVisible := c.parents[id]
	if stillVisible || c.settings[id] != nil {
		return failedMutation(result, &Error{Code: "verification", Message: "could not verify removal of list folder membership and settings"}, true)
	}
	result.Outcome = "removed"
	return result, nil
}

func (c *Client) updateSettings(ctx context.Context, handler string, settings *pb.PBListSettings) error {
	meta := c.metadata(handler)
	return c.sendUpdate(ctx, "/data/list-settings/update", meta.GetOperationId(), &pb.PBListSettingsOperationList{
		Operations: []*pb.PBListSettingsOperation{{Metadata: meta, UpdatedSettings: settings}}})
}

func (c *Client) ShareList(ctx context.Context, id, email string) (MutationResult, error) {
	result := MutationResult{ID: id}
	list, err := c.visibleList(id)
	if err != nil {
		return failedMutation(result, err, false)
	}
	meta := c.metadata("share-shopping-list")
	data, err := c.send(ctx, "/data/shopping-lists/share-list", "operation", &pb.PBListOperation{
		Metadata: meta, ListId: proto.String(id), UpdatedValue: proto.String(email)})
	if err != nil {
		return failedMutation(result, err, false)
	}
	response := new(pb.PBShareListOperationResponse)
	if len(data) == 0 || proto.Unmarshal(data, response) != nil {
		return failedMutation(result, &Error{Code: "protocol", Message: "AnyList returned an invalid sharing response", Unknown: true}, false)
	}
	if response.StatusCode == nil {
		return failedMutation(result, &Error{Code: "protocol", Message: "AnyList sharing response is missing its status", Unknown: true}, false)
	}
	if response.GetStatusCode() != 0 {
		return failedMutation(result, &Error{Code: "operation_failed", Message: "AnyList rejected the sharing request"}, false)
	}
	if response.GetErrorTitle() != "" || response.GetErrorMessage() != "" {
		return failedMutation(result, &Error{Code: "protocol", Message: "AnyList returned conflicting sharing status", Unknown: true}, false)
	}
	if response.SharedUser == nil || !strings.EqualFold(response.SharedUser.GetEmail(), email) {
		return failedMutation(result, &Error{Code: "protocol", Message: "AnyList did not confirm the requested sharing recipient", Unknown: true}, false)
	}
	result.Outcome = "invited"
	if response.SharedUser.GetUserId() != "" {
		result.Outcome = "shared"
	}
	value := listValue(list)
	result.List = &value
	return result, nil
}
