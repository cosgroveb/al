# al

Manage AnyList lists and items from a Go CLI. `al` prints styled terminal output
by default and one JSON document with `--json`.

## Install

Install on macOS through Homebrew:

```sh
brew install cosgroveb/tap/al
al --version
al --help
```

For Debian or Ubuntu on amd64 or arm64, download a release package, verify its
checksum, and install it with apt:

```sh
version=0.1.0
arch="$(dpkg --print-architecture)"
package="al_${version}-1_${arch}.deb"
release="https://github.com/cosgroveb/al/releases/download/v${version}"
curl -fLO "$release/$package"
curl -fLO "$release/SHA256SUMS"
sha256sum --ignore-missing --check SHA256SUMS
sudo apt install "./$package"
al --version
```

Packages include the executable, man page, and dependency license notices.
CI verifies installation on Debian trixie and Ubuntu noble. To upgrade, use
`brew upgrade cosgroveb/tap/al` on macOS or repeat the apt steps with the new
[release version](https://github.com/cosgroveb/al/releases). Linux packages come
from GitHub releases, without an apt repository for automatic updates.

## Configure

Set `ANYLIST_EMAIL` and `ANYLIST_PASSWORD` in the process environment before
using network commands. `al` keeps passwords and authentication tokens in
memory. It does not read credential files or save credentials in its config.

Verify those credentials with AnyList:

```sh
al auth status
al auth status --json
```

Each invocation checks authentication with the service. It does not fetch your
lists or create a persistent login session. Missing credentials, rejected
credentials, and service failures produce distinct errors and exit status 1.

Choose a default list:

```sh
al lists
al config set default-list "Groceries"
al config show
al items
```

The config contains `clientId` and `defaultListId` at
`os.UserConfigDir()/al/config.json`. `al config show` prints the resolved path
without creating a file. Config changes use atomic file replacement. The saved
list ID survives renames. A missing or inaccessible saved list produces an error.

Help, version, config display, invalid syntax, and empty input arrays need no
credentials or network access.

## Lists and selectors

```sh
al lists
al list create "Weekend errands"
al list rename "Weekend errands" "Saturday errands"
al list share --id LIST_ID person@example.com
al list delete --id LIST_ID
al config set default-list --list-id LIST_ID
```

`al lists` shows lists reachable from the account's root folder. Creation makes
a generic list with an `Other` category. Deletion removes the account's folder
membership and list settings. The result describes removal from this account,
without establishing deletion for other participants in a shared list.

List lifecycle commands require an explicit name or `--id`. They do not use the
saved default. For rename and share, `--id` replaces the target name:

```sh
al list rename --id LIST_ID "New name"
al list share --id LIST_ID person@example.com
```

Item commands and `categories` accept `--list NAME` or `--list-id ID`, then fall
back to the saved default. Names match exactly and case-sensitively. Duplicate
names produce an error with candidate IDs. Use the ID flag from the error to
select one candidate.

Flags work before or after positional arguments. Use `--` before names that
begin with a hyphen:

```sh
al add --list "Groceries" -- --special
al list create -- --special
```

## Items

```sh
al items --list "Groceries"
al items --list-id LIST_ID --all
al items --list-id LIST_ID --checked
al categories --list-id LIST_ID

al add milk --list "Groceries" --quantity "2 cartons" --notes "whole"
al add milk --list "Groceries" --new
al add --list-id LIST_ID --id ITEM_ID
al edit milk --list "Groceries" --name "whole milk"
al edit --list-id LIST_ID --id ITEM_ID --category-id CATEGORY_ID
al check --list-id LIST_ID --id ITEM_ID
al uncheck milk --list "Groceries"
al remove --list-id LIST_ID --id ITEM_ID
```

`items` shows unchecked items by default. `--checked` shows checked items and
`--all` shows both. Mutation lookup includes checked and unchecked items.

An add by name reuses one exact match and unchecks it. An unchecked match with
no metadata changes stays unchanged. `--new` creates an intentional duplicate
and cannot accompany `--id`. An add by ID requires an existing item.

Supplied metadata replaces the selected field. Omitted metadata retains its
value. Quantity is text: setting `2` replaces the quantity rather than adding
two. Empty strings clear quantity, notes, or the active category assignment:

```sh
al edit --list-id LIST_ID --id ITEM_ID --quantity '' --notes '' --category ''
```

Use `--category NAME` or `--category-id ID` to select an existing category in the
list's active category group. Category changes preserve assignments in other
groups. Item commands do not create category definitions.

## JSON input

Use `--stdin` on `add`, `edit`, `check`, `uncheck`, or `remove` to read one JSON
array. The list selector applies to all records. `--stdin` excludes positional
item targets, `--id`, and per-item flags. `--json` controls output independently.

```sh
printf '%s\n' '[{"name":"milk","quantity":"2"},{"name":"eggs"}]' |
  al add --stdin --list "Groceries" --json

printf '%s\n' '[{"name":"milk","newName":"whole milk"},{"name":"whole milk","notes":""}]' |
  al edit --stdin --list "Groceries" --json

printf '%s\n' '[{"id":"ITEM_ID"}]' |
  al check --stdin --list-id LIST_ID --json
```

Each record has exactly one `name` or `id` selector. Fields depend on the command:

| Command | Additional fields |
| --- | --- |
| `add` | `quantity`, `notes`, `category` or `categoryId`, `new` |
| `edit` | `newName`, `quantity`, `notes`, `category` or `categoryId` |
| `check`, `uncheck`, `remove` | None |

All fields are strings except Boolean `new`. An edit needs metadata or `newName`.
Missing metadata preserves its value. Empty `quantity`, `notes`, or
`category` clears that field. Names and IDs cannot be empty. Nulls, duplicate
or unknown fields, invalid types, conflicting selectors, and trailing JSON
produce input errors. `[]` succeeds without contacting AnyList.

`al` validates and resolves the full batch before writing. It plans records in
order, so later records see earlier planned additions, renames, and removals.
Two bare adds of a new name plan one item, then reuse it. Set `new: true`
when you intend to create another item with that name.

## Results and recovery

JSON output uses one envelope for success and failure:

```json
{"ok":true,"data":{"lists":[]},"error":null}
```

Collections stay arrays when empty. Reads return `data.lists`, `data.items`, or
`data.categories`. Item and category reads also return `data.list`. Mutations
return `data.results`, including target `id` and `outcome`. Item results include
a zero-based `index`, including single-item commands. Config output contains
`data.config` and `data.path`. Help and version output use `data.text`.

Successful authentication checks return `data.authenticated: true`. Failed
checks return an error with `data: null`, including when the service is
unreachable and credentials could not be verified.

Errors contain `code` and `message`, with `httpStatus` or `candidates` when
available. `authentication` and `permission` are distinct error codes. JSON
errors go to stdout without usage text. Human diagnostics go to stderr. Human
output escapes control characters in names and metadata. Styling is disabled
for pipes, `TERM=dumb`, and non-empty `NO_COLOR`.

| Exit status | Meaning |
| --- | --- |
| `0` | Success |
| `2` | Invalid command or input |
| `1` | Configuration, selection, authentication, transport, or operation failure |

Batch execution stops on the first upstream failure and reports every input
index. Outcomes are `added`, `updated`, `unchanged`, `removed`, `failed`,
`unknown`, or `skipped`. `unknown` means a write may have reached the service,
or an earlier field update succeeded before a later field failed. `skipped`
means `al` attempted no request for that record.

Read the list again before retrying a partial batch:

```sh
al items --list-id LIST_ID --all --json
```

Compare current state with `data.results`, then submit the remaining changes
using IDs. Replaying an uncertain add with `--new` can create a duplicate.
Batches have no transaction or cross-client isolation. `al` does not roll back
changes that could overwrite another person's edits, or retry uncertain writes.

List creation and deletion can require several requests. Failure results retain
the target list ID. If deletion removed folder membership before settings
cleanup failed, finish cleanup with that ID:

```sh
al list delete --id LIST_ID --json
```

Successful list outcomes are `created`, `renamed`, `removed`, `unchanged`,
`invited`, or `shared`. Sharing uses `invited` when the response identifies an
email recipient and `shared` when it also identifies an AnyList user. An accepted
invitation does not establish recipient access.

## Development and protocol limits

`al` uses AnyList's unofficial protocol. Service changes can break compatibility.
The Go client uses bounded requests and has no production endpoint override.
Quantity writes use the service's raw-text representation. They do not implement
numeric quantity totals or unit conversion. Category resolution follows the
list's active settings and reports ambiguity when it cannot choose one group.

Use Go 1.25.0 or later and the Makefile for development:

```sh
make help
make build
make test
make lint
make clean
```

`make` builds `./al`. `make test` runs the local suite with race detection and
shuffled test order. The suite uses HTTP fixtures and does not contact AnyList.
`make clean` removes the built executable.

`make lint` requires [golangci-lint](https://golangci-lint.run/docs/welcome/install/local/)
v2 on `PATH` (verified with v2.13.2). It runs govet, errcheck, staticcheck,
unused, and exhaustive. To check the minimum supported Go version:

```sh
GOTOOLCHAIN=go1.25.0 go test ./...
```

Fixture coverage verifies request encoding and CLI behavior. It does not establish
live service compatibility. Live sharing requires a chosen recipient and remains
unverified by the default suite.

The runtime needs neither Node nor a protobuf compiler. To regenerate the checked-in
schema with protoc 36.2 and protoc-gen-go v1.36.12:

```sh
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12
protoc --go_out=. --go_opt=paths=source_relative internal/anylistpb/anylist.proto
```

Put `protoc-gen-go` on `PATH` before running `protoc`.

## Releases

Push an unused stable `vMAJOR.MINOR.PATCH` tag after main CI passes. The release
workflow builds Darwin amd64/arm64 archives and Linux amd64/arm64 Debian
packages. Debian builds use sid for its Go toolchain, then CI installs, runs,
and removes each package on trixie and noble. Linux executables use
`CGO_ENABLED=0` so the same package works across these distributions.

The workflow publishes those files, Debian source artifacts, and `SHA256SUMS`
to the GitHub release. It verifies the published checksums, generates
`Formula/al.rb` in `cosgroveb/homebrew-tap`, and installs, tests, and audits the
formula before pushing it. Repository secret `HOMEBREW_TAP_TOKEN` needs write
access to that tap. Consumers do not need a GitHub token.

For local packaging checks, run `scripts/build-archive VERSION ARCH` on macOS
or `scripts/build-deb VERSION` in a sid environment with the build dependencies
from `debian/control`, plus Git and lintian. Both scripts write to `dist/`.
Packaging vendors modules in a temporary directory. The checkout stays clean.
