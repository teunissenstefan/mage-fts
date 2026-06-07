# mage-fts

Full-text search across every table and column of a local DDEV database or remote Magento database. Useful for tracking down where a specific value lives without knowing the schema upfront.

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

### Local DDEV database

Run from inside a DDEV project directory:

```bash
mage-fts "search term"
```

### Remote database via SSH

```bash
mage-fts "search term" --ssh-json='{"username":"user","server":"example.com","port":22}'
```

Connects over SSH, auto-detects the Magento root using `bin/magento`, retrieves database credentials via magerun, and runs the search through an SSH tunnel.

## Using with [server](https://github.com/teunissenstefan/server-mage-db)

`server` is a companion command that lets you pick an SSH server interactively via a fuzzy finder. Its `--json` flag outputs the selected server as JSON, which can be piped directly into `mage-fts`:

```bash
mage-fts --ssh-json="$(server --json)" "search term"
```

This opens the server picker, and once you select a server the search runs against its database automatically.
