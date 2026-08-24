# cinch

A coding agent that runs in your terminal. It reads, searches and edits files in
the directory you start it from, runs commands to check its own work, and asks
before it changes anything.

cinch is a single Go binary with one dependency. It talks to the OpenAI
Responses API in stateless mode, which means OpenAI stores nothing: cinch keeps
the whole conversation itself, and can save and resume it.

> Early work in progress. The agent loop, the tools, approval, streaming,
> sessions and compaction all work. There is no full terminal interface yet, no
> sandbox, and no subagents.

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
$ cinch
cinch — ask about the files in this directory. ctrl-c to quit.

you: where is the step limit defined?
 -> grep "maxSteps"

cinch: internal/agent/agent.go:14 defines it as 25.

you: raise it to 40 and run the tests

allow edit file internal/agent/agent.go: replace "maxSteps = 25" with "maxSteps = 40"? [y/N/a] y
allow run: go test ./internal/agent/? [y/N/a] y

cinch: Changed it at internal/agent/agent.go:14. Tests pass.
```

### Commands

| Command | Meaning |
|---|---|
| `cinch` | Start a chat session (same as `cinch chat`) |
| `cinch sessions` | List saved sessions |
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
| `-h`, `--help` | Show help |

### While chatting

| | |
|---|---|
| `/compact` | Shrink the conversation now |
| Ctrl-C during a turn | Cancel that turn, keep the session |
| Ctrl-C at the prompt | Quit |
| Ctrl-D | Quit |

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
  default answer is no. Answer `a` to allow that tool for the rest of the
  session.
- **Step limit.** One request runs at most 25 model turns, so a confused model
  cannot loop forever.

Two limits worth knowing. **`bash` breaks workspace confinement** — `cd ..`
works, and the path checks do not apply to a shell. And answering `a` to a bash
prompt grants shell access for the rest of the session, which is a much larger
grant than `a` on `edit_file`.

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
