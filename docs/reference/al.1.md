% AL(1) User Commands
%
% September 21, 2026

# NAME

al - manage AnyList lists and items

# SYNOPSIS

**al** [**--json**] *command* [*options*] [*arguments*]

**al** [**--help** | **--version**]

# DESCRIPTION

**al** manages lists and their items in one AnyList account. It reads
credentials from the environment. Human-readable output is the default.
**--json** selects one JSON result document, including errors.

Commands that contact AnyList authenticate for each invocation. Passwords,
tokens, and account data stay in process memory. Local configuration stores a
client identifier and an optional default list identifier.

Names match exactly and case-sensitively. Multiple matching names produce an
error with candidate identifiers. Item lookup includes checked and unchecked
items, regardless of the default display filter.

Options may appear before or after arguments. **--** ends option parsing, so
the following argument can begin with a hyphen. Multiword names require shell
quoting.

The CLI supports list and item operations. It does not expose photos, recipes,
meal planning, category creation, or an interactive TUI. AnyList's service
protocol is unofficial and can change independently of this program.

# COMMANDS

## Authentication

**al auth status**
:   Verifies **ANYLIST_EMAIL** and **ANYLIST_PASSWORD** with AnyList. Each
    invocation performs a fresh authentication request. It does not fetch
    lists or establish a persistent login session. The command creates a local
    client identifier if one is missing and preserves the saved default list.
    On success, human mode prints `Authenticated with AnyList.` and JSON mode
    returns `{"authenticated":true}` as data. Failure data is `null`.

## Lists

**al lists**, **al ls**
:   Shows lists reachable from the account's root folder, including nested
    folders. Each result contains the list name and identifier.

**al list create** *NAME*
:   Creates a generic list with an `Other` category. The name must be nonempty.
    Duplicate list names are permitted. The command verifies the created list
    and its active category settings before reporting `created`.

**al list rename** (*NAME* | **--id** *LIST_ID*) *NEW_NAME*
:   Replaces the selected list's name. Both names must be nonempty. The command
    reports `unchanged` when the current name already equals *NEW_NAME*.
    Otherwise it verifies the new name before reporting `renamed`.

**al list delete** (*NAME* | **--id** *LIST_ID*)
:   Removes the list from this account's folder membership and removes its
    per-list settings. Success means account-visible removal and settings
    cleanup. It does not establish deletion for other participants in a shared
    list. An explicit identifier can finish settings cleanup after the list
    becomes invisible. A missing list with no residual settings produces
    `not_found`.

**al list share** (*NAME* | **--id** *LIST_ID*) *EMAIL*
:   Requests sharing with an email address. *EMAIL* must contain only the
    address, without a display name. The command reports `invited` when AnyList
    confirms the email recipient, or `shared` when the response also identifies
    an AnyList user. These outcomes describe the service response. They do not
    independently verify recipient access.

Rename, delete, and share require an explicit list name or **--id**. They do
not use the saved default list. List mutations have no confirmation prompt.

## Item and category reads

**al items** [**--list** *NAME* | **--list-id** *ID*] [**--all** | **--checked**]
:   Shows unchecked items by default. **--checked** selects checked items.
    **--all** includes both states. The two flags are mutually exclusive.
    Results include the selected list and each item's name, identifier,
    checked state, quantity, notes, and category.

**al categories** [**--list** *NAME* | **--list-id** *ID*]
:   Shows categories in the selected list's active category group, in category
    sort order. It does not create or modify categories. An unresolved active
    group produces `category_ambiguous`. A list with no category group returns
    an empty category array.

## Item mutations

Each command accepts one item name or **--id** *ITEM_ID*. **--stdin** replaces
the single target with a JSON array. The selected list applies to all records.
Single-item calls and batches use the same validation and result format.

**al add** (*NAME* | **--id** *ITEM_ID*) [*metadata-options*] [**--new**]
:   Reuses one exact name match and unchecks it. If no item has that name,
    creates an unchecked item. **--new** creates an intentional duplicate.
    An identifier requires an existing item and cannot accompany **--new**.
    Omitted metadata preserves the reused item's values. An already unchecked
    item with no changes reports `unchanged`.

**al edit** (*NAME* | **--id** *ITEM_ID*) [*metadata-options*] [**--name** *NEW_NAME*]
:   Replaces the supplied fields on an existing item. At least one metadata
    option or **--name** is required. Omitted fields retain their values.
    Renaming does not change checked state and can create duplicate names.

