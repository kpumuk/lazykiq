---
title: "Installation"
description: "Install Lazykiq from releases or source."
summary: "Install Lazykiq from releases or source."
date: 2025-12-30T00:00:00Z
lastmod: 2025-12-30T00:00:00Z
draft: false
weight: 10
toc: true
---

## Releases

Download the latest release for your platform from the
[GitHub Releases page](https://github.com/kpumuk/lazykiq/releases).

{{< callout context="note" title="Note" icon="outline/info-circle" >}}
Lazykiq uses special glyphs in the UI; without a Nerd Font in your terminal some characters may render incorrectly.
{{< /callout >}}

## Homebrew Tap

```bash
brew install --cask kpumuk/tap/lazykiq
```

## AUR

```bash
yay -S --noconfirm lazykiq-bin
```

## Install from source

Install the current development version with Go 1.25:

```bash
go install github.com/kpumuk/lazykiq/cmd/lazykiq@latest
```

## Download a binary

Pick a version and platform, then download, verify, and extract the archive.
Replace the variables with the values that match the release you want.

```bash
VERSION="x.y.z"
TAG="v${VERSION}"
OS="darwin"
ARCH="arm64"
ASSET="lazykiq-${VERSION}-${OS}-${ARCH}.tar.gz"

curl -sLO "https://github.com/kpumuk/lazykiq/releases/download/${TAG}/${ASSET}"
curl -sLO "https://github.com/kpumuk/lazykiq/releases/download/${TAG}/checksums.txt"

shasum -a 256 -c --ignore-missing checksums.txt
tar -xzf "${ASSET}"
```
