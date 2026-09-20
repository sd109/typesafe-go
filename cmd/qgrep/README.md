# qgrep

`qgrep` reads a document from standard input and asks TypeSafe
natural-language questions about it.

## Install

```sh
go install github.com/sd109/typesafe-go/cmd/qgrep@latest
```

Set the API key before running it:

```sh
export TYPESAFE_API_KEY=...
```

The SDK also supports `TYPESAFE_BASE_URL`, `TYPESAFE_DEFAULT_MODEL`,
and `TYPESAFE_LOG_LEVEL`. Use `--model` to override the model for one
invocation.

## Usage

```sh
cat << EOF > file.txt
# Meeting 2000-01-01

Attendees: Alice, Bob

Discussing Bob's new house in Antarctica.
EOF

cat file.txt | qgrep \
  --noul "Does this file contain personal information?" \
  --choice "Is this file an {application log, meeting note, other}?" \
  --score "Is the information in this file older than [1 day, 1 week, 1 year]?"
```

Question flags may be repeated:

- `--noul QUESTION` asks a yes/no question.
- `--choice QUESTION` asks for one choice. Put the choices in one
  `{...}` group.
- `--score QUESTION` asks for an ordered score. Put the rubric in one
  `[...]` group.

Criteria are comma-separated, trimmed, non-empty, and must be unique.
Delimiter groups may not be nested or repeated. At least one question
is required.

The input document is sent as the request state. Positional arguments
are not accepted; use stdin for content. Diagnostics are written to
stderr, while successful results are written to stdout.

## Output

Human-readable output is rendered as a table with the columns `Type`,
`Value`, `Confidence`, and `Question`. The table defaults to a maximum
width of 300 characters; use `--table-width` to change it. Long
question cells wrap at word boundaries within the configured width.

```text
┌────────┬──────────────┬────────────┬──────────────────────────────┐
│  TYPE  │    VALUE     │ CONFIDENCE │           QUESTION           │
├────────┼──────────────┼────────────┼──────────────────────────────┤
│ noul   │ 0.8500       │            │ Does this file contain       │
│        │              │            │ personal information?        │
├────────┼──────────────┼────────────┼──────────────────────────────┤
│ choice │ meeting note │ 1.0000     │ Is this file an {application │
│        │              │            │ log, meeting note, other}?   │
├────────┼──────────────┼────────────┼──────────────────────────────┤
│ score  │ 1.9700       │ 0.9600     │ Is the information in this   │
│        │              │            │ file older than [1 day, 1    │
│        │              │            │ week, 1 year]?               │
└────────┴──────────────┴────────────┴──────────────────────────────┘
```

Use `--json` for machine-readable output. The table-width setting does
not affect JSON output. Results are an ordered array with `id`,
`question`, `type`, and the complete typed SDK answer:

```sh
cat file.txt | qgrep --noul "Does this contain personal information?" --json
```

Question IDs are deterministic (`noul-1`, `choice-1`, and `score-1`).
Output order is noul questions, then choice questions, then score
questions, preserving order within each type.

`qgrep` exits zero only after a successful API request and output.
Invalid CLI input, missing stdin, client construction failures, API
failures, malformed responses, and output failures produce a non-zero
exit status.

## Shell completion

Cobra supplies completion commands for supported shells:

```sh
qgrep completion bash
qgrep completion zsh
qgrep completion fish
qgrep completion powershell
```
