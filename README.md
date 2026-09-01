# cinch

A coding agent that runs in your terminal. It reads, searches and edits files in
the directory you start it from, runs commands to check its own work, and asks
before it changes anything.

cinch is a single Go binary. It talks to the OpenAI Responses API in stateless
mode, which means OpenAI stores nothing: cinch keeps the whole conversation
itself, and can save and resume it.

> Early work in progress. The agent loop, the tools, approval, streaming,
> sessions, compaction and the terminal interface all work. Shell commands are
> judged before they run, and on Linux they can be confined by the kernel.
> A repository can carry its own settings and skills. There are no subagents yet.

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

### Settings for a project

A repository can carry its own settings in `.cinch/config.yaml`, committed
alongside the code so everyone working on it gets the same behaviour:

```yaml
# .cinch/config.yaml
model: gpt-5.6-mini
effort: high
compact_at: 60000
sandbox: confined
```

The same file works in your home directory, `~/.cinch/config.yaml`, for
preferences that follow you rather than the project.

Settings are read in layers, each one beating the last:

| | |
|---|---|
| built-in defaults | |
| `~/.cinch/config.yaml` | your own preferences |
| `<project>/.cinch/config.yaml` | what this repository asks for |
| `.env` | |
| the environment | `CINCH_MODEL=… cinch`, for one run |

**A project file is only half-trusted.** It arrived with a repository you may
never have read, so it may make cinch more careful but never less:

- `sandbox: confined` is accepted — the project is being helpful
- `sandbox: off` is refused
- `provider` and `base_url` are refused outright. They decide where your API key
  is sent, and a repository does not get to answer that

There is no `api_key` field. Keys stay in the environment, where they cannot end
up in git history.

Run `cinch doctor` to see which files were read — and which settings a variable
is overriding, which is nearly always the answer to "why is my config being
ignored":

```
warn  settings   /home/you/code/app/.cinch/config.yaml — but the environment wins: model overridden by OPENAI_MODEL
```

### Environment variables

These override every file:

