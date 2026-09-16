<p align="center">
  <img src="assets/toci-logo-red.png" alt="toci logo" width="400">
</p>

# toci - Terminal UI for OCI

A fast, keyboard-driven terminal UI for browsing and managing Oracle Cloud Infrastructure (OCI) — compartments, compute, networking, and database resources — without leaving your terminal.

---

[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26%2B-00ADD8.svg)](https://go.dev/)
[한국어](README.ko.md)

---

## Screenshots

<p align="center">
  <img src="assets/screenshot-inst.png" alt="Instance table" width="100%">
</p>

<p align="center">
  <img src="assets/screenshot-sl.png" alt="Security List table" width="49%">
  <img src="assets/screenshot-sl-rules.png" alt="Security List rules view" width="49%">
</p>

Read-only by default. Write actions (instance start/stop, Bastion SSH sessions) are gated behind an explicit `--write` flag and a type-the-resource-name confirmation.

## Features

- **Resource search** — press `:` for a two-pane fuzzy picker (list + description) over every resource kind, jump straight to one.
- **Compartment switching** — press `c` for a fuzzy compartment tree picker; Compartments themselves are an info-only view (`Enter`/`d` shows detail).
- **VCN/DRG-scoped pickers** — `Enter` (or `i`) on a VCN row floats a picker over just its Subnets/Route Tables/Security Lists/Gateways; a DRG row does the same for its Attachments/Route Tables/Route Distributions.
- **VCN-scoped filtering** — pick a VCN and every VCN-scoped resource (Subnets, Route Tables, Security Lists, NSGs, Instances, Load Balancers, Internet/NAT/Service Gateways, OKE Clusters, DB Systems, Autonomous DBs, Exadata VM Clusters) filters down to just that VCN.
- **24 resource kinds** across Compute, Network, Gateways, Storage, Containers, and Database — see the `:` search for the full, categorized list.
- **Recently Created** — a home-screen shortcut (also in the `:` search) listing every resource created in the last 3 days, across every kind, newest first. Creation-only: OCI's list APIs don't expose a last-modified timestamp, so this can't track edits to existing resources.
- **Instance table** with live CPU%/MEM% (OCI Monitoring), OCPU/memory spec, OS image version, subnet, public/private IP, and a colored STATE column (every resource kind gets green/red/yellow text for healthy/failed/needs-attention states — see [docs/COLOR_SYSTEM.md](docs/COLOR_SYSTEM.md)).
- **Rules viewer** — `v` on a Security List/Route Table/NSG/DRG Route Table row floats its ingress/egress or route rules as a table over the bottom of the screen, instead of raw nested YAML.
- **CSV export** (UTF-8 BOM, opens cleanly in Excel) for whatever's currently on screen — including a rules table.
- **Mermaid diagram export** — generates a `.mmd` flowchart (`graph TD` + nested `subgraph`) of a VCN's subnets, the Instances/DB Systems/Autonomous DBs/Exadata VM Clusters in each, and any DRGs attached to the VCN.
- **Resource map** — an in-app, AWS-console-style view of a VCN's Subnets, the Route Tables they use, and the Internet/NAT/Service/Local Peering Gateways and DRGs those route tables target, connected column to column.
- **LazyVim-style shortcuts popup** — press `space` for a which-key-style overlay of every binding that applies to the current screen.
- **Region switcher**, local fuzzy filter, live refresh.
- **Bastion SSH** — resolve an instance's private IP, create a Bastion session, and drop straight into an SSH shell.

## Prerequisites

- Go 1.26 or newer (only needed to build from source).
- An OCI CLI-style config file at `~/.oci/config` with at least one profile (the same file the [OCI CLI](https://docs.oracle.com/en-us/iaas/Content/API/SDKDocs/cliinstall.htm) uses).
- IAM permissions to `read` (or `manage`, if using `--write`) the resource types you want to browse in your tenancy/compartment.

## Installation

The binary is a static, pure-Go executable (`CGO_ENABLED=0`) — no glibc dependency, so it runs unmodified on Oracle Linux, RHEL, Ubuntu/Debian, other distros, and WSL.

### 1. Install script (any Linux or macOS)

```bash
curl -fsSL https://raw.githubusercontent.com/juseok1729/toci/master/install.sh | sh
```

Downloads the right release for your OS/arch, verifies it against `checksums.txt`, and installs to `/usr/local/bin` (falls back to `~/.local/bin` if that's not writable and you're not root — no `sudo` required).

### 2. dnf (Oracle Linux / RHEL / Fedora)

```bash
curl -1sLf 'https://dl.cloudsmith.io/public/toci/toci/setup.rpm.sh' | sudo -E bash
sudo dnf install toci
```

### 3. apt (Debian / Ubuntu)

```bash
curl -1sLf 'https://dl.cloudsmith.io/public/toci/toci/setup.deb.sh' | sudo -E bash
sudo apt install toci
```

### 4. Homebrew (macOS / Linuxbrew)

```bash
brew install juseok1729/toci/toci
```

On macOS, Gatekeeper will refuse to run it and offer to move it to the Trash — the binary isn't code-signed/notarized yet. Clear the quarantine flag once after installing:

```bash
xattr -d com.apple.quarantine "$(brew --prefix)/bin/toci"
```

### 5. Manual download / `go install`

Download a `toci_<os>_<arch>.tar.gz` from the [Releases page](https://github.com/juseok1729/toci/releases/latest) (each release also ships a `checksums.txt`), or build from source:

```bash
go install github.com/juseok1729/toci/cmd/toci@latest
```

### Upgrading

| Installed via | Command |
| --- | --- |
| Install script | Re-run the same `curl \| sh` command from method 1 |
| dnf | `sudo dnf upgrade toci` |
| apt | `sudo apt update && sudo apt upgrade toci` |
| Homebrew | `brew upgrade toci` |

### From Source

```bash
git clone git@github.com:juseok1729/toci.git
cd toci
go build -o toci ./cmd/toci
```

For a smaller binary (this is what the release builds use):

```bash
go build -ldflags="-s -w" -trimpath -o toci ./cmd/toci
```

Cross-compile for another platform with `GOOS`/`GOARCH` (e.g. `GOOS=darwin GOARCH=arm64 go build ...`).

## Quick Start

```bash
./toci                                      # profile: $OCI_CLI_PROFILE or DEFAULT
./toci --profile DEV                        # a specific profile
./toci --profile DEV --region us-ashburn-1  # override region (default: profile's region)
./toci --profile DEV --write                # enable write actions (instance start/stop, Bastion SSH)
```

## Key Bindings

| Key | Action |
| --- | --- |
| `j` / `k` (or arrow keys) | Move up/down |
| `Enter` | Compartment: view detail (YAML) · VCN: float a picker over its Subnets/Route Tables/Security Lists/Gateways, same as `i` · DRG: float a picker over its Attachments/Route Tables/Route Distributions · Instance (`--write` only): float the action menu (start/stop, with a type-to-confirm prompt) · everything else: no-op |
| `d` | View detail (YAML) for the selected row, any resource kind |
| `Esc` | Close whatever window/view is open (detail, resource map, rules view, the `:` search, ...) |
| `Backspace` | Go back: clear the filter, then step back through the resources you've actually visited one at a time (e.g. VCN → Subnet → DB System → Subnet → VCN) |
| `Tab` | Cycle to the next resource kind |
| `:` | Search every resource kind in a two-pane picker (list + description) and jump to one |
| `/` | Filter the current list by name |
| `r` | Switch region (subscribed regions only) |
| `R` | Refresh the current list |
| `c` | Switch compartment (fuzzy tree picker) |
| `C` | Toggle subtree mode (fan the current resource out across every sub-compartment) |
| `e` | Export the current view to CSV (UTF-8 BOM) |
| `i` | *(on a VCN or DRG row)* Same as `Enter` on that row |
| `v` | *(on a Security List/Route Table/NSG/DRG Route Table row)* Float its rules (ingress/egress or route rules) as a table over the bottom of the screen |
| `m` | *(with a VCN filter active)* Export a Mermaid diagram of the VCN's topology |
| `M` | *(with a VCN filter active)* View the VCN's resource map (Subnets/Route Tables/Network Connections) |
| `s` | *(Instance, `--write` only)* SSH via Bastion |
| `space` | Toggle the shortcuts popup |
| `q` / `Ctrl-C` | Quit |

## Documentation

- [`docs/USAGE.md`](docs/USAGE.md) — original usage notes (Korean)
- [`docs/PROGRESS.md`](docs/PROGRESS.md) — implementation log and design decisions (Korean)
- [`docs/COLOR_SYSTEM.md`](docs/COLOR_SYSTEM.md) — color system reference (Korean)

## License

MIT — see [LICENSE](LICENSE).
