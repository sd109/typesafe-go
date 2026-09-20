# kubectl-qlog

A `kubectl` plugin for asking TypeSafe natural language questions
about a set of pod logs.

## Usage

```sh
kubectl qlog -n kube-system --all-pods \
  --noul "Do these logs contain any errors?" \
  --score "Is this pod logging at [debug, info, warn, error] level?" \
  --choice "Is this pod concerned with {storage, networking, compute, other}?"
```

## Limitations

### Context Length

TypeSafe's current `jev-latest` model has a context length limit of
32k tokens, including input content and questions. The TypeSafe API
return a `max_tokens_exceeded` HTTP 400 error for requests exceeding
this limit.

The `qlog` plugin sends each pod's logs as a separate request but
explicitly avoids context manipulation (e.g. splitting a pod's logs
into smaller batches) since the original question may not make sense
when applied to batched subsets of a logs.

Instead, `qlog` supports existing `kubectl` log filtering flags
including `--tail`, `--since`, `--since-time` and `--limit-bytes`. The
caller should use these flags to ensure the volume of logs sent to the
TypeSafe API does not exceed the target model's context length limit.
