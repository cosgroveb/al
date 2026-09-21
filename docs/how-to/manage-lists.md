# How to manage lists and items

Select a list, change its contents, and verify the resulting state.

## How to verify account credentials

1. Load your AnyList email and password into the process environment as
   `ANYLIST_EMAIL` and `ANYLIST_PASSWORD`. Use your local secret environment,
   outside the repository. `al` does not read credential files.
2. Check authentication:

   ```sh
   al auth status
   ```

   Success prints `Authenticated with AnyList.` The check does not fetch lists.
3. If the command fails, correct the reported configuration or authentication
   error before continuing. For a service or connection error, check again
   after service access recovers.

## How to select a list and save a default

1. Find the list's ID:

   ```sh
   al lists
   ```

   `al ls` is a short alias for the same command.

2. Replace `LIST_ID` with that ID and inspect its items:

   ```sh
   al items --list-id LIST_ID --all
   ```

   Use `--list "Groceries"` for an exact, case-sensitive name. If several lists
   match, select one of the returned candidate IDs.
3. Save the ID for later item commands:

   ```sh
   al config set default-list --list-id LIST_ID
   al config show
   al items
   ```

   The saved ID survives a rename. List creation, renaming, sharing, and deletion
   still require an explicit target.

## How to add an item or put it back on a list

1. Add the item by exact name:

   ```sh
   al add --list-id LIST_ID --quantity "2 cartons" --notes "whole" milk
   ```

   This reuses and unchecks one exact match, including a checked item. Use
   `--new` only when you want another item with the same name.
2. Verify the unchecked items:

   ```sh
   al items --list-id LIST_ID
   ```

   For a name beginning with a hyphen, place it after `--`:

   ```sh
   al add --list-id LIST_ID -- --special
   ```

## How to replace or clear item details

1. Find the item and available category IDs:

   ```sh
   al items --list-id LIST_ID --all
   al categories --list-id LIST_ID
   ```

2. Replace the fields you need:

   ```sh
   al edit --list-id LIST_ID --id ITEM_ID --name "whole milk" \
     --quantity "1 carton" --notes "for breakfast" --category-id CATEGORY_ID
   ```

   Omit fields to preserve them. Quantity replaces text rather than adding to a
   total. To clear quantity, notes, and the active category assignment:

   ```sh
   al edit --list-id LIST_ID --id ITEM_ID --quantity '' --notes '' --category ''
   ```

3. Read `al items --list-id LIST_ID --all` and confirm the changed fields.

## How to check, uncheck, or remove an item

1. Run `al items --list-id LIST_ID --all` and choose the item ID.
2. Run the command for the intended change:

   ```sh
   al check --list-id LIST_ID --id ITEM_ID
   al uncheck --list-id LIST_ID --id ITEM_ID
   al remove --list-id LIST_ID --id ITEM_ID
   ```

   Choose one command. `remove` deletes the item from the list.
3. Run `al items --list-id LIST_ID --all` and verify the result.

## How to create or rename a list

1. Create a list and note its returned ID:

   ```sh
   al list create "Weekend errands"
   ```

2. To rename it, supply that ID and the new name:

   ```sh
   al list rename --id LIST_ID "Saturday errands"
   ```

3. Run `al lists` and confirm the name and ID.

## How to share a list

1. Run `al lists` and confirm the list ID and intended recipient's email address.
   Agents need an explicit recipient before requesting sharing.
2. Request sharing with that recipient:

   ```sh
   al list share --id LIST_ID person@example.com
   ```

3. Check the reported `invited` or `shared` result. Verify recipient access in
   AnyList. An accepted sharing request alone does not establish their access.

## How to remove a list from your account

1. Run `al lists` and confirm the ID to remove. This operation removes this
   account's folder membership and list settings. It does not establish deletion
   for other participants in a shared list.
2. Delete using the explicit ID:

   ```sh
   al list delete --id LIST_ID --json
   ```

3. Run `al lists` and confirm the list is absent. If the deletion reported a
   failure after removing membership, repeat the deletion with the same ID to
   finish settings cleanup.

See [command reference](../reference/README.md) for flags and
[behavior](../explanation/behavior.md) for shared-state limits.
