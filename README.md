## TypeSafe Go

A collection of TypeSafe API utilities written in Go.

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

## qgrep CLI

A Cobra-based `qgrep` CLI for asking questions about stdin. See
[`cmd/README.md`](cmd/README.md) for installation, syntax, and output
formats.
