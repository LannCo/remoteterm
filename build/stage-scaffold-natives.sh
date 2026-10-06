#!/bin/sh
# Copyright 2026, Command Line Inc.
# SPDX-License-Identifier: Apache-2.0
#
# Stage a tsunami scaffold whose node_modules holds the Tailwind natives (oxide,
# lightningcss, @parcel/watcher) for ONE platform, whatever host runs this.
#
# usage: stage-scaffold-natives.sh <src scaffold dir> <dst scaffold dir> <os> <cpu>
#   os: npm os name (darwin, linux); cpu: npm cpu name (arm64, x64)
#
# The source scaffold must hold the package.json and package-lock.json that the plain
# `npm install` produced: the lockfile lists every platform's natives, so `npm ci --os
# --cpu` installs the same versions the host build resolved, for the platform asked for.
set -eu

if [ "$#" -ne 4 ]; then
    echo "usage: $0 <src scaffold dir> <dst scaffold dir> <os> <cpu>" >&2
    exit 2
fi
src=$1
dst=$2
os=$3
cpu=$4

for f in package.json package-lock.json app-main.go.tmpl; do
    if [ ! -f "$src/$f" ]; then
        echo "stage-scaffold-natives: $src/$f is missing; build the scaffold first (task build:tsunamiscaffold)" >&2
        exit 1
    fi
done

rm -rf "$dst"
mkdir -p "$dst"
cp -R "$src/." "$dst/"
rm -rf "$dst/nm" "$dst/node_modules"

(cd "$dst" && npm ci --os="$os" --cpu="$cpu" --ignore-scripts --no-audit --no-fund)
mv "$dst/node_modules" "$dst/nm"

missing=0
for pkg in "@tailwindcss/oxide-$os-$cpu" "lightningcss-$os-$cpu" "@parcel/watcher-$os-$cpu"; do
    # linux names carry a libc suffix (oxide-linux-x64-gnu), darwin and windows do not.
    found=0
    for candidate in "$dst/nm/$pkg" "$dst/nm/$pkg"-*; do
        [ -d "$candidate" ] && found=1
    done
    if [ "$found" -eq 0 ]; then
        echo "stage-scaffold-natives: $pkg is missing from $dst/nm" >&2
        missing=1
    fi
done
[ "$missing" -eq 0 ] || exit 1

# Nothing for another platform may ride along: a wrong-arch native is only found when
# Tailwind fails on the user's machine.
stray=$(cd "$dst/nm" && ls -d @tailwindcss/oxide-* lightningcss-* @parcel/watcher-* 2>/dev/null | grep -v -e "oxide-$os-$cpu" -e "lightningcss-$os-$cpu" -e "watcher-$os-$cpu" || true)
if [ -n "$stray" ]; then
    echo "stage-scaffold-natives: natives for other platforms in $dst/nm:" >&2
    echo "$stray" >&2
    exit 1
fi
echo "staged $dst for $os-$cpu"
