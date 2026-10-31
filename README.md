<p align="center">
  <img src="assets/icon.png" alt="Troopers Transfer icon" width="96" height="96">
</p>

<h1 align="center">Troopers Transfer</h1>

<p align="center">
  <strong>Send files and folders between any two computers with a three-word code. No accounts, no installer, no cloud storage.</strong>
</p>

<p align="center">
  <img alt="Go" src="https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white">
  <img alt="Platforms" src="https://img.shields.io/badge/platforms-Windows%20%7C%20Linux%20%7C%20macOS-555">
  <img alt="Built on croc" src="https://img.shields.io/badge/built%20on-croc%20v11-5b8def">
  <img alt="GUI: Fyne" src="https://img.shields.io/badge/GUI-Fyne%20v2-7e57c2">
  <img alt="License: MIT" src="https://img.shields.io/badge/license-MIT-green">
</p>

---

Troopers Transfer is a portable, point-and-click desktop app for sending files
between computers. It wraps [croc](https://github.com/schollz/croc), the
end-to-end encrypted file transfer tool, in a friendly GUI that anyone can
use. It's a **single executable**: no installer, no admin rights, and no
registry writes, so it runs happily from a USB stick.

<p align="center">
  <img src="docs/screenshots/send.png" alt="Sending files: the code phrase, a QR code and live progress" width="600">
</p>

## Table of contents

- [Features](#features)
- [Screenshots](#screenshots)
- [Download](#download)
- [Quick start](#quick-start)
- [Using it](#using-it)
  - [First launch on Windows (SmartScreen)](#first-launch-on-windows-smartscreen)
  - [Sending](#sending)
  - [Receiving](#receiving)
  - [Hosting a relay for friends](#hosting-a-relay-for-friends)
  - [Connecting to a friend's relay](#connecting-to-a-friends-relay)
  - [History](#history)
  - [Settings](#settings)
- [Security and privacy](#security-and-privacy)
- [Self-hosting a croc relay](#self-hosting-a-croc-relay)
- [Troubleshooting](#troubleshooting)
- [Building from source](#building-from-source)
- [Developer CLI](#developer-cli)
- [Project layout](#project-layout)
- [How the croc integration works](#how-the-croc-integration-works)
- [Safety design](#safety-design)
- [Tests](#tests)
- [Continuous integration and releases](#continuous-integration-and-releases)
- [Contributing](#contributing)
- [License and credits](#license-and-credits)

## Features

- **Drag and drop sending.** Drop files or folders on the window (or browse),
  click **Send**, and read the code phrase to the other person. A QR code is
  shown too.
- **Preview before receiving.** The receiver sees the file list and total size
  and chooses **Accept** or **Decline** before a single byte is written.
- **Live progress.** Progress bar, speed, time remaining, current file, and a
  **Cancel** button.
- **Plain-English errors.** "Wrong code…", "Sender closed the app…",
  "Connection lost, try again." Technical details are tucked away under an
  expander for when you need them.
- **Never overwrites.** Existing files are left alone; duplicates are saved as
  `name (1).ext`, `name (2).ext`, …
- **End-to-end encrypted.** croc uses PAKE + AES-256. Relays only see
  encrypted bytes.
- **Fast on a LAN.** When both computers are on the same network, croc
  connects them directly.
- **Your relay or the public one.** Works with croc's free public relays or
  your own self-hosted relay.
- **Host a relay in one click.** The Host tab runs a relay on your PC and
  opens the router ports automatically (UPnP / NAT-PMP). If that fails, it
  explains why and gives step-by-step manual instructions filled in for your
  network. Friends paste your connection code into the Connect tab.
- **Truly portable.** Settings live in a JSON file next to the exe.
- **Session history** of everything you sent and received.
- **Cross-platform.** Windows, Linux and macOS from one Go codebase.

## Screenshots

| Receive | Accept or decline |
|---|---|
| ![Receive tab](docs/screenshots/receive.png) | ![Accept dialog](docs/screenshots/accept.png) |
| **History** | **Settings** |
| ![History tab](docs/screenshots/history.png) | ![Settings tab](docs/screenshots/settings.png) |

## Download

Grab the latest build from the [**Releases**](../../releases) page:

| Platform | File |
|---|---|
| Windows (64-bit) | `TroopersTransfer-windows-amd64.zip` |
| Linux (64-bit) | `TroopersTransfer-linux-amd64.tar.gz` |
| macOS (Apple Silicon) | `TroopersTransfer-macos-arm64.tar.gz` |

Unzip it and run it. That's the whole installation. Builds of every commit are
also available as artifacts on the [**Actions**](../../actions) tab.

## Quick start

1. Both people open Troopers Transfer.
2. **Sender:** drag files onto the window and click **Send**. A code like
   `curry-salsa-blunt` appears.
3. **Receiver:** open the **Receive** tab, type the code, click **Receive**,
   then **Accept**.

The files land in `Downloads\Troopers Transfer` by default.

## Using it

### First launch on Windows (SmartScreen)

The exe isn't code-signed, so Windows may show **"Windows protected your PC"**
the first time you run it. To continue:

1. Click **More info**.
2. Click **Run anyway**.

You only need to do this once per copy of the file. Windows Firewall may also
ask whether to allow network access. Allowing it on **private networks** lets
two computers on the same Wi-Fi/LAN connect directly, which is much faster.

On **macOS** the binary is also unsigned: right-click it and choose **Open**
the first time.

### Sending

1. Open the **Send** tab and drag in files or folders, or use **Add files…** /
   **Add folder…**. Remove an item with its trash icon, or **Clear** them all.
2. Optional: type your own code phrase (at least 6 characters). Otherwise a
   random one like `curry-salsa-blunt` is generated.
3. Click **Send**. Tell the receiver the code (or let them scan the QR code,
   or click **Copy**), and **keep the app open until the transfer finishes**.

### Receiving

1. Open the **Receive** tab and paste or type the code. Spaces are fine:
   `curry salsa blunt` works too.
2. Pick a destination folder with **Change…**. The default is
   `Downloads\Troopers Transfer`. **Open** shows the folder in your file
   manager.
3. Click **Receive**, check the file list, and click **Accept** (or
   **Decline**; nothing is saved).

The sender has to start first. If nobody is sending with that code within
30 seconds, you'll see "Wrong code, or the sender isn't sending right now."

### Hosting a relay for friends

The **Host** tab runs a croc relay inside the app, so you and your friends
transfer through your PC instead of the public relays. That's often faster,
and your files never touch anyone else's server.

1. Open **Host** and click **Start hosting**. The relay uses five TCP ports
   in a row, starting at 9009 by default (9009–9013). Change **Starting port**
   if those are taken.
2. If Windows asks whether to allow Troopers Transfer through the firewall,
   click **Allow**.
3. With **Open the ports on my router automatically (UPnP)** ticked, the app
   asks your router to forward the ports (UPnP, with NAT-PMP as a fallback).
   They are closed again when you stop hosting or quit, and they expire after
   an hour if the app crashes.
4. Send your friend the **connection code**, for example
   `203.0.113.5:9009/amber-falcon-ridge`. There's one code for friends over
   the internet and one for friends on the same Wi-Fi.

While you're hosting, your own transfers go through your relay automatically.

If the ports can't be opened, the Host tab explains why in plain English and
what to do. The steps are filled in with your router's address, your PC's IP
address and the exact ports, and **Copy these steps** puts them on the
clipboard:

| What happened | What the app tells you |
|---|---|
| The router didn't answer (UPnP off or unsupported) | Step-by-step manual port forwarding, plus a tip to switch UPnP on |
| The router refused | The same manual steps |
| Another device already uses the ports | Pick a different starting port, or remove the old router rule |
| Your provider uses carrier-grade NAT | Hosting can't work on this connection. Port forwarding won't help, so it suggests a friend hosts or you use the public relays |
| Your router is behind another router or modem | How to forward on the upstream device, or use bridge mode |
| The router's address doesn't match your public address | Usually a VPN; turn it off and try again |
| Another program on this PC uses the ports | Close it, or choose another starting port |

**Router guides** opens step-by-step pages for specific router models.

### Connecting to a friend's relay

On the other computer, open **Connect**, paste the connection code and click
**Connect**. The app checks it first: that the relay answers, that the
password is right, and that all five ports can be reached. If something's
wrong, it says what (for example, that only the first port was forwarded).
**Use public relays** switches back. **Test connection** checks a code
without switching to it.

The Connect tab fills in the relay address and password under **Settings**,
so you can still edit them there by hand.

### History

The **History** tab lists this session's transfers, newest first: what was
sent or received, how big it was, the code used, and whether it succeeded.
History is kept in memory only and is never written to disk.

### Settings

| Setting | Meaning |
|---|---|
| Relay address | Leave blank for croc's public relays. Set `host:port` to use your own relay. **Both sides must use the same relay.** |
| Relay password | The password your relay was started with. Blank = croc's default. |
| Zip folders before sending | Sends each folder as a single zip that's unpacked on arrival. Faster for many small files. |
| Default save folder | Where received files go. |

Settings are stored in `TroopersTransfer.json` **next to the exe**, so the app
stays portable. If that folder isn't writable (for example in
`Program Files`), they're stored in the per-user config folder instead
(`%APPDATA%\TroopersTransfer\TroopersTransfer.json` on Windows,
`~/.config/TroopersTransfer/` on Linux,
`~/Library/Application Support/TroopersTransfer/` on macOS). The Settings tab
shows which file is in use. Nothing is written until you click
**Save settings**.

Example `TroopersTransfer.json`:

```json
{
  "relay_address": "relay.example.com:9009",
  "relay_password": "choose-a-password",
  "zip_folders": true,
  "receive_folder": "D:\\Incoming"
}
```

> [!NOTE]
> The relay password is stored in plain text in that JSON file. It only
> protects access to the relay; your files are still end-to-end encrypted by
> the code phrase.

## Security and privacy

- **Encryption:** every transfer is end-to-end encrypted by croc. The code
  phrase is used for a PAKE key exchange, and data is encrypted with AES-256.
  Neither the relay nor anyone on the network can read your files.
- **What a relay sees:** the relay only forwards encrypted bytes between two
  peers that know the same code. On the same LAN, croc skips the relay and
  connects directly.
- **Code phrases:** anyone who knows the code while the sender is waiting can
  receive the files, so share it only with the intended person. Use a longer
  custom code for sensitive transfers.
- **Receiving is opt-in:** you always see the offered files and can decline.
  Unsafe file names (absolute paths, `..`, links that point outside the
  destination) automatically decline the transfer. See
  [Safety design](#safety-design).
- **No telemetry:** the app makes no network connections except to the croc
  relay you use and to the other computer.
- **Clipboard:** croc's clipboard integration is disabled; the app only copies
  the code when you click **Copy**.

## Self-hosting a croc relay

The easiest way is the **Host** tab ([above](#hosting-a-relay-for-friends)).
To run a relay on an always-on server instead:

1. On a server reachable by both computers, install croc
   (<https://github.com/schollz/croc#install>). For example on Linux:
   ```bash
   curl https://getcroc.schollz.com | bash
   ```
   or download a release binary, or use Docker:
   ```bash
   docker run -d --restart unless-stopped -p 9009-9013:9009-9013 \
     -e CROC_PASS='choose-a-password' schollz/croc relay
   ```
2. Start the relay with a password (skip if you used Docker):
   ```bash
   CROC_PASS='choose-a-password' croc relay
   ```
   It listens on TCP ports **9009–9013** (a control port plus four transfer
   ports). Use `--port 7000` to move the whole range to 7000–7004.
3. Open those ports in the server's firewall.
4. In the app on **both** computers, open **Settings** and set:
   - Relay address: `your.server.example.com:9009`
   - Relay password: `choose-a-password`

   Then click **Save settings**.

A wrong password shows "The relay rejected the password." To keep a non-Docker
relay running after reboots, wrap the command in a systemd service, for
example:

```ini
# /etc/systemd/system/croc-relay.service
[Unit]
Description=croc relay
After=network-online.target

[Service]
Environment=CROC_PASS=choose-a-password
ExecStart=/usr/local/bin/croc relay
Restart=always

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable --now croc-relay
```

## Troubleshooting

| Message | What it means / what to do |
|---|---|
| *Wrong code, or the sender isn't sending right now.* | Check the code. The sender must click **Send** first and keep the app open. |
| *Wrong code. Double-check the code phrase and try again.* | The code doesn't match what the sender is using. |
| *Couldn't reach the relay server.* | Check your internet connection, or the relay address in Settings. Corporate firewalls may block ports 9009–9013. |
| *The relay rejected the password.* | The relay password in Settings doesn't match the one the relay was started with. |
| *That code is already in use by another transfer.* | Pick a different custom code. |
| *Connection lost, try again.* | The network dropped or the other side vanished for 45 seconds. Start again. |
| *The sender offered a file with an unsafe name.* | The transfer was blocked to protect you. Nothing was saved. |
| *Not enough disk space in the destination folder.* | Free some space or choose another folder. |
| Transfers are slow | Allow the app through Windows Firewall on private networks so LAN peers connect directly. Enable **Zip folders** for folders with many small files. |
| A `.troopers-incoming-*` folder was left behind | The app was force-closed mid-transfer. It's safe to delete. |

Both computers must use the **same relay settings**. One on the public relay
and one on a private relay will never find each other.

## Building from source

Requirements: **Go** (the version in [`go.mod`](go.mod)) and a **C compiler**,
because Fyne uses cgo for OpenGL.

```bash
git clone https://github.com/<your-username>/<your-repo>.git
cd <your-repo>
```

### Windows

```powershell
winget install GoLang.Go
winget install BrechtSanders.WinLibs.POSIX.UCRT   # MinGW-w64 gcc
```

Open a new terminal so `go` and `gcc` are on `PATH`, then:

```powershell
.\build.ps1 -Version 1.0.0
```

This produces `dist\TroopersTransfer.exe`. The script:

- builds with `-trimpath -ldflags "-s -w -H windowsgui"` (no console window),
  stamping the version into the About line;
- runs [`go-winres`](https://github.com/tc-hib/go-winres) (installed
  automatically) to embed the icon, version info and a manifest (per-monitor
  DPI, long paths, runs as invoker) from `winres/winres.json`. This is done as
  a post-link patch because linking a `.syso` clashes with the default
  manifest MinGW adds.

### Linux

```bash
sudo apt-get install gcc libgl1-mesa-dev xorg-dev libxkbcommon-dev libwayland-dev
./build.sh        # -> dist/TroopersTransfer
```

### macOS

```bash
xcode-select --install
./build.sh        # -> dist/TroopersTransfer
```

`build.sh` reads the version from the `VERSION` environment variable
(`VERSION=1.2.3 ./build.sh`).

### Running without building

```bash
go run .
```

### Regenerating the icon

The app icon (`assets/icon.png`) is drawn by code:

```bash
go run ./tools/genicon
```

## Developer CLI

`cmd/tts-cli` is a small command-line harness around the same `transfer`
package, handy for testing two instances without the GUI:

```bash
go run ./cmd/tts-cli send -code my-test-code-123 somefile.bin
go run ./cmd/tts-cli recv -out ./incoming my-test-code-123
```

| Flag | Applies to | Meaning |
|---|---|---|
| `-code CODE` | send | Use a custom code phrase. |
| `-relay HOST:PORT` | both | Use a specific relay. |
| `-pass PW` | both | Relay password. |
| `-zip` | send | Zip folders before sending. |
| `-out DIR` | recv | Destination folder (default `.`). |
| `-decline` | recv | Decline the offer (tests the decline path). |
| `-nolocal` | both | Skip LAN discovery and force the relay path. |
| `-result FILE` | both | Also write the `DONE`/`ERROR` outcome to a file. |

Set `TTS_DEBUG=1` for croc's debug log.

## Project layout

```
.
├── main.go                 entry point
├── ui/                     Fyne GUI: send, receive, host, connect, progress, history, settings
├── transfer/               croc wrapper: callbacks, friendly errors, safety checks
├── relay/                  hosted relay: in-app croc relay, UPnP/NAT-PMP, network checks, plain-English guides
├── config/                 portable JSON settings, Downloads folder lookup
├── assets/                 embedded app icon
├── tools/genicon/          program that draws assets/icon.png
├── cmd/tts-cli/            developer CLI for the transfer package
├── winres/winres.json      Windows icon, version info and manifest
├── docs/screenshots/       images used in this README
├── build.ps1               Windows build script
├── build.sh                Linux/macOS build script
└── .github/workflows/      CI: test + build for all three platforms
```

## How the croc integration works

croc is imported as a Go library (`github.com/schollz/croc/v11/src/croc`). No
croc binary is run. croc's library API has no progress or accept callbacks, so
`transfer/` adds them:

- **Progress:** croc's exported counters (`FilesToTransferCurrentNum`,
  `TotalSent`, step flags) are polled four times a second and turned into
  bytes done, speed (smoothed) and ETA.
- **Accept/Decline:** croc asks "Accept …? (Y/n)" by reading a line from a
  package-level stdin reader. A GUI has no stdin, so that reader is replaced
  (via `go:linkname`) with one that calls the GUI's `OnOffer` callback the
  moment croc asks, then answers `y` or `n`. The file list is shown before any
  file data is requested.
- **Public relay selection:** with the public relay pool, the sender picks the
  fastest relay and generates a code that maps to it; the receiver derives the
  same relay from the code, so no extra configuration is needed.
- **Timeouts:** a receiver with a mistyped code ends up on a different relay
  room and croc would wait forever, so we give up after 30 s. If a transfer
  makes no progress for 45 s (the peer vanished and croc's reconnects failed),
  it's reported as "Connection lost, try again."
- **Friendly errors:** `transfer/errors.go` maps croc's internal errors to
  plain-English messages while keeping the original for the
  "Technical details" expander.

Because croc keeps the working directory and stdin reader process-wide, only
one transfer runs at a time.

> [!WARNING]
> `go:linkname` ties this build to croc's internals. When upgrading croc,
> re-run the tests and a two-process transfer with `tts-cli`.

## Safety design

- **croc's own protection:** croc v11 normalises every incoming path
  (`receivefs.Normalize`: rejects absolute paths, `..`, drive letters) and does
  all receive I/O through an `os.Root` opened on the destination, so the OS
  refuses writes outside it. Symlink targets that escape are rejected, and
  received zip archives are validated before extraction.
- **Our checks (defense in depth):**
  1. Before answering Accept, every offered path and symlink target is checked
     again (`transfer/safety.go: safeRelative`). Anything unsafe declines the
     transfer automatically.
  2. Files are received into an empty hidden staging folder
     (`.troopers-incoming-*`) inside the destination. croc never sees your
     existing files, so it can't overwrite them, and zip extraction can't
     either.
  3. After a successful transfer the staging tree is walked. Anything outside
     it is rejected and symlinks pointing outside are removed.
  4. Each top-level item is then moved into the destination, renamed to
     `name (1).ext`, `name (2).ext`, … if the name is taken.
  5. On decline, error or cancel, the staging folder is deleted. If the app is
     force-killed mid-transfer, a `.troopers-incoming-*` folder may be left
     behind and can be deleted safely.

## Tests

```bash
go vet ./...
go test ./...
```

- `transfer/safety_test.go`: path validation, rename-on-collision,
  never-overwrite, escaping-symlink removal (the symlink case skips on Windows
  without Developer Mode).
- `ui/ui_test.go`: builds the real window with Fyne's software renderer and
  walks every screen. Set `TTS_SCREENSHOTS=<dir>` to save PNGs; that's how the
  images in this README were made.

## Continuous integration and releases

[`.github/workflows/build.yml`](.github/workflows/build.yml) runs on every push
to `main`, every pull request, and on demand. It vets, tests and builds:

| Job | Runner | Output |
|---|---|---|
| `windows-amd64` | `windows-latest` | `TroopersTransfer.exe` |
| `linux-amd64` | `ubuntu-latest` | `TroopersTransfer` |
| `macos-arm64` | `macos-latest` | `TroopersTransfer` |

Binaries are uploaded as workflow artifacts. To publish a release, push a
version tag:

```bash
git tag v1.0.0
git push origin v1.0.0
```

The workflow stamps the version into each binary and attaches
`TroopersTransfer-<platform>.zip` / `.tar.gz` to a GitHub release
automatically.

## Contributing

Bug reports and pull requests are welcome.

1. Fork the repository and create a branch: `git checkout -b my-change`.
2. Make your change and run `go vet ./...` and `go test ./...`.
3. If you touched `transfer/`, try a real transfer between two terminals with
   `tts-cli` (add `-nolocal` to exercise the relay path).
4. Open a pull request describing what changed and why.

Please keep the app portable: no installers, registry writes, or files written
outside the settings file and the chosen destination folder.

## License and credits

Troopers Transfer is released under the [MIT License](LICENSE).

Built on:

- [croc](https://github.com/schollz/croc) by Zack Scholl (MIT License)
- [Fyne](https://fyne.io) (BSD 3-Clause License)
- [go-qrcode](https://github.com/skip2/go-qrcode) (MIT License)

vibe coded by trooper, with ollama, gemma4 and claude