| Variable | Default | Meaning |
|---|---|---|
| `OPENAI_API_KEY` | — | Your API key. Required |
| `OPENAI_MODEL` | `gpt-5.6` | Which model to use |
| `CINCH_PROVIDER` | `openai` | `openai` or `openai-compatible` |
| `CINCH_BASE_URL` | — | Address of an OpenAI-compatible Responses API |
| `CINCH_COMPACT_AT` | `100000` | Token count that triggers compaction. `0` disables it |
| `CINCH_EFFORT` | — | How hard the model thinks: `none`…`max`. Unset lets the API decide |
| `CINCH_SANDBOX` | `policy` | How `bash` is judged: `off`, `policy`, `strict` or `confined` |
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
| `cinch skills` | List the skills available here |
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
| `skill` | no | Read one of this repository's skills. Only offered when there are some |
| `write_file` | yes | Create a file or replace it completely |
| `edit_file` | yes | Replace an exact string in a file |
| `bash` | depends | Run a shell command. See [Approvals](#approvals) |

## Approvals

`write_file` and `edit_file` always ask. `bash` is judged first, and what
happens next depends on what the command does:

| | |
|---|---|
| **Runs** | Reading inside the workspace: `ls`, `cat go.mod`, `git status`, `grep -rn x internal` |
| **Asks** | Anything with effects, anything reaching the network, anything reading or writing outside the workspace |
| **Refused** | `rm -rf /`, `mkfs`, `dd of=/dev/…`, `shutdown`, fork bombs, `curl … \| sh` |

A refused command never reaches the shell, and answering the prompt is not
offered — the model is told why and asked to suggest something else.

When cinch asks, it says what it noticed:

```
  warning: curl reaches the network
allow run: curl -s https://example.com? [y/N/s]
```

The judgement is a filter on accidents, **not a security boundary**. A command
it cannot read — command substitution, `eval`, a nested `bash -c` — is escalated
to you rather than allowed, but nothing stops a command you approve.

`CINCH_SANDBOX` chooses how much of this applies:

| Mode | Meaning |
|---|---|
| `off` | No judgement. `bash` asks every time and nothing is refused |
| `policy` | The default, as above |
| `strict` | The same refusals, but reads ask too |
| `confined` | `policy`, plus the kernel enforces the workspace. **Linux only** |

`confined` is the only one that is a real boundary — see
[Confinement](#confinement).

The prompt shows what is about to happen, and the default answer is no:

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

A saved prefix describes **one** command. Before matching, the line is split on
`;`, `&&`, `||`, `|` and `&`, and every part must be covered by some rule:

```
saved: go test

go test ./...              allowed
go build && go test        allowed only if `go build` is saved too
go test && rm -rf ~        asks
go test > ~/.bashrc        asks
```

Some lines cannot be described by a prefix at all — command substitution
(`$(…)`, backticks), `eval`, a nested `bash -c`, an unbalanced quote. Those are
never matched against saved rules and always ask, and `s` refuses to save them
rather than storing a rule that would not mean what it says.

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

## Skills

A skill is a set of instructions for one kind of work, kept in the repository:

```
.cinch/skills/
  release-notes/SKILL.md
  deploy/SKILL.md
```

Each file names itself and says when it applies:

```markdown
---
name: release-notes
description: The format for release notes here. Use when asked to write or review them.
---

Group entries under Loud and Quiet. End with the release date.
```

**Only the description is in the prompt.** The body is read when the model
decides it needs it, by calling the `skill` tool:

```
› draft release notes for the bug fix

⏺ skill release-notes
  ⎿ 6 lines

## Loud
...
```

That is the whole point. Ten skills cost ten lines of context until one is
actually used, so a repository can carry a lot of them cheaply.

`AGENTS.md` is still the place for instructions that **always** apply. A skill
is for instructions that apply *sometimes*, and the description is what teaches
the model when.

Skills also load from `~/.cinch/skills/`, for ones that follow you rather than
the project. A project skill wins over a personal one with the same name.

A skill without a description is refused — the model would never learn it
exists — and descriptions are capped at 300 characters, since that text is sent
on every single turn. `cinch skills` and `cinch doctor` list what loaded and
name anything that did not:

```
warn  skills   1 usable, 1 ignored: .cinch/skills/wip/SKILL.md: no description — the model cannot know when to use it
```

A skill can only *tell the model to do* something. Anything it induces — an
edit, a shell command — still goes through approval and the sandbox, so a skill
grants no new power.

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

One limit worth knowing: **unless you run `confined`, `bash` breaks workspace
confinement.** `cd ..` works, and the path checks do not apply to a shell.
Approval is the only control on it, which is why saved approvals for `bash` are
command prefixes rather than a blanket allow, and why a prefix only ever covers
a single simple command.

Note what judging a command does **not** do. It decides whether to run, ask or
refuse; it is not a boundary. A command it cannot read is sent to a human rather
than blocked, and anything you approve runs with your full user permissions.

Run cinch in a git repository, so you can always see and undo what it changed.

## Confinement

`CINCH_SANDBOX=confined` adds the part judgement cannot give you: the kernel
refuses the syscall, so a command **you approved** still cannot leave the
workspace.

```bash
CINCH_SANDBOX=confined cinch
```

This uses [Landlock](https://landlock.io), so it needs **Linux 5.13 or newer**.
On any other platform cinch refuses to start rather than running unprotected:

```
cinch: sandbox: kernel confinement is not available on windows
```

`cinch doctor` reports what is actually enforcing, never what was asked for:

```
ok    sandbox   confined — commands judged, and landlock (kernel ABI v5) enforcing the workspace
fail  sandbox   confined was asked for but none (no kernel sandbox on windows)
```

What stays reachable is wider than you might expect, because a coding agent has
to be able to build code: the workspace and `/tmp` read-write, the toolchain
(`/usr`, `/bin`, `/lib`, `/etc`) read-only, and the build caches (`~/.cache`,
`~/go/pkg`, `~/.cargo`, `~/.npm`, `~/.m2`) read-write.

What it shuts out is the part that matters — **the rest of your home
directory**. `~/.ssh`, `~/.aws`, `~/.gnupg` and `~/.config` become unreadable to
cinch and to everything it starts, whatever you approve.

Two things it does not do. It does not restrict the network, and it applies to
the whole cinch process rather than only the shell — Landlock cannot be undone
once applied, so it happens once at startup.

## Development

```bash
make check    # everything CI runs: vet, test, gofmt, build
make test
make fmt
```

CI builds and tests on Linux and Windows. Both matter: the path handling code
behaves differently on each.

See [AGENTS.md](AGENTS.md) for the layout and conventions.
