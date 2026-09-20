# kubectl-qlog

`kubectl-qlog` is a native Kubernetes CLI plugin for asking TypeSafe
natural-language questions about pod logs.

## Motivation

Not all applications use the same log format so a simple string search
for `error` in a log aggregation platform rarely shows you what you're
looking for immediately. Narrowing down the initial search space with
natural language questions can save a lot of time.

For example, you might query a misbehaving Kubernetes cluster's
`kube-system` logs to obtain general error classifications as an
initial triage step when trying to track down a source of
misbehaviour:

```sh
kubectl qlog -n kube-system --all-pods --tail 50 \
  --choice "Do these logs contain {networking, storage, rbac, none} errors?"
```

which produces:

```
┌────────────────────────────────────────────┬────────────┬────────────┐
│                  RESOURCE                  │   VALUE    │ CONFIDENCE │
├────────────────────────────────────────────┼────────────┼────────────┤
│ coredns-7d764666f9-j4xvw                   │ none       │ 0.9900     │
├────────────────────────────────────────────┼────────────┼────────────┤
│ coredns-7d764666f9-n8hfh                   │ none       │ 0.9900     │
├────────────────────────────────────────────┼────────────┼────────────┤
│ etcd-kind-control-plane                    │ none       │ 0.9900     │
├────────────────────────────────────────────┼────────────┼────────────┤
│ kindnet-6b6bz                              │ none       │ 0.9800     │
├────────────────────────────────────────────┼────────────┼────────────┤
│ kube-apiserver-kind-control-plane          │ none       │ 0.9700     │
├────────────────────────────────────────────┼────────────┼────────────┤
│ kube-controller-manager-kind-control-plane │ none       │ 0.9400     │
├────────────────────────────────────────────┼────────────┼────────────┤
│ kube-proxy-j2c9s                           │ networking │ 0.6600     │
├────────────────────────────────────────────┼────────────┼────────────┤
│ kube-scheduler-kind-control-plane          │ rbac       │ 0.9900     │
└────────────────────────────────────────────┴────────────┴────────────┘
```

More complex examples are provided below.

## Install

```sh
go install github.com/sd109/typesafe-go/cmd/kubectl-qlog@latest
```

Put the resulting `kubectl-qlog` binary on `PATH`. Kubernetes
discovers it as:

```sh
kubectl qlog --help
```

TypeSafe authentication and endpoint configuration use the SDK
environment variables:

```sh
export TYPESAFE_API_KEY=...
# Optional:
export TYPESAFE_BASE_URL=...
export TYPESAFE_DEFAULT_MODEL=jev-latest
```

The plugin uses the active kubeconfig context by default. Standard
kubeconfig, context, TLS, token, and exec-credential flags are
available, including `--kubeconfig`, `--context`, and `--namespace`.

## Selection

A command must select either explicit resources or all pods:

```sh
# A bare name means pod/<name>.
kubectl qlog -n payments api-0 pod/api-1 \
  --noul "Do these logs contain errors?"

# Resolve all pods owned by the Deployment.
kubectl qlog -n payments deployment/api \
  --score "What is the severity? [debug, info, warn, error]"

# Resolve all pods in one namespace.
kubectl qlog -n payments --all-pods \
  --choice "What subsystem is this? {storage, networking, compute, other}"

# Resolve multiple resources across any namespace.
kubectl qlog -A deployment/local-path-provisioner pod/etcd-kind-control-plane \
  --noul "Does this pod require attention?"

# Resolve all pods in all namespaces.
kubectl qlog -A --all-pods \
  --noul "Does this pod require attention?"
```

Standard resource aliases (e.g. `statefulset`, `statefulsets`, `sts`)
are supported.

## Questions

Question flags are repeatable:

- `--noul QUESTION` asks a yes/no question.
- `--choice QUESTION` asks for one value from a
  `{comma-separated, list}`.
- `--score QUESTION` asks for an ordered score from a
  `[low, medium, high]` rubric.

At least one question is required unless `--dry-run` is used.

## Log options

The plugin fetches finite log snapshots using these options:

```text
--since DURATION
--since-time RFC3339
--tail LINES
-c, --container NAME
--all-containers
```

`--since` and `--since-time` are mutually exclusive. `--tail` defaults
to `-1` (all available lines). `--container` and `--all-containers`
are mutually exclusive.

Without `--container`, qlog honors the
`kubectl.kubernetes.io/default-container` annotation when valid, then
selects the first regular container. `--all-containers` fetches init,
regular, and ephemeral containers in that order.

Each pod produces one TypeSafe request. The state is a JSON object
with one entry per selected container:

```json
{
  "namespace": "payments",
  "resource": "pod/api-0",
  "containers": [
    {
      "name": "api",
      "type": "regular",
      "image": "example/api:v1",
      "logs": "complete log content"
    }
  ]
}
```

Container logs are fetched sequentially within a pod. The
`--parallelism N` flag controls how many pod pipelines may run
concurrently; it defaults to `4`.

### Dry run

`--dry-run` resolves and validates the selected pods and containers
without fetching logs, constructing a TypeSafe client, or making
TypeSafe requests. Questions are optional:

```sh
kubectl qlog -n payments deployment/api --all-containers --dry-run -o json
```

Dry-run JSON contains `{namespace, resource, containers}` with each
container's `name`, `type`, and `image`; it never includes a `logs`
field. Dry-run table output contains `Namespace`, `Resource`, and
`Containers` columns, showing the pod name without `pod/` and
container names only.

## Errors and limits

Kubernetes resolution, log-stream, TypeSafe, cancellation,
malformed-response, and output errors produce a non-zero exit status.
TypeSafe requests are issued only after all target pods have been
resolved.

TypeSafe's current `jev-latest` model has a 32k-token context limit,
including questions and container metadata. qlog does not split or
otherwise manipulate logs. Use `--tail`, `--since`, or `--since-time`
to keep each pod's selected logs within the model limit. With
`--all-containers`, filters apply independently to each container
stream, so the combined request can still be large.
