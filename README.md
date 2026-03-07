# Crit

A terminal-based review tool for markdown documents. Read a plan, leave inline comments, and let Claude Code address your feedback automatically.

Built for the human-in-the-loop workflow: Claude writes a plan, you review it in a TUI, Claude reads your comments and edits the document.

![crit demo](demo/demo.gif)

## Install

```bash
go install github.com/kevindutra/crit/cmd/crit@latest
```

## Claude Code Integration

### Plugin (recommended)

```
/plugin install crit
```

Then use `/crit:review <path>` to open the TUI. After you close it, Claude reads your comments and edits the document to address them.

### Manual skill install

```bash
crit setup-claude          # Install globally (~/.claude/skills/)
crit setup-claude --project # Install for current project only
```

Then use `/crit-review <path>` in Claude Code.

## Requirements

- **Go 1.21+** for building from source
- **tmux** — required for the Claude Code integration. Crit opens the review TUI in a tmux split pane next to Claude Code.

### Starting a tmux session

If you're not already in tmux, start one before launching Claude Code:

```bash
tmux new -s work
# Now launch Claude Code inside this tmux session
claude
```

If you forget, crit will tell you — but the split-pane review won't work outside of tmux.

## Interactive Review (TUI)

```bash
crit review docs/plans/my-plan.md
```

Opens a full-screen terminal UI with syntax-highlighted markdown, a comment sidebar, and modal overlays for adding/editing comments.

### tmux split pane mode

When running inside tmux, you can open the TUI in a side-by-side split pane:

```bash
# Open review in a tmux split and return immediately
crit review docs/plan.md --detach

# Open review in a tmux split and block until it closes
crit review docs/plan.md --detach --wait
```

This is how the Claude Code skill invokes crit — `--detach --wait` is a single blocking call that opens the TUI next to Claude Code and waits for you to finish reviewing.

**Keybindings:**

| Key | Action |
|-----|--------|
| `j` / `k` | Scroll down / up |
| `ctrl+d` / `ctrl+u` | Half page down / up |
| `g` / `G` | Jump to top / bottom |
| `enter` | Add comment at current line |
| `v` | Visual select mode (multi-line comments) |
| `tab` | Switch between content and comment panes |
| `[` / `]` | Jump to prev / next comment |
| `q` | Save & quit |

## Scriptable CLI

```bash
# Add a comment programmatically
crit comment docs/plan.md --line 15 --body "This needs more detail"

# Multi-line comment
crit comment docs/plan.md --line 10 --end-line 20 --body "Rethink this section"

# Get review comments as JSON
crit status docs/plan.md
```

## How It Works

1. Claude writes a plan (or you open any markdown file)
2. `crit review <path>` opens the TUI — read through and leave inline comments
3. Comments are stored as JSON in a local `.crit/` directory (gitignored by default)
4. `crit status <path>` outputs comments as JSON for Claude (or any tool) to consume
5. Claude reads the comments, edits the document, and you can re-review

## Shell Completions

```bash
# Bash
crit completion bash > /etc/bash_completion.d/crit

# Zsh
crit completion zsh > "${fpath[1]}/_crit"

# Fish
crit completion fish > ~/.config/fish/completions/crit.fish
```

## Development

```bash
go test ./...
go build ./...
go vet ./...
```

## License

MIT
