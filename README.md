# mage-fts

Full-text search across every table and column of a local DDEV database, remote Magento database, or remote WordPress database. Useful for tracking down where a specific value lives without knowing the schema upfront.

## Install

```bash
brew install epenthesis/tap/mage-fts
```

Or, if you would rather tap once and use short names afterwards:

```bash
brew tap epenthesis/tap
brew install mage-fts
```

With a Go toolchain:

```bash
go install github.com/epenthesis/mage-fts@latest
```

Or from a checkout:

```bash
go build -o mage-fts .
```

### Updating

```bash
brew update && brew upgrade mage-fts        # or `brew upgrade` for everything at once
```

## Usage

```bash
mage-fts <search-term> [options]
```

### Options

| Flag | Default | Description |
|---|---|---|
| `--limit=N` | 20 | Max results per table |
| `--include=PATTERN` | | Only search matching tables (glob) |
| `--exclude=PATTERN` | | Exclude matching tables (glob) |
| `--column-limit=N` | 5 | Number of columns to display per row |
| `--truncate-length=N` | 50 | Max character length per column value |
| `--no-truncate` | | Disable column truncation |
| `--dry-run` | | Print queries without executing them |
| `--ssh-json=JSON` | | Connect to a remote server via SSH |
| `--wp` | | Search a remote WordPress database (requires `--ssh-json=`) |
| `--output=FORMAT` | `table` | Output format: `table` or `raw` |

### Local DDEV database

Run from inside a DDEV project directory:

```bash
mage-fts "search term"
```

### Remote Magento database via SSH

```bash
mage-fts "search term" --ssh-json='{"username":"user","server":"example.com","port":22}'
```

Connects over SSH, auto-detects the Magento root by searching for `bin/magento`, retrieves database credentials via magerun, and runs the search through an SSH tunnel.

### Remote WordPress database via SSH

```bash
mage-fts "search term" --ssh-json='{"username":"user","server":"example.com","port":22}' --wp
```

Connects over SSH, searches the remote filesystem for `wp-config.php`, reads the database credentials from it, and runs the search through an SSH tunnel.

## Using with [server](https://github.com/epenthesis/server-mage-db)

`server` is a companion command that lets you pick an SSH server interactively via a fuzzy finder. Its `--json` flag outputs the selected server as JSON, which can be piped directly into `mage-fts`:

```bash
# Remote Magento database
mage-fts --ssh-json="$(server --json)" "search term"

# Remote WordPress database
mage-fts --ssh-json="$(server --json)" --wp "search term"
```

This opens the server picker, and once you select a server the search runs against its database automatically.
