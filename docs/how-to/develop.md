# How to build, test, and release al

Build a local executable, run the checks, and publish versioned packages.

## How to build and test the CLI

1. Install Go 1.25.0 or later and run these commands from the repository root:

   ```sh
   make help
   make build
   ./al --help
   ```

   `make` also builds `./al`. It does not install an executable on `PATH`.
2. Run tests and linting. Put golangci-lint v2 on `PATH` first. CI uses v2.13.2.

   ```sh
   make test
   make lint
   ```

   Tests use HTTP fixtures, race detection, and shuffled order. They do not
   contact AnyList.
3. Check the minimum supported Go version when changing Go compatibility:

   ```sh
   GOTOOLCHAIN=go1.25.0 go test ./...
   ```

4. Run `make clean` to remove `./al` and the generated man page.

## How to build the manual

1. Install Pandoc and a `man` reader, then generate the manual:

   ```sh
   make man
   ```

2. Open the generated manual without installing it:

   ```sh
   man -M "$PWD/build/docs/man" al
   ```

3. After changing commands or flags, update
   [the manual source](../reference/al.1.md), rebuild it, and compare it with
   `./al --help` and the affected subcommand's help.

## How to regenerate the protobuf schema

1. Install protoc 36.2 and the Go generator:

   ```sh
   go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12
   ```

   Put `protoc-gen-go` on `PATH` before continuing.
2. Regenerate the checked-in Go file:

   ```sh
   protoc --go_out=. --go_opt=paths=source_relative internal/anylistpb/anylist.proto
   ```

3. Inspect the diff and run `make test`.
   Building and running `al` does not require protoc or Node.

## How to check release packages locally

1. Choose an unprefixed release version, such as `0.1.0`, from a committed checkout.
2. On macOS, build the archive for the desired architecture:

   ```sh
   scripts/build-archive 0.1.0 arm64
   ```

   Use `amd64` for Intel Macs. Install Go and Pandoc before running the script.
3. For Debian, use a sid environment with the build dependencies in
   `debian/control`, plus Git and lintian:

   ```sh
   scripts/build-deb 0.1.0
   ```

   Run on `amd64` or `arm64` to build that architecture. Both scripts write to
   `dist/` and vendor modules in a temporary directory.
4. Test the produced package through the
   [installation procedure](install.md), including `al --version` and `man al`.
   GitHub CI also installs and removes Debian packages on trixie and noble.

## How to publish a release

1. Confirm that repository secret `HOMEBREW_TAP_TOKEN` has write access to
   `cosgroveb/homebrew-tap`. Consumers do not need this token.
2. Push the release changes to `main` and confirm its CI run passes:

   ```sh
   gh run list --workflow ci.yml --branch main
   ```

3. Choose an unused stable `vMAJOR.MINOR.PATCH` tag, then tag and push the checked
   commit to the repository's remote:

   ```sh
   git tag v0.1.0
   git push REMOTE v0.1.0
   ```

   Replace `REMOTE` with the checkout's remote name and `v0.1.0` with the selected
   version.
4. Check the release workflow and published assets:

   ```sh
   gh run list --workflow release.yml
   gh release view v0.1.0
   ```

   Require the package builds, published checksum verification, Go module
   installation, and Homebrew installation, test, audit, and tap push to pass.
   The release contains Darwin archives, Debian packages and source artifacts,
   and `SHA256SUMS`.

   The Go module job downloads the tag through `proxy.golang.org`, then installs
   and runs the command. That request makes the version available for
   [pkg.go.dev indexing](https://pkg.go.dev/about#adding-a-package). Indexing
   can take a few minutes. Check the version at
   `https://pkg.go.dev/github.com/cosgroveb/al@v0.1.0`, substituting the release
   tag. Publish a new tag for corrections. Published Go module versions are
   immutable.
5. Follow [the installation guide](install.md) for the published version and
   verify both the executable and manual.
