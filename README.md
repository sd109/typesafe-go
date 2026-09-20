## TypeSafe Go

A collection of [TypeSafe](https://typesafe.ai) API utilities written
in Go.

## CLI (qgrep)

A grep-like CLI for asking natural language questions.

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

See [`cmd/README.md`](cmd/README.md) for installation, syntax, and
output formats.

## Kubectl Plugin (qlog)

Ask TypeSafe questions about Kubernetes pod logs:

```sh
kubectl qlog -n payments deployment/api \
  --noul "Do these logs contain RBAC errors?" \
  --tail 500
```

See [`cmd/kubectl-qlog/README.md`](cmd/kubectl-qlog/README.md) for
motivation, use cases and detailed documentation.

## SDK

A standard-library-only Go client for the TypeSafe API.

```go
// Reads standard client env vars by default.
// Use sdk.With<option-name> to override.
client, err := sdk.NewClient()
if err != nil {
    log.Fatal(err)
}
defer client.Close()

result, err := client.SystemOne(context.Background(), sdk.SystemOneRequest{
    State: "I was charged twice. Please help.",
    Questions: map[string]sdk.Question{
        "billing": sdk.NoulQuestion{Instructions: "Is this about billing?"},
    },
})
if err != nil {
    log.Fatal(err)
}
fmt.Println(result.Answers["billing"])
```

The `TYPESAFE_API_KEY`, `TYPESAFE_BASE_URL`, and
`TYPESAFE_DEFAULT_MODEL` environment variables are used when
corresponding options are not supplied. `Client` is safe for
concurrent use. Use `WithHTTPClient` for custom transports and
`WithRequestRetryPolicy` for per-call retry behavior.
