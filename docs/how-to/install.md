# How to install and upgrade al

Install `al` and its manual through Homebrew or a release Debian package.

## How to install on macOS

1. Install from the tap:

   ```sh
   brew install cosgroveb/tap/al
   ```

2. Verify the executable and manual:

   ```sh
   al --version
   al --help
   man al
   ```

3. [Configure credentials](manage-lists.md#how-to-verify-account-credentials)
   before using commands that contact AnyList.

## How to install on Debian or Ubuntu

1. Choose a version from [GitHub releases](https://github.com/cosgroveb/al/releases).
   Run these commands in an empty directory. Packages support `amd64` and `arm64`.

   ```sh
   version=0.1.0
   arch="$(dpkg --print-architecture)"
   package="al_${version}-1_${arch}.deb"
   release="https://github.com/cosgroveb/al/releases/download/v${version}"
   curl -fLO "$release/$package"
   curl -fLO "$release/SHA256SUMS"
   ```

2. Verify the downloaded package. Continue only if the package reports `OK`:

   ```sh
   sha256sum --ignore-missing --check SHA256SUMS
   ```

3. Install the package and a manual reader:

   ```sh
   sudo apt install "./$package" man-db
   ```

4. Verify the installed version and manual:

   ```sh
   dpkg-query -W al
   al --version
   man al
   ```

## How to upgrade

1. On macOS, refresh the tap and upgrade:

   ```sh
   brew update
   brew upgrade cosgroveb/tap/al
   ```

   On Debian or Ubuntu, repeat the download, checksum, and apt steps with the
   new release version. There is no apt repository for automatic updates.

2. Run `al --version` and confirm the requested version.

## How to locate an installed manual

1. Ask `man` for the installed path:

   ```sh
   man -w al
   ```

2. If Homebrew installed `al` but your manual search path excludes it, open the
   formula's manual directory:

   ```sh
   man -M "$(brew --prefix al)/share/man" al
   ```

   On Debian or Ubuntu, check the package's files and install `man-db` if the
   `man` command is missing:

   ```sh
   dpkg -L al
   sudo apt install man-db
   man al
   ```

3. If Ubuntu prints `This system has been minimized`, restore its
   documentation support:

   ```sh
   sudo unminimize
   ```

   Repeat [the release package installation](#how-to-install-on-debian-or-ubuntu)
   if the system excluded al's manual when it first installed the package.

For a source checkout, use [the development guide](develop.md#how-to-build-the-manual).
