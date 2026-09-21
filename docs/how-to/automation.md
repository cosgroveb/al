# How to automate list changes

Read JSON results, submit a batch, and recover from an interrupted change.

## How to consume command results

1. Configure [account credentials](manage-lists.md#how-to-verify-account-credentials)
   in the agent or script environment. Install `jq` for the examples below.
2. Request JSON and branch on the command's exit status:

   ```sh
   if al lists --json > lists.json
   then
     jq '.data.lists' lists.json
   else
     al_exit=$?
     jq '.error' lists.json >&2
     exit "$al_exit"
   fi
   ```

   Run this as a script. `--json` writes one JSON document, including errors, to
   stdout. Keep the document intact until you have checked success.
3. Select a list ID from the result and use it in later commands. Do not parse
   the styled human output or choose the first matching name from an ambiguity
   error. See [reference](../reference/README.md) for result fields and exit codes.

## How to submit a batch

1. Write one JSON array to `additions.json`:

   ```json
   [
     {"name":"milk","quantity":"2 cartons"},
     {"name":"eggs","notes":"one dozen"}
   ]
   ```

   Use string values for metadata. Omit unchanged fields. Use an empty string
   to clear quantity, notes, or category. Do not use `null`, duplicate keys,
   unknown fields, or JSONL.
2. Send the batch to one explicit list and preserve its exit status and result:

   ```sh
   al_exit=0
   al add --stdin --list-id LIST_ID --json < additions.json > result.json || al_exit=$?
   jq . result.json
   printf 'al exit: %s\n' "$al_exit"
   ```

   Do not combine `--stdin` with positional item names or per-item flags. For
   an intentional duplicate, put `"new":true` on that add record.
3. Check each result's `index`, `id`, and `outcome`. If the exit status is nonzero,
   use the recovery procedure below before submitting more changes.

## How to recover from a partial or uncertain batch

1. Preserve `result.json` and inspect both the error and per-record outcomes:

   ```sh
   jq '{error, results: .data.results}' result.json
   ```

   Treat `unknown` as a change that may have reached AnyList or only partly
   completed. `skipped` means that record made no write request.
2. Fetch current state, including checked items:

   ```sh
   al items --list-id LIST_ID --all --json > current.json
   jq '.data.items' current.json
   ```

   Continue only after the read succeeds. Compare current values with the
   intended changes and the IDs from the original results.
3. Put only the remaining changes in a new batch. Use current item IDs, for
   example in `remaining.json`:

   ```json
   [{"id":"ITEM_ID","quantity":"2 cartons"}]
   ```

   ```sh
   al edit --stdin --list-id LIST_ID --json < remaining.json
   ```

   Do not replay an uncertain add with `"new":true`. Inspect whether the new item
   already exists before deciding to create it.
4. Read the list again and verify the final state. If another person changed a
   field during recovery, preserve their change or resolve it with them before
   replacing it.

For accepted fields, see [reference](../reference/README.md). For preflight and
shared-state limits, see [behavior](../explanation/behavior.md).
