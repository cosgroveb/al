# al

[![CI](https://github.com/cosgroveb/al/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/cosgroveb/al/actions/workflows/ci.yml)
[![Release workflow](https://github.com/cosgroveb/al/actions/workflows/release.yml/badge.svg)](https://github.com/cosgroveb/al/actions/workflows/release.yml)
[![Latest release](https://img.shields.io/github/v/release/cosgroveb/al?sort=semver)](https://github.com/cosgroveb/al/releases/latest)
[![Go version](https://img.shields.io/github/go-mod/go-version/cosgroveb/al)](go.mod)
[![Go Reference](https://pkg.go.dev/badge/github.com/cosgroveb/al.svg)](https://pkg.go.dev/github.com/cosgroveb/al)
[![License](https://img.shields.io/github/license/cosgroveb/al)](LICENSE)
[![Homebrew tap](https://img.shields.io/badge/Homebrew-tap-FBB040?logo=homebrew&logoColor=white)](docs/how-to/install.md#how-to-install-on-macos)
[![Debian and Ubuntu packages](https://img.shields.io/badge/Debian%2FUbuntu-.deb-A81D33?logo=debian&logoColor=white)](docs/how-to/install.md#how-to-install-on-debian-or-ubuntu)

Manage AnyList lists and items from a Go CLI. `al` prints styled terminal
output by default and one JSON document with `--json`.

## Install

On macOS:

```sh
brew install cosgroveb/tap/al
al --version
man al
```

On Debian or Ubuntu, [download a release package and install it with
apt](docs/how-to/install.md#how-to-install-on-debian-or-ubuntu).
Packages support amd64 and arm64. Both installation methods include the
manual and these docs.

With Go 1.25.0 or later:

```sh
go install github.com/cosgroveb/al@latest
```

See [Go installation](docs/how-to/install.md#how-to-install-with-go) for
executable location and manual availability. Tagged releases also appear on
[pkg.go.dev](https://pkg.go.dev/github.com/cosgroveb/al).

## Authenticate

Provide `ANYLIST_EMAIL` and `ANYLIST_PASSWORD` through the process environment,
then check them with AnyList:

```sh
al auth status
```

Credentials and tokens stay in process memory. `al auth status` contacts the
service without fetching lists or creating a persistent login session. The
[first-list tutorial](docs/tutorials/first-list.md) includes credential prompts
that keep passwords out of shell history.

```sh
al lists
al items --list "Groceries"
al add --list "Groceries" --quantity "2 cartons" milk
al items --list "Groceries" --json
```

Use `al ls` as a short alias for `al lists`.

## Documentation

| Need | Read |
| --- | --- |
| Learn by using a practice list | [Your first list](docs/tutorials/first-list.md) |
| Complete a task | [Install and upgrade](docs/how-to/install.md), [manage lists](docs/how-to/manage-lists.md), [automate changes](docs/how-to/automation.md) |
| Look up commands, fields, and errors | [CLI reference](docs/reference/README.md), also available through `man al` |
| Understand selection, sharing, and partial changes | [List and item behavior](docs/explanation/behavior.md) |

## Development

Use Go 1.25.0 or later:

```sh
make help
make build
make test
make lint
```

`make test` uses HTTP fixtures, race detection, and shuffled test order.
`make lint` requires golangci-lint v2. `make man` generates the manual from
the Markdown reference and requires Pandoc. See [development and release
tasks](docs/how-to/develop.md) for tooling, generation, and package checks.

`al` uses AnyList's unofficial protocol. Service changes can break
compatibility. See [protocol limits](docs/explanation/behavior.md).

## License

Copyright 2026 Brian Cosgrove. Licensed under [Apache 2.0](LICENSE).
