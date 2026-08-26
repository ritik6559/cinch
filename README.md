# cinch

A coding agent that runs in your terminal. It reads, searches and edits files in
the directory you start it from, runs commands to check its own work, and asks
before it changes anything.

cinch is a single Go binary. It talks to the OpenAI Responses API in stateless
mode, which means OpenAI stores nothing: cinch keeps the whole conversation
itself, and can save and resume it.

> Early work in progress. The agent loop, the tools, approval, streaming,
> sessions, compaction and the terminal interface all work. There is no sandbox
> and there are no subagents.

## Requirements

- Go 1.26 or newer
- An OpenAI API key
- A POSIX shell for the `bash` tool. On Windows, Git Bash provides one.
- [ripgrep](https://github.com/BurntSushi/ripgrep) (optional, makes search
  faster and skips files listed in `.gitignore`)

## Install

```bash
git clone https://github.com/ritik6559/cinch.git
cd cinch
make build
```

The binary is written to `bin/cinch`. Use `make install` to put it on your PATH
instead.

## Configure

Copy the example file and add your key:

```bash
cp .env.example .env
```

| Variable | Default | Meaning |
|---|---|---|
| `OPENAI_API_KEY` | — | Your API key. Required |
| `OPENAI_MODEL` | `gpt-5.6` | Which model to use |
| `CINCH_PROVIDER` | `openai` | `openai` or `openai-compatible` |
| `CINCH_BASE_URL` | — | Address of an OpenAI-compatible Responses API |
| `CINCH_COMPACT_AT` | `100000` | Token count that triggers compaction. `0` disables it |
| `CINCH_EFFORT` | — | How hard the model thinks: `none`…`max`. Unset lets the API decide |
| `CINCH_NO_TUI` | — | Set to anything to use the plain line prompt |

Real environment variables win over `.env`, so this also works:

```bash
OPENAI_MODEL=gpt-5.6-mini cinch
```

Check that everything is set up:

```bash
cinch doctor
```

## Use

Run `cinch` inside the project you want to work on. The directory you start in
becomes the workspace.

```
› where is the step limit defined?

⏺ grep maxSteps
  ⎿ 3 matches

internal/agent/agent.go:14 defines it as 25.

› raise it to 40 and run the tests

  edit_file  internal/agent/agent.go
  - maxSteps = 25
  + maxSteps = 40

  Allow?  y yes · n no · a this session · s always

✻ Cerebrating… (12s · ↑ 1.2k tokens · esc to interrupt)
```

The status line under the conversation shows the model, the reasoning effort,
how much context the session is using, and the workspace name.

When stdin is not a terminal, cinch drops to a plain line prompt instead, so
pipes and scripts keep working:

```bash
printf 'what does this package do?\n' | cinch
```

`--no-tui` forces the plain prompt in a terminal too.

### Commands

| Command | Meaning |
|---|---|
| `cinch` | Start a chat session (same as `cinch chat`) |
| `cinch sessions` | List saved sessions |
| `cinch approvals` | List saved approvals |
| `cinch approvals rm <prefix>` | Take a saved approval back |
| `cinch doctor` | Check that the local setup is complete |
| `cinch version` | Print version information |
| `cinch help` | Show all commands |

### Flags

Flags must come before the command name.

| Flag | Meaning |
|---|---|
| `-c`, `--continue` | Resume the most recent session |
| `--resume id` | Resume a saved session by id |
| `-v`, `--version` | Print version and exit |
| `--debug` | Print extra information |
| `--cwd dir` | Run as if cinch started in `dir` |
| `--no-tui` | Use the plain line prompt |
| `-h`, `--help` | Show help |

### While chatting

Type `/` to see the commands. The list filters as you keep typing, Tab
completes, and ↑/↓ then Enter picks one.

| Command | Meaning |
|---|---|
| `/model` | Pick a model from the list the API reports. `/model <id>` sets it directly |
| `/effort` | Pick how hard the model thinks |
| `/compact` | Shrink the conversation now |
| `/sessions` | Pick a saved session to switch to |
| `/resume <id>` | Resume a session by id |
| `/approvals` | Review saved approvals, Enter removes one |
| `/cost` | Token usage for this session |
| `/clear` | Start a new conversation |
| `/help` | List the commands |
| `/quit` | Save and exit |

`/model` and `/effort` take effect on the next turn and are recorded in the
session, so resuming it reports what it actually used.

| Key | Meaning |
|---|---|
| Enter | Send |
| Shift-Enter | New line |
| Esc | Cancel the turn, or dismiss a picker |
| Ctrl-C | Cancel the turn; again at an empty prompt quits |
| Ctrl-D | Quit |
| Tab | Complete a command |

Scrolling the conversation:

| Key | Meaning |
|---|---|
| Mouse wheel | Scroll |
| PgUp / PgDn | Half a page |
| Shift-↑ / Shift-↓ | One line |
| End | Jump back to the newest output |

