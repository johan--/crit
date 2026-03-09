# Changelog

## 0.2.1

### Fixed

- `crit review --code --detach --wait` now correctly opens in a tmux split pane instead of failing with a TTY error

## 0.2.0

### Added

- **Multi-file code review** — `crit review --code` detects changed files in your git repo and opens a tabbed TUI with syntax highlighting and diff markers
- **Tabbed file navigation** — `tab`/`shift+tab` to switch files, `/` to search
- **Change navigation** — `n`/`N` to jump between changed lines within a file
- **Aggregate status** — `crit status --code` outputs comments across all reviewed files as JSON
- **Session management** — code review sessions are persisted to `.crit/code-review.yaml`
- **Review router skill** — `/crit:review` asks whether to review code or a document, then routes to the appropriate workflow
- **Code review skill** — `/crit:code-review` runs the full code review workflow in Claude Code
- **Plan review skill** — `/crit:plan-review` routes single-file document reviews

### Changed

- `tab` now switches between file tabs in code review mode (was: switch panes)
- `s` toggles the comment sidebar (was: `tab`)
- Review skill restructured into router pattern with separate code and plan review skills

## 0.1.0

Initial release.

- Interactive TUI for reviewing markdown documents
- Inline comments with visual select mode
- tmux split pane integration (`--detach --wait`)
- Scriptable CLI (`crit comment`, `crit status`)
- Claude Code skill (`/crit-review`)
- Shell completions (bash, zsh, fish)
