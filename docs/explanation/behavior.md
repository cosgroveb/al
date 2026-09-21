# List behavior and command outcomes

`al` reads and changes the lists in your AnyList account. Commands that inspect
or change lists fetch account data from the service. People sharing a list can
change that same data through AnyList while a command runs.

## Shared lists and account visibility

An item belongs to its list. Checking it, changing its notes, or removing it
changes the shared list that other participants use. A command does not create
a private working copy or reserve the list while it runs.

List deletion has a narrower meaning. `al list delete` removes the list from
your account's folders and removes your list settings. Its success establishes
that the list is no longer visible in that account. It does not establish that
AnyList deleted the underlying shared list for every participant. Sharing also
has a boundary: an accepted invitation or sharing response does not establish
that the recipient has opened or gained access to the list.

## Names, identity, and defaults

Names make commands readable, but several lists or items can have the same
name. `al` compares names exactly, including case, and reports matching
candidates when a name is ambiguous. It does not choose the first match. An ID
identifies one object and remains the same when its name changes, which makes
IDs useful for scripts and recurring tasks.

The saved default is a list ID. Renaming that list does not change the target
of later item commands, though removing access to it makes those commands fail.
The default saves repeated selection for items and categories. List renaming,
sharing, and deletion still require an explicit target. These operations affect
the list itself, so a local preference is insufficient to select their target.

## Reusing items and replacing details

Household lists contain recurring needs. Adding a name that matches one
existing item reuses that item and unchecks it, including when the item was
already checked. An unchecked item with no requested changes remains unchanged.
Creating a duplicate requires an explicit choice. This keeps repeated requests
for milk attached to the same item and its existing details.

Item details follow replacement semantics. A supplied quantity replaces the
quantity text, so adding “2 cartons” twice does not accumulate four cartons.
Omitting quantity, notes, or category preserves that field. An empty quantity,
note, or category name clears it. This distinction lets a command express only
the intended changes.

Categories belong to category groups, and the account's list or store settings
determine which group is active. `al` displays and changes assignments within
that group. It reports ambiguity when it cannot resolve an active group, rather
than choosing a category set that could mean something different in AnyList.
The CLI uses existing categories and does not manage category groups.

## Human output and machine results

The default output presents names, IDs, checked markers, and outcomes as text.
Terminal styling adds emphasis, while the words and markers carry the meaning.
Piped output, a nonempty `NO_COLOR`, or `TERM=dumb` disables added styling.
Human diagnostics go to stderr, separate from results on stdout.

`--json` makes one command produce one JSON result, including an error when the
command fails. A single document keeps the overall outcome together with any
per-item results. It also gives scripts a defined interface without depending
on the appearance of the human output. A batch is not a stream of independent
JSON successes.

A successful exit means the command completed, including a request that needed
no changes. A failed exit does not mean that nothing changed. Item writes
require an acknowledgment for each submitted operation, but their returned
values are not a fresh read of the list after the command. Another participant
may change an item before or after that acknowledgment.

## Preflight and incomplete changes

Before sending item writes, `al` validates the entire input and resolves its
targets. For a batch, it accounts for earlier records when checking later
ones. A bad target or ambiguous name found during this preflight prevents the
batch's writes. Preflight checks a snapshot. It does not lock the list against
changes from another person.

After preflight, writes run in order and stop at the first failure. They do not
form a transaction. An edit of several fields can require several requests,
and some of those requests may succeed before another fails. Earlier records
can therefore be complete while the current record is incomplete and later
records are skipped.

An `unknown` outcome means a request may have reached AnyList, or an operation
may have completed only in part. A missing response cannot distinguish those
states. Retrying could repeat a change, and rolling back could overwrite
another person's intervening work. `al` does neither automatically. Recovery
depends on a successful read of current state and a decision about which
changes remain. The [automation guide](../how-to/automation.md) describes that
recovery procedure.

## Authentication and protocol limits

Each invocation that needs AnyList reads credentials from its environment and
obtains a token for that process. `al auth status` checks those credentials with
the service without fetching lists. It does not report a saved login session.
Passwords and tokens remain in memory. Local configuration holds a client ID
and the default list ID, so a later command can retain its client identity and
list preference without retaining credentials.

`al` implements an unofficial AnyList protocol in Go. Service changes can break
its assumptions without a published API compatibility guarantee. Its scope is
lists and their items, including sharing and existing category assignments.
Photos, recipes, meal planning, and the rest of the AnyList application remain
outside that scope.

The [first-list tutorial](../tutorials/first-list.md) provides a small working
example. The [list guide](../how-to/manage-lists.md) covers daily tasks, and the
[reference](../reference/README.md) specifies commands, result fields, and exit
codes.