**al check** (*NAME* | **--id** *ITEM_ID*)
:   Marks an existing item checked. An already checked item reports
    `unchanged`.

**al uncheck** (*NAME* | **--id** *ITEM_ID*)
:   Marks an existing item unchecked. An already unchecked item reports
    `unchanged`.

**al remove** (*NAME* | **--id** *ITEM_ID*)
:   Removes an existing item. A missing item produces `not_found`.

## Configuration

**al config show**
:   Displays the resolved configuration path, client identifier, and default
    list identifier. This command reads local configuration without contacting
    AnyList or creating a configuration file.

**al config set default-list** (*NAME* | **--list-id** *LIST_ID*)
:   Resolves an account-visible list and saves its identifier. It requires
    credentials and contacts AnyList. The saved identifier survives list
    renames. A later command fails if the saved list is missing or inaccessible.
    There is no command to clear the default.

## Help

**al help** [*command* ...]
:   Displays help for the root or a subcommand. Running **al**, **al auth**,
    **al list**, **al config**, or **al config set** without a subcommand also
    displays help.

# OPTIONS

## Global options

**--json**
:   Prints one JSON envelope to standard output. It applies to successful
    results, errors, help, and version output. It does not select JSON input.

**-h**, **--help**
:   Displays help and exits successfully without contacting AnyList.

**-v**, **--version**
:   Displays the version at the root command and exits successfully without
    contacting AnyList.

## List selectors

**--list** *NAME*
:   Selects an exact list name for **items**, **categories**, and item mutation
    commands. Mutually exclusive with **--list-id**. Empty names are invalid.

**--list-id** *ID*
:   Selects a list identifier for **items**, **categories**, item mutations,
    and **config set default-list**. Empty identifiers are invalid.

For item and category commands, an explicit selector takes precedence over
the saved default list. With neither selector nor a default, the command fails
with `configuration`. The CLI does not choose a list based on the number of
visible lists.

## Item selectors and input

**--id** *ID*
:   Selects an existing item for item mutations. It replaces the positional
    item name. On **list rename**, **list delete**, and **list share**, this
    flag selects the list instead. Empty identifiers are invalid.

**--stdin**
:   Reads one JSON array from standard input on **add**, **edit**, **check**,
    **uncheck**, or **remove**. It excludes positional item targets and
    per-item flags, including **--id**, even when a supplied Boolean flag is
    false. List selection and **--json** remain available.

**--new**
:   Creates an item even when its name already exists. Available only on
    **add**. This flag cannot accompany **--id**, including **--new=false**.

## Item display filters

**--all**
:   Includes checked and unchecked items in **items** output.

**--checked**
:   Includes only checked items in **items** output. Mutually exclusive with
    **--all**, including when either flag is set to false.

## Metadata options

**--name** *NEW_NAME*
:   Replaces an item's name on **edit**. The new name must be nonempty. The
    positional *NAME* remains the selector for the original name.

**--quantity** *TEXT*
:   Replaces quantity text on **add** or **edit**. An empty string clears it.
    Quantity is text. A value of `2` replaces the current quantity without
    incrementing it or converting units.

**--notes** *TEXT*
:   Replaces notes on **add** or **edit**. An empty string clears them.

**--category** *NAME*
:   Assigns an existing category by exact name in the active category group.
    An empty string clears the active group's assignment. Available on **add**
    and **edit**. Mutually exclusive with **--category-id**.

**--category-id** *ID*
:   Assigns an existing category identifier in the active category group.
    Available on **add** and **edit**. An empty identifier is invalid.

Category changes preserve assignments in other groups. Clearing a category
requires an active group and records an empty assignment in that group.
The CLI does not change the active group. Category selection follows the
list's active store filter, then its category settings, then its default
group, then a sole remaining group. Multiple unresolved groups produce
`category_ambiguous`.

# STANDARD INPUT

**--stdin** accepts one complete JSON array and reads through end of input.
It does not accept JSON Lines. Each array member must be an object with exactly
one nonempty `name` or `id` selector. Keys are case-sensitive.

`name`
:   String. Exact item name to select, or the name to create for **add**.

`id`
:   String. Existing item identifier. Mutually exclusive with `name`.

`quantity`, `notes`, `category`, `categoryId`
:   Optional strings on **add** and **edit**, corresponding to the metadata
    flags. `category` and `categoryId` are mutually exclusive.

`newName`
:   Optional nonempty string on **edit**, corresponding to **--name**.

`new`
:   Optional Boolean on **add**, corresponding to **--new**. Its presence is
    incompatible with `id`, including `"new":false`.

