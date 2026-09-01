# Repository change requirements

These requirements apply to every change in this repository. Do not declare a
change complete without reporting the commands run and their results.

## Preserve compatibility and ownership semantics

- Preserve the exported API and existing default behavior unless the user
  explicitly authorizes a breaking change.
- Treat files, buffers, channels, goroutines, timers, and temporary artifacts
  as owned resources. Define who creates, closes, flushes, and releases each
  resource. Do not transfer ownership implicitly.
- A caller-owned `io.Writer`, buffer, or file must not be closed by lumberjack.
  Resources created internally must be closed on success and every error path.
- Do not introduce an unbounded goroutine, channel, queue, allocation, retry,
  or background worker. Workers must have a justified lifetime and termination
  rule; disabled features must not allocate or start workers.
- Keep `Logger` operations safe under their documented concurrent use. Avoid
  copying live values containing mutexes, and do not access mutable state
  outside the appropriate lock.
- Preserve partial-write accounting, maximum-size overflow protection,
  rotation ordering, file permissions/ownership, and cleanup behavior.

## Required tests for every change

Run all of the following from the repository root. Use a writable `GOCACHE`
when the environment's default cache is unavailable.

```sh
go test ./...
go test -race ./...
go test -gcflags=all=-d=checkptr=2 ./...
go vet ./...
go test -run '^$' -bench . -benchmem ./...
```

Also run `git diff --check`.

Add focused regression tests for changed behavior. Tests involving resource
ownership must cover the success path and relevant failures, including close,
flush, rotation, removal, and repeated calls where applicable. For goroutine or
background-work changes, verify that disabled/no-op paths start no worker and
that enabled paths cannot grow workers without bound.

If the package contains fuzz targets relevant to the changed parser, filename,
size, compression, or rotation logic, run each relevant target for a bounded
duration. Add a fuzz target when untrusted or highly variable input introduces
a meaningful state space that table tests cannot cover well.

## Benchmark gate

Every change must run the repository benchmark suite, including documentation
and test-only changes, so benchmark regressions are visible.

For any change to executable code, capture an A/B benchmark against the exact
pre-change revision using the same machine, Go version, benchmark command,
`-benchtime`, and `-count`. Use at least five samples per side. Compare the
relevant production workload, not only a synthetic helper. Include filesystem
writes and `Flush`/rotation costs when those occur in production.

Prefer `benchstat` for comparison when available. Otherwise report the raw
sample range and median. Do not claim an improvement from a single run.

Accept an executable-code change only when one of these is true:

- it measurably improves the intended production workload without a meaningful
  regression elsewhere; or
- it is required for correctness or safety and its measured performance cost
  is explicitly reported and justified.

Do not optimize by silently weakening durability, file-removal detection,
locking, cleanup, or error semantics. Such tradeoffs must be explicit opt-in
behavior and need separate benchmarks for both the default and opt-in paths.

## Completion report

Report:

- compatibility and ownership impact;
- tests, race/checkptr/vet results;
- benchmark environment and exact A/B command;
- allocation, latency, and throughput results relevant to production; and
- any remaining risk or benchmark limitation.

If a required command cannot run, state the blocker. Do not represent the
change as fully verified.
