# Performance design

`hypermap.Map` is optimized for ordered-map workloads, not for one isolated
hash lookup. Its index, ordered storage, deletion reuse, traversal, and garbage
collector cost are measured together.

## Data layout

The map uses two cooperating structures:

1. A Swiss-style, open-addressed hash index. Each eight-slot group stores one
   64-bit control word and eight `uint32` arena references. The seven low hash
   bits form the control tag; the remaining bits select a group. Groups use a
   maximum average load of 7/8 and quadratic probing.
2. An arena slice. Each entry stores one key, one value, and two `uint32` links.
   Index zero is a circular-list sentinel. The slice is append-dense before
   deletion; deleted slots then form a free list and are reused without
   allocation.

The index intentionally does not duplicate keys. A matching control tag leads
to an arena reference, and equality is checked against the key already stored
there. This is the same broad locality idea as Meta's F14Vector: a compact hash
index names entries in dense vector storage. The index backing array contains
no pointer fields for any key type; key and value pointers live only in the
arena.

Fresh insertion order is identical to arena order. `RangeFunc` therefore walks
the arena linearly until an effective move or deletion fragments it; fragmented
maps fall back to the linked order. `Range` preserves the idiomatic `iter.Seq2`
API, while `RangeFunc` exposes the lower-overhead callback path.

## Why this design

- Go's current map implementation documents the Swiss-table control-byte
  scheme, eight-slot groups, quadratic probing, and 7/8 load policy in
  [the Go Swiss Tables article](https://go.dev/blog/swisstable) and
  [runtime source](https://go.dev/src/internal/runtime/maps/map.go).
- Abseil's original
  [Swiss Tables design notes](https://abseil.io/about/design/swisstables)
  explain the control-byte filtering and compact open-addressed layout that
  inspired Go's implementation.
- The seven-dimensional hash-table study by Richter, Alvarez, and Dittrich
  shows why layout must be selected for the workload: cache locality, load,
  successful versus unsuccessful lookup, and entry representation all change
  the result. See the
  [VLDB paper](https://www.vldb.org/pvldb/vol9/p96-richter.pdf).
- Meta's
  [F14 design](https://engineering.fb.com/2019/04/25/developer-tools/f14/)
  uses a dense vector plus compact indices when iteration and memory density
  matter.
- Go's
  [GC optimization guide](https://go.dev/doc/gc-guide#Optimization_guide)
  recommends reducing pointer-rich representations only after measuring the
  complete application tradeoff. A local `map[K]*entry` prototype improved
  isolated lookup, but was rejected: at roughly one million live `int` entries,
  it added about 36 MiB of scannable heap, made a forced GC about 100 times
  slower, and made construction 18–23% slower in repeated runs.

[`hash/maphash.Comparable`](https://go.dev/src/hash/maphash/maphash.go#L291)
supplies runtime-compatible hashing for arbitrary comparable Go keys, with an
independent random seed per map. The implementation remains pure Go and does
not depend on `unsafe`, `linkname`, or architecture-specific assembly. Current
32-bit Go builds invoke the underlying hasher twice to form a `uint64`, so the
`amd64` lookup results below do not transfer directly to 32-bit targets.

## Complexity and allocation behavior

Lookup, replacement, insertion, deletion, movement, and neighbor access are
O(1) average. Ordered traversal is O(n). Growing the arena or hash index is O(n),
so callers that know the maximum live size should pass it to `New` or `Init`.

With the benchmarked scalar and string keys, populated-map reads, replacements,
moves, traversal, and warmed delete/reinsert churn perform no steady-state heap
allocation. Stable pointer keys also avoid allocation, but an ephemeral
stack-pointer key (or a comparable value containing one) can escape through
`maphash.Comparable`; that lookup may allocate.

A correctly hinted initial fill allocates the arena and index backing arrays
once each. Tombstone compaction rebuilds the current index in place without
allocation. If different-key churn exhausts the fill budget while tombstones
are still sparse, the index grows once to avoid repeated O(n) compactions even
when `Len` is constant. That growth allocates. Consequently, a benchmark's
rounded `0 B/op` describes amortized steady state, not a promise that no
individual operation can ever grow storage.

## Benchmark protocol

The comparison suite uses pinned dependency versions. Adapter tests validate
normalized length, values, insertion order, and early-stop behavior. The
[headline benchmarks](../benchmarks/orderedmap_test.go) use 4,096 sequential
`int`/`int` entries. The [broader workloads](../benchmarks/workloads_test.go)
add sizes 8, 64, and 4,096; integer and string hits and misses; hinted and
unhinted construction; fragmented traversal (N/2 live entries after starting
with N); four-entry early termination; and different-key churn at 64, 4,096,
and 7,168 live entries.

Replacement uses Hypermap `Replace`, wk8/Elliott `Set`, and Lorenzo `Update`.
Traversal uses Hypermap `RangeFunc`, wk8 `Oldest`/`Next`, Elliott
`AllFromFront`, and Lorenzo `Range`. The headline traversal starts fresh and
therefore exercises Hypermap's dense arena path. Headline delete/set reinserts
the same key; the different-key workload separately exposes tombstone
maintenance. Capacity hints are used where available; Lorenzo does not expose
a hinted constructor. The move workload performs two effective relinks per
iteration.

On Windows, the maintained measurements pin the shell and inherited benchmark
process to one logical CPU before the timed run:

```powershell
cd benchmarks
$benchProcess = Get-Process -Id $PID
$benchProcess.ProcessorAffinity = [IntPtr]4
$benchProcess.PriorityClass = 'High'
$headline = '^(BenchmarkGet|BenchmarkSetReplace|BenchmarkDeleteSet|BenchmarkMoveToFrontBack|BenchmarkRange|BenchmarkFill)$'
go test -run '^$' -bench $headline -benchmem -benchtime=100ms -count=1 -cpu=1 | Out-Null
go test -run '^$' -bench $headline -benchmem -benchtime=1s -count=10 -cpu=1
```

`-cpu=1` sets `GOMAXPROCS`; processor affinity is a separate operating-system
control. Results are medians, not single samples. Run the broader matrix with
the same affinity settings:

```powershell
go test -run '^$' -bench '^BenchmarkWorkload' -benchmem -benchtime=1s -count=10 -cpu=1
```

Microbenchmarks are evidence for the stated machine, Go version, key shapes,
and workloads. The 4,096-entry headline wins are not universal: at 8 or 64
entries, fixed hashing and callback costs let competitors win several hit,
replace, traversal, and early-stop cases. Every map also receives a random hash
seed, so probe distributions can widen repeated-sample confidence intervals;
the replacement comparison is especially seed-sensitive. Changes are
therefore also checked with model-based tests, the race detector, `go vet`, and
32-bit builds.
