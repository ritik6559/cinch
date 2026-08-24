# AGENTS.md

Instructions for coding agents working in this repository.

## Commands

- **Check everything:** `make check` — vet, tests, gofmt and build in one step.
  Run this before saying work is finished.
- Build: `make build` (output in `bin/`)
- Format: `gofmt -w .` — required. CI fails on unformatted files, and editors on
  Windows save CRLF, which gofmt rewrites.

## Layout

- `internal/agent` — the loop. Knows nothing about HTTP or terminals.
- `internal/llm` — provider-neutral message and provider types.
- `internal/llm/openai` — the only package that knows OpenAI exists.
- `internal/provider` — chooses which provider a session uses.
- `internal/tools` — everything the model can do to the machine.
- `internal/compact` — shrinking a conversation that grew too large.
- `internal/session` — saving and resuming conversations.
- `internal/cli` — flags, commands, the REPL.

## Conventions

- Tool failures are returned as **text**, not Go errors. The model reads them
  and corrects itself; an aborted turn is wasted.
- Every tool call must be answered by a result carrying the same id. Never drop
  a `ToolResult` block — empty its content instead.
- Encrypted reasoning (`llm.Thinking.Opaque`) is replayed byte for byte. Never
  re-encode it.
- Paths from the model always go through `Workspace.resolve` before use.
- Errors are lowercase and do not end with punctuation.

## Testing

- Table tests where the cases are data, not copied blocks.
- Test names describe the behaviour, not the function: `TestDeniedToolStill
  ProducesAResult`, not `TestExecute`.
- A test that passes both with and without the fix proves nothing — check that
  a new regression test fails before you keep it.

## Do not

- Add a dependency without asking. The binary has one today (`godotenv`).
- Add comments that restate the code. Comment the reasoning, not the mechanics.