An edit record requires at least one of `newName`, `quantity`, `notes`,
`category`, or `categoryId`. Check, uncheck, and remove records accept only
the selector. Empty `quantity`, `notes`, and `category` strings clear their
fields. An empty `categoryId` is invalid.

Unknown fields, duplicate keys, null values, unsupported fields, incorrect
types, conflicting selectors, and trailing data are input errors. The CLI
validates the whole array before contacting AnyList. An empty array succeeds
with `{"results":[]}` without loading configuration or contacting AnyList.
Command-line flag conflicts still produce errors for an empty array.

Example add records:

```json
[{"name":"milk","quantity":"2 cartons"},{"name":"eggs","new":true}]
```

Example edit records:

```json
[{"id":"ITEM_ID","newName":"whole milk","notes":"","category":""}]
```

After input validation, the CLI resolves the full batch against one fetched
list state before sending mutation requests. It resolves records in order
against projected changes, including planned additions, renames, and removals.
Two bare adds of a previously missing name create one item and then reuse it.
A resolution failure prevents all writes in that batch.

Execution sends mutations in record order and stops at the first failure.
An edit can require several requests. Batches have no transaction, rollback,
or isolation from other clients. The CLI does not retry uncertain writes.

# OUTPUT

Human output goes to standard output. Diagnostics go to standard error.
An unsuccessful mutation can print completed or partial results before its
diagnostic. Human output includes identifiers and textual outcomes. Items use
`[ ]` for unchecked and `[x]` for checked. Control and Unicode format characters
in names and metadata appear as escaped text.

Color styling requires a terminal output stream, a **TERM** value other than
`dumb`, and an empty or unset **NO_COLOR**. JSON output contains no terminal
styling. Human presentation is not a machine-readable contract.

## JSON envelope

**--json** writes one JSON object followed by a newline:

```json
{"ok":true,"data":{"lists":[]},"error":null}
```

`ok`
:   Boolean. True when the command succeeds.

`data`
:   Command-specific object, or `null` when no result data exists. A failure
    may include partial results. Empty collections remain arrays.

`error`
:   `null` on success. On failure, an object with `code` and `message` strings,
    optional integer `httpStatus`, and optional `candidates` array. Candidate
    objects contain `id` and `name`. Error messages are human diagnostics.

JSON errors use standard output without additional usage text. An output-write
failure can prevent a complete JSON document and emits a diagnostic on standard
error.

## Data by command

`auth status`
:   `authenticated: true` on success. Failure data is `null`.

`lists`
:   `lists`, an array of list objects.

`items`
:   `list`, the selected list object, and `items`, an array of item objects.

`categories`
:   `list`, the selected list object, and `categories`, an array of category
    objects.

Item mutations
:   `list`, the selected list object, and `results`, an array of mutation
    results. An empty input array returns only `results: []`. Failures before
    batch planning, such as malformed input or an inaccessible list, return
    `null` data.

List mutations
:   `results`, an array containing one mutation result once the target
    identifier is known. Earlier failures return `null` data.

`config show`
:   `config`, an object with `clientId` and `defaultListId` strings, and `path`,
    the resolved configuration filename. Unset values are empty strings.

`config set default-list`
:   `config`, `path`, and `list`, the selected list object.

Help and version
:   `text`, a string containing the human help or version text.

## List, category, and item objects

List and category objects contain `id` and `name` strings.

Item objects contain:

`id`, `name`
:   Item identifier and name strings.

`checked`
:   Boolean checked state.

`quantity`
:   Display quantity string. For a structured service quantity, this is raw
    text when available, otherwise the amount and unit joined with a space.

`quantityDetails`
:   Optional object with `amount`, `unit`, and `raw` strings when the service
    provides a structured quantity. Quantity writes replace this representation
    with the supplied raw text and empty amount and unit.

`notes`
:   Notes string.

`categoryId`, `categoryName`
:   Effective category in the list's active category group. Either string may
    be empty. The CLI reads explicit assignments and supported legacy category
    matches.

## Mutation results

`index`
:   Zero-based input index on item mutations, including single-item commands.
    Absent from list mutation results.

`id`
:   Target identifier string. A preflight failure can leave this empty for
    unresolved records. Identifiers allocated during planning do not establish
    that the service created an item.

`outcome`
:   One of the values below.

`item`
:   Optional item object on successful add, edit, check, and uncheck results,
    including unchanged results. It reflects the fetched state plus
    acknowledged changes, without an independent read after each item write.

`list`
:   Optional list object for list mutations.

