#!/bin/sh
set -eu

usage() {
    printf '%s\n' \
        'Usage: sh install.sh [--version VERSION] [--install-dir DIRECTORY]' \
        '' \
        'Install gocloc from a GitHub Release (no Go toolchain required).' \
        'Defaults: latest stable release, $HOME/.local/bin.' \
        'Versions may include or omit the v prefix.'
}

fail() {
    printf 'Error: %s\n' "$*" >&2
    exit 1
}

version=latest
install_dir=
while [ "$#" -gt 0 ]; do
    case "$1" in
        --version|--install-dir)
            [ "$#" -ge 2 ] && [ -n "$2" ] || fail "$1 requires a value"
            case "$1" in
                --version) version=$2 ;;
                --install-dir) install_dir=$2 ;;
            esac
            shift 2
            ;;
        -h|--help) usage; exit 0 ;;
        *) fail "Unknown argument: $1 (use --help)" ;;
    esac
done

if [ -z "$install_dir" ]; then
    [ -n "${HOME:-}" ] || fail 'HOME is unset; use --install-dir'
    install_dir=$HOME/.local/bin
fi
# Absolute paths also prevent paths beginning with '-' being treated as options.
case "$install_dir" in
    /*) ;;
    *) install_dir=$PWD/$install_dir ;;
esac

os=$(uname -s)
arch=$(uname -m)
case "$os" in
    Darwin|Linux) ;;
    *) fail "Unsupported OS: $os. On Windows, use install.ps1." ;;
esac
case "$arch" in
    x86_64|amd64) arch=x86_64 ;;
    arm64|aarch64) arch=arm64 ;;
    i386|i486|i586|i686) arch=i386 ;;
    *) fail "Unsupported architecture: $arch" ;;
esac
[ "$os/$arch" != Darwin/i386 ] || fail 'No release binary for Darwin/i386'

for dependency in curl tar awk mktemp; do
    command -v "$dependency" >/dev/null 2>&1 || fail "Required command not found: $dependency"
done
if command -v sha256sum >/dev/null 2>&1; then
    checksum_tool=sha256sum
elif command -v shasum >/dev/null 2>&1; then
    checksum_tool=shasum
else
    fail 'SHA-256 verification requires sha256sum or shasum'
fi

release_url=https://github.com/longafterx/gocloc/releases
if [ "$version" = latest ]; then
    resolved_url=$(curl --fail --silent --show-error --location \
        --proto '=https' --proto-redir '=https' --tlsv1.2 \
        --connect-timeout 15 --max-time 60 --retry 2 \
        --output /dev/null --write-out '%{url_effective}' "$release_url/latest") ||
        fail 'Cannot find the latest release. Check your network and GitHub Releases; a Git tag alone has no binaries.'
    case "$resolved_url" in
        "$release_url"/tag/*) version=${resolved_url##*/} ;;
        *) fail 'GitHub did not return a release tag; try --version VERSION' ;;
    esac
fi
case "$version" in
    v*) ;;
    *) version=v$version ;;
esac
case "$version" in
    *[!0-9A-Za-z.+-]*) fail "Invalid release version: $version" ;;
esac
printf '%s\n' "$version" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$' ||
    fail "Invalid release version: $version"

archive=gocloc_${os}_${arch}.tar.gz
checksums=gocloc_${version#v}_checksums.txt
download_url=$release_url/download/$version
work_dir=$(mktemp -d)
staged_binary=
cleanup() {
    if [ -n "$staged_binary" ]; then rm -f "$staged_binary"; fi
    # work_dir is always the unique directory returned by mktemp above.
    rm -rf "$work_dir"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

download() {
    curl --fail --silent --show-error --location \
        --proto '=https' --proto-redir '=https' --tlsv1.2 \
        --connect-timeout 15 --max-time 300 --retry 2 \
        --output "$work_dir/$1" "$download_url/$1" ||
        fail "Cannot download $1 for $version. Check the release assets and your network."
}

printf 'Downloading gocloc %s for %s/%s...\n' "$version" "$os" "$arch"
download "$archive"
download "$checksums"
expected=$(awk -v name="$archive" '$2 == name || $2 == "*" name {print tolower($1)}' "$work_dir/$checksums")
[ "${#expected}" -eq 64 ] || fail "Missing or ambiguous SHA-256 entry for $archive"
case "$expected" in *[!0-9a-f]*) fail 'Invalid SHA-256 checksum' ;; esac
if [ "$checksum_tool" = sha256sum ]; then
    actual=$(sha256sum "$work_dir/$archive")
else
    actual=$(shasum -a 256 "$work_dir/$archive")
fi
actual=${actual%% *}
[ "$actual" = "$expected" ] || fail 'SHA-256 mismatch; the existing installation has not been changed'

# Extract only the executable, not arbitrary paths from the archive.
tar -xzf "$work_dir/$archive" -C "$work_dir" gocloc || fail 'Cannot extract gocloc from release archive'
[ -f "$work_dir/gocloc" ] && [ ! -L "$work_dir/gocloc" ] || fail 'Archive does not contain a regular gocloc binary'
mkdir -p "$install_dir"
[ ! -d "$install_dir/gocloc" ] && [ ! -L "$install_dir/gocloc" ] ||
    fail "Refusing to replace a directory or symlink: $install_dir/gocloc"

# Verify the executable before atomically replacing an existing installation.
staged_binary=$(mktemp "$install_dir/.gocloc.XXXXXX")
cp "$work_dir/gocloc" "$staged_binary"
chmod 755 "$staged_binary"
installed_version=$("$staged_binary" --version) || fail 'Downloaded binary cannot run on this machine'
mv -f "$staged_binary" "$install_dir/gocloc"
staged_binary=
printf 'Installed %s\n%s\n' "$install_dir/gocloc" "$installed_version"
case ":${PATH:-}:" in
    *":$install_dir:"*) ;;
    *) printf 'Add %s to the beginning of PATH to use this installation.\n' "$install_dir" ;;
esac
existing=$(command -v gocloc || true)
if [ -n "$existing" ] && [ "$existing" != "$install_dir/gocloc" ]; then
    printf 'Note: your PATH currently selects %s instead.\n' "$existing"
fi