Scrolling up **stops the view following new output**, so a reply streaming in
cannot drag you away from what you are reading. A hint appears showing how many
lines are below you; `End` or scrolling back to the bottom resumes following.

Plain ↑/↓ belong to the prompt box, and `End` only scrolls when you are already
scrolled up — at the bottom it moves the text cursor as usual.

## Tools

The model can use these, and nothing else:

| Tool | Needs approval | Purpose |
|---|---|---|
| `read_file` | no | Read a file, line numbered and size capped |
| `list_files` | no | List one directory |
| `grep` | no | Search file **contents** with a regular expression |
| `glob` | no | Find files by **name**, with `*`, `**` and `?` |
| `write_file` | yes | Create a file or replace it completely |
| `edit_file` | yes | Replace an exact string in a file |
| `bash` | yes | Run a shell command and return its output and exit status |

## Approvals

`write_file`, `edit_file` and `bash` ask before they run. The prompt shows what
is about to happen, and the default answer is no:

```
  bash  go test ./...

  Allow?  y yes · n no · s always
```

For `edit_file` the prompt shows the actual diff, so you can see the change
rather than a one-line description of it.

| Answer | Meaning |
|---|---|
| `y` | Allow this one call |
| `n`, Enter, anything else | Refuse |
| `a` | Allow this tool for the rest of the session (not offered for `bash`) |
| `s` | Save it, so it never asks again |

Answering `s` on a `bash` prompt saves a **command prefix**, not the whole
command. `go test ./internal/agent` saves as `go test`, so it covers the next
run with different arguments — but approving `go test` never approves `rm`.
The prefix ends at a word boundary, so `go test` does not match `go testify`.

```bash
cinch approvals                  # see what is saved
cinch approvals rm "go test"     # take one back
```

Approvals live in `~/.cinch/approvals.json`.

`a` is deliberately not offered for `bash`. Allowing every shell command for a
whole session is a much larger grant than the prompt appears to be asking for,
and `s` covers the real need better.

## Project instructions

If the workspace root has an `AGENTS.md`, cinch appends it to its system prompt.
Use it to record what a newcomer would need: the test command, the directories
to leave alone, the conventions of the codebase.

`AGENTS.md` is a shared convention rather than something specific to cinch, so a
repository that already has one works with no extra setup.

It is sent on every turn, so keep it short — anything over 32 KB is truncated.
Project instructions cannot override cinch's own rules about approval or
workspace confinement.

## Sessions

Conversations are saved to `~/.cinch/sessions` after every turn, so a crash or a
closed terminal does not lose your work.

```bash
cinch --continue      # pick up the most recent conversation
cinch --resume <id>   # pick up a specific one
cinch sessions        # list what is saved
```

Session files hold everything cinch read, including file contents. They are
written owner-only, but keep that in mind when working on a private repository.

## Compaction

Every turn re-sends the whole conversation, so a long session grows until it
will not fit. cinch shrinks it in two layers once a request passes
`CINCH_COMPACT_AT` tokens.

**Clearing** empties the contents of old tool results. Nothing is deleted: each
result keeps its id and becomes a short note saying how much was cleared, so the
model can run the tool again if it needs the output. The six most recent results
are always kept.

**Summarizing** runs only when clearing was not enough. The older part of the
conversation is replaced by a summary written by the model, and recent messages
are kept exactly as they are. It costs an inference and it is lossy, which is
why it is second.

Type `/compact` to shrink the conversation by hand.

## Safety

- **Workspace confinement.** Every path the model gives is checked before use.
  Absolute paths, `..` traversal, Windows drive letters and UNC paths are
  rejected on all platforms.
- **Secret files.** `.env`, `.env.local`, `.netrc` and `.npmrc` cannot be read.
  Otherwise your API key would end up in the transcript, and from there in the
  next request.
- **Secrets are stripped from commands.** Environment variables whose names
  contain `KEY`, `TOKEN`, `SECRET`, `PASSWORD` or `CREDENTIAL` are removed
  before a `bash` command runs, so `env` cannot leak your key.
- **Approval.** Tools that change files or run commands ask first, and the
  default answer is no. See [Approvals](#approvals).
- **Step limit.** One request runs at most 25 model turns, so a confused model
  cannot loop forever.

One limit worth knowing: **`bash` breaks workspace confinement.** `cd ..` works,
and the path checks do not apply to a shell. Approval is the only control on it,
which is why saved approvals for `bash` are command prefixes rather than a
blanket allow.

cinch does not sandbox anything. Run it in a git repository, so you can always
see and undo what it changed.

## Development

```bash
make check    # everything CI runs: vet, test, gofmt, build
make test
make fmt
```

CI builds and tests on Linux and Windows. Both matter: the path handling code
behaves differently on each.

See [AGENTS.md](AGENTS.md) for the layout and conventions.
