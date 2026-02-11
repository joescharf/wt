# VHS Tape Files

Terminal recordings for wt documentation, built with [VHS](https://github.com/charmbracelet/vhs).

## Prerequisites

```bash
brew install vhs
```

## Usage

Generate all screenshots:

```bash
make all
```

Generate a single tape:

```bash
vhs wt-help.tape
```

Or use the Makefile convenience targets:

```bash
make help-tape
make list-tape
make workflow-tape
```

## Tapes

| Tape | Description | Best for |
|------|-------------|----------|
| `wt-help.tape` | Main help output | Commands reference page |
| `wt-list.tape` | Worktree listing with status | Commands page, README |
| `wt-create.tape` | Create command help + dry-run | Getting started page |
| `wt-config.tape` | Config show output | Configuration page |
| `wt-workflow.tape` | Full lifecycle demo | README hero, blog posts |

## Tips

- **Best results for `wt-list`**: Have 2-3 active worktrees in different states before recording
- **Re-record after changes**: Run `make clean && make all` to regenerate everything
- **Custom themes**: Edit the `Set Theme` line in any tape (run `vhs themes` to see options)
- Output goes to `docs/assets/screenshots/` as both `.gif` and `.webm`
