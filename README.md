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
  <img src="assets/screenshot-instance.png" alt="Instance table" width="100%">
</p>

<p align="center">
  <img src="assets/screenshot-security-list.png" alt="Security List table" width="49%">
  <img src="assets/screenshot-security-list-rules.png" alt="Security List rules view" width="49%">
</p>

Read-only by default. Write actions (instance start/stop, Bastion SSH sessions) are gated behind an explicit `--write` flag and a type-the-resource-name confirmation.

## Features

- **Resource search** — press `f` for a centered fuzzy picker over every resource kind, jump straight to one.
- **Compartment navigation** — lazy drill-down (`Enter` to descend, `Esc` to go up), no tenancy-wide `inspect` permission required.
- **VCN-scoped filtering** — pick a VCN and every VCN-scoped resource (Subnets, Route Tables, Security Lists, NSGs, Instances, Load Balancers, DB Systems, Autonomous DBs, Exadata VM Clusters) filters down to just that VCN.
- **12 resource kinds**: Compartments, Instances, VCNs, Subnets, Route Tables, Security Lists, NSGs, DRGs, Load Balancers, DB Systems, Autonomous Databases, Exadata VM Clusters.
- **Instance table** with live CPU%/MEM% (OCI Monitoring), OCPU/memory spec, public/private IP, AD/FD, and a colored RUNNING/STOPPED state badge.
- **Security List rule viewer** — ingress/egress rules as a readable table instead of raw nested YAML.
- **CSV export** (UTF-8 BOM, opens cleanly in Excel) for whatever's currently on screen — including the Security List rules table.
- **Mermaid diagram export** — generates a `.mmd` flowchart (`graph TD` + nested `subgraph`) of a VCN's subnets, the Instances/DB Systems/Autonomous DBs/Exadata VM Clusters in each, and any DRGs attached to the VCN.
- **LazyVim-style shortcuts popup** — press `space` for a which-key-style overlay of every binding that applies to the current screen.
- **Region switcher**, local fuzzy filter, live refresh.
- **Bastion SSH** — resolve an instance's private IP, create a Bastion session, and drop straight into an SSH shell.

## Prerequisites

- Go 1.26 or newer (only needed to build from source).
- An OCI CLI-style config file at `~/.oci/config` with at least one profile (the same file the [OCI CLI](https://docs.oracle.com/en-us/iaas/Content/API/SDKDocs/cliinstall.htm) uses).
- IAM permissions to `read` (or `manage`, if using `--write`) the resource types you want to browse in your tenancy/compartment.

## Installation

The binary is a static, pure-Go executable — no runtime dependencies beyond your `~/.oci/config`.

### From a Release

```bash
curl -sL https://github.com/juseok1729/toci/releases/latest/download/toci-x86_64-unknown-linux-gnu.tar.gz | tar xz
sudo mv toci /usr/local/bin/
```

Swap the filename for your platform — available targets on the [Releases page](https://github.com/juseok1729/toci/releases/latest):

| Platform | Target |
| --- | --- |
| Linux x86_64 | `toci-x86_64-unknown-linux-gnu.tar.gz` |
| Linux arm64 | `toci-aarch64-unknown-linux-gnu.tar.gz` |
| macOS Intel | `toci-x86_64-apple-darwin.tar.gz` |
| macOS Apple Silicon | `toci-aarch64-apple-darwin.tar.gz` |

Each release also ships a `checksums.txt` for verification.

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
./toci                                    # profile: $OCI_CLI_PROFILE or DEFAULT
./toci --profile ETEVERS                # a specific profile
./toci --profile ETEVERS --region us-ashburn-1   # override region (default: profile's region)
./toci --profile ETEVERS --write        # enable write actions (instance start/stop, Bastion SSH)
```

On startup you'll land on the tenancy root's Compartments list. Drill down with `Enter`; if a compartment has no sub-compartments, toci lands you on its VCNs instead.

## Key Bindings

| Key | Action |
| --- | --- |
| `j` / `k` (or arrow keys) | Move up/down |
| `Enter` | Compartment: descend · VCN: filter all VCN-scoped resources to it, same as `i`, then opens the resource search to pick one · everything else: no-op |
| `d` | View detail (YAML) for the selected row, any resource kind |
| `Esc` | Close detail → clear filter → back out of a VCN filter → go up a compartment (whichever applies first) |
| `Tab` | Cycle to the next resource kind |
| `f` / `:` | Search resource kinds in a centered picker and jump to one |
| `/` | Filter the current list by name |
| `r` | Switch region (subscribed regions only) |
| `R` | Refresh the current list |
| `e` | Export the current view to CSV (UTF-8 BOM) |
| `i` | *(on a VCN row)* Filter all VCN-scoped resources to this VCN, same as `Enter`, then opens the resource search to pick one |
| `v` | *(on a Security List row)* View ingress/egress rules as a table |
| `m` | *(with a VCN filter active)* Export a Mermaid diagram of the VCN's topology |
| `a` | *(Instance, `--write` only)* Action menu — start/stop, with a type-to-confirm prompt |
| `s` | *(Instance, `--write` only)* SSH via Bastion |
| `space` | Toggle the shortcuts popup |
| `q` / `Ctrl-C` | Quit |

## Documentation

- [`docs/USAGE.md`](docs/USAGE.md) — original usage notes (Korean)
- [`docs/PROGRESS.md`](docs/PROGRESS.md) — implementation log and design decisions (Korean)
- [`docs/COLOR_SYSTEM.md`](docs/COLOR_SYSTEM.md) — color system reference (Korean)

## License

MIT — see [LICENSE](LICENSE).
