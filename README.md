# cinch

A coding agent that runs in your terminal. It reads and edits files in the
directory you start it from, and asks before it changes anything.

cinch is a single Go binary with almost no dependencies. It talks to the OpenAI
Responses API in stateless mode, which means OpenAI stores nothing: cinch keeps
the whole conversation itself.

> Early work in progress. The agent loop, the file tools and the approval system
> work. Sessions, streaming and a full terminal interface do not exist yet.

## Requirements

- Go 1.26 or newer
- An OpenAI API key
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

| Variable | Required | Default | Meaning |
|---|---|---|---|
| `OPENAI_API_KEY` | yes | — | Your API key |
| `OPENAI_MODEL` | no | `gpt-5.6` | Which model to use |

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
becomes the workspace, and cinch cannot read or write outside it.

```
$ cinch
cinch — ask about the files in this directory. ctrl-c to quit.

you: where is the step limit defined?
 -> grep "maxSteps"

cinch: internal/agent/agent.go:14 defines it as 25.

you: add a comment above it explaining why

allow edit file internal/agent/agent.go: replace "const maxSteps = 25" with "// ..."? [y/N/a] y

cinch: Added a comment at internal/agent/agent.go:14.
```

### Commands

| Command | Meaning |
|---|---|
| `cinch` | Start a chat session (same as `cinch chat`) |
| `cinch doctor` | Check that the local setup is complete |
| `cinch version` | Print version information |
| `cinch help` | Show all commands |

### Flags

Flags must come before the command name.

| Flag | Meaning |
|---|---|
| `-v`, `--version` | Print version and exit |
| `--debug` | Print extra information |
| `--cwd dir` | Run as if cinch started in `dir` |
| `-h`, `--help` | Show help |

## Tools

The model can use these, and nothing else:

| Tool | Needs approval | Purpose |
|---|---|---|
| `read_file` | no | Read a file, line numbered and size capped |
| `list_files` | no | List one directory |
| `grep` | no | Search file contents with a regular expression |
| `write_file` | yes | Create a file or replace it completely |
| `edit_file` | yes | Replace an exact string in a file |

## Safety

- **Workspace confinement.** Every path the model gives is checked before use.
  Absolute paths, `..` traversal and Windows drive letters are rejected on all
  platforms.
- **Secret files.** `.env`, `.env.local`, `.netrc` and `.npmrc` cannot be read.
  Otherwise your API key would be sent to OpenAI in the next request.
- **Approval.** Tools that change files ask first. The default answer is no.
  Answer `a` to allow that tool for the rest of the session.
- **Step limit.** One request runs at most 25 model turns, so a confused model
  cannot loop forever.

cinch does not sandbox anything yet. Run it in a git repository, so you can
always see and undo what it changed.

## Development

```bash
make check    # everything CI runs: vet, test, gofmt, build
make test
make fmt
```

CI builds and tests on Linux and Windows. Both matter: the path handling code
behaves differently on each.
