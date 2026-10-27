#!/usr/bin/env bash
# Patch the package's bundled runtime independently of the saved Code Server pin.
# Keep the Node major and native-module ABI; never replace it with system Node.
set -euo pipefail

root="${1:?Code Server installation directory is required}"
arch="${2:?Debian architecture is required}"
node_binary="$root/lib/node"
current="$("$node_binary" -p 'process.versions.node')"
if [[ ! "$current" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "Cannot determine Code Server's bundled Node version" >&2
    exit 1
fi

# Supported runtime minimums. Digests are pinned from the respective
# https://nodejs.org/dist/v<VERSION>/SHASUMS256.txt (linux .tar.gz archives).
case "${current%%.*}:$arch" in
    22:amd64)
        target=22.23.4; node_arch=x64
        checksum=b8492284c76a54baf58d61668dd0e41ee0fc98adabf9503d30f05d08f55b4406 ;;
    22:arm64)
        target=22.23.4; node_arch=arm64
        checksum=f7a1e486dbc8b286847815b477d4fb5e9fb963ed4489c3e1588853d685f589f9 ;;
    24:amd64)
        target=24.22.1; node_arch=x64
        checksum=eb6728e77c576a35162c122c45ccadf224d89ff00abc8dae46cc2897c6002d7c ;;
    24:arm64)
        target=24.22.1; node_arch=arm64
        checksum=33c37be234ba8bee041961047b19160f4d7d0f164e8e246f78bdd10feff148b3 ;;
    *)
        echo "Unsupported Code Server Node $current on $arch; use a Code Server release based on Node 22 or 24" >&2
        exit 1 ;;
esac

if dpkg --compare-versions "$current" ge "$target"; then
    echo "Code Server bundled Node $current meets minimum $target"
    exit 0
fi

current_abi="$("$node_binary" -p 'process.versions.modules')"
# Stage beside the destination so replacement is atomic. Failure leaves the
# existing binary untouched, and Remote will report the installation as failed.
stage="$(mktemp -d "$root/lib/.node-runtime.XXXXXX")"
trap 'rm -rf "$stage"' EXIT
archive="node-v${target}-linux-${node_arch}"
echo "Updating Code Server bundled Node $current to $target"
curl -fSL --retry 3 --connect-timeout 20 --speed-limit 1024 --speed-time 30 \
    -o "$stage/node.tar.gz" "https://nodejs.org/dist/v${target}/${archive}.tar.gz"
if ! printf '%s  %s\n' "$checksum" "$stage/node.tar.gz" | sha256sum --check --status; then
    echo "Code Server Node archive checksum verification failed" >&2
    exit 1
fi
tar -xzf "$stage/node.tar.gz" -C "$stage" --strip-components=2 --no-same-owner "$archive/bin/node"
if [ "$("$stage/node" -p 'process.versions.node')" != "$target" ] \
    || [ "$("$stage/node" -p 'process.versions.modules')" != "$current_abi" ]; then
    echo "Code Server Node replacement has the wrong version or native-module ABI" >&2
    exit 1
fi
chmod 0755 "$stage/node"
mv -fT "$stage/node" "$node_binary"
echo "Code Server bundled Node $("$node_binary" --version) installed"