`added`
:   The service acknowledged creation of an item.

`updated`
:   The service acknowledged changes to an existing item, including a check
    or uncheck.

`unchanged`
:   The requested item state or list name already matched. No mutation request
    was needed.

`removed`
:   The service acknowledged item removal, or the CLI verified account-visible
    list removal and settings cleanup.

`created`, `renamed`
:   The CLI verified list creation or renaming.

`invited`, `shared`
:   The service confirmed the sharing recipient as described under
    **al list share**.

`failed`
:   The record failed validation or resolution, the service rejected the
    mutation, or the operation failed before any write could apply.

`unknown`
:   A write may have reached the service, or an earlier request for this record
    succeeded before a later request or verification failed. This is not a
    claim that the whole record failed or that no data changed.

`skipped`
:   The CLI attempted no mutation request for this record. A resolution failure
    marks the failing record `failed` and all other records `skipped`. An
    execution failure preserves earlier outcomes and marks later records
    `skipped`.

An uncertain item result requires comparison with a fresh **items --all**
read to establish current state. Replaying an add with **--new** can create a
duplicate. A list create or delete failure can retain an identifier even when
only part of the operation succeeded. **list delete --id** can finish residual
settings cleanup after folder removal, provided those settings still exist.

# ERROR CODES

`input`
:   Invalid command syntax, conflicting options, or invalid JSON input.

`configuration`
:   Missing credentials or list selection, or unreadable, invalid, or unwritable
    local configuration.

`not_found`
:   A selected list, item, or category is missing or inaccessible.

`ambiguous`
:   More than one list, item, or category matches the exact name. `candidates`
    contains identifiers and names.

`category_ambiguous`
:   The selected list has multiple category groups and no resolvable active
    group. Group selection is available in AnyList, not in this CLI.

`authentication`
:   AnyList returned HTTP 401.

`permission`
:   AnyList returned HTTP 403.

`transport`
:   A request failed, timed out, was canceled, or its response could not be
    read. Each service request has a 30-second timeout.

`canceled`
:   The command was canceled while reading standard input.

`protocol`
:   AnyList returned malformed or inconsistent data, an oversized response,
    or no matching operation acknowledgment.

`operation_failed`
:   AnyList rejected an operation or returned another unsuccessful HTTP status.

`verification`
:   A read after a list mutation did not establish the requested final state.

`conflict`
:   A proposed new item identifier already exists.

`internal`
:   The CLI could not encode a service request.

# ENVIRONMENT

**ANYLIST_EMAIL**
:   Account email address. Required for commands that contact AnyList.

**ANYLIST_PASSWORD**
:   Account password. Required for commands that contact AnyList. The CLI has
    no password argument, credential-file fallback, or persistent token cache.

**NO_COLOR**
:   Any nonempty value disables color styling.

**TERM**
:   The value `dumb` disables color styling.

**XDG_CONFIG_HOME**
:   Overrides the configuration directory on Linux. The value must be an
    absolute path. It does not override the macOS configuration location.

Help, version, configuration display, invalid input, and empty stdin arrays
need no credentials or service access. A valid nonempty item batch requires
authentication even when its resolved operations make no changes.

# FILES

`~/Library/Application Support/al/config.json`
:   Configuration on macOS.

`$XDG_CONFIG_HOME/al/config.json`
:   Configuration on Linux when **XDG_CONFIG_HOME** is set.

`~/.config/al/config.json`
:   Configuration on Linux otherwise.

The file contains one JSON object with `clientId` and `defaultListId` string
fields. Unknown fields, invalid types, null in place of the whole object, and
trailing JSON are invalid. A missing file represents empty configuration.
The CLI saves changes by replacing the file atomically. Credentials and
account content are not stored in this file.

# EXIT STATUS

**0**
:   Success, including unchanged results, help, version, and an empty batch.

**1**
:   Configuration, selection, authentication, permission, transport, operation,
    cancellation, or output failure.

**2**
:   Invalid command syntax or input.

# EXAMPLES

```sh
al auth status --json
al lists
al items --list "Groceries" --all
al add milk --list "Groceries" --quantity "2 cartons"
al edit --list-id LIST_ID --id ITEM_ID --notes '' --category ''
al check --list-id LIST_ID --id ITEM_ID
al add --list "Groceries" -- --special
printf '%s\n' '[{"name":"milk"},{"name":"eggs"}]' |
  al add --stdin --list "Groceries" --json
```

# SEE ALSO

**man**(1)

[Project documentation and releases](https://github.com/cosgroveb/al).
