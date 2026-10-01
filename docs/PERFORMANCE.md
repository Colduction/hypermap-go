# Performance design

`hypermap.Map` is optimized for ordered-map workloads, not for one isolated
hash lookup. Its index, ordered storage, deletion reuse, traversal, and garbage
collector cost are measured together.

## Data layout

The map uses two cooperating structures:

1. A Swiss-style, open-addressed hash index. Each 36-byte group stores an
   8-byte control word and seven `uint32` arena references. Control bytes 0–6
   are `0x00` for an empty slot or `0x80|tag` for a full slot, where the tag is
   seven hash bits; the remaining hash bits select a group, and groups are
   probed quadratically. Byte 7 is the group's overflow count: the number of
   keys whose probe sequence passed the group while it was full, saturating at 255. A lookup stops at the first group whose count is zero. Groups hold at
   most six keys on average (6/7 load), and a zeroed group is empty, so fresh
   and cleared storage needs no initialization pass.
2. An arena slice. Each entry stores one key, one value, and two `uint32` links.
   Index zero is a circular-list sentinel. The slice is append-dense before
   deletion; deleted slots then form a free list and are reused without
   allocation. Removing the last slot of a dense arena, such as a `PopBack` of
   the newest entry, shrinks it instead and keeps it dense.

Deletion clears the slot and decrements the overflow counts along the deleted
key's probe path, so a freed slot is immediately reusable. The index has no
tombstones: churn never triggers an O(n) purge and never grows the index, which
grows only when its live keys exceed the 6/7 load.

The index intentionally does not duplicate keys. A matching control tag leads
to an arena reference, and equality is checked against the key already stored
there. This is the same broad locality idea as Meta's F14Vector: a compact hash
index names entries in dense vector storage. The index backing array contains
no pointer fields for any key type; key and value pointers live only in the
arena.

Integer, float, and string keys, including interface keys holding them, are
hashed inline with the construction of the Go runtime's wyhash-derived
fallback hasher, combining process-wide secret words with a random per-map
seed. Floats fold `-0` into `+0` to match `==`. Every other key type uses
[`hash/maphash.Comparable`](https://go.dev/src/hash/maphash/maphash.go), which
supplies runtime-compatible hashing for arbitrary comparable keys. A key that
is not equal to itself, such as a NaN, can never be found again, so it is kept
out of the index entirely; popping it needs no index work.

Fresh insertion order is identical to arena order. Traversal walks the arena
by index until an effective move or deletion fragments it; fragmented maps
follow the links. `RangeFunc` has a single callback call site, so it and the
`Range` iterator inline into callers, and a range loop body compiles into a
direct loop.

## Why this design

- Go's current map implementation documents the Swiss-table control-byte
  scheme, eight-slot groups, quadratic probing, and 7/8 load policy in
  [the Go Swiss Tables article](https://go.dev/blog/swisstable) and
  [runtime source](https://go.dev/src/internal/runtime/maps/map.go).
- Abseil's original
  [Swiss Tables design notes](https://abseil.io/about/design/swisstables)
  explain the control-byte filtering and compact open-addressed layout that
  inspired Go's implementation. Both use tombstones, which accumulate under
  different-key churn until an O(n) in-place rebuild or a growth.
- Meta's
  [F14 design](https://engineering.fb.com/2019/04/25/developer-tools/f14/)
  replaces tombstones with per-chunk overflow counts, and uses a dense vector
  plus compact indices when iteration and memory density matter. Hypermap
  adopts both ideas. Against an eight-slot tombstone index with otherwise
  identical code, overflow counts cut different-key churn by 13–30% across 64
  to one million live entries and removed its periodic rebuilds, while lookups
  stayed within about 5%.
- The seven-dimensional hash-table study by Richter, Alvarez, and Dittrich
  shows why layout must be selected for the workload: cache locality, load,
  successful versus unsuccessful lookup, and entry representation all change
  the result. See the
  [VLDB paper](https://www.vldb.org/pvldb/vol9/p96-richter.pdf). A Robin Hood
  index with backward-shift deletion and eight-byte `distance|fingerprint`,
  reference buckets in the style of
  [ankerl::unordered_dense](https://github.com/martinus/unordered_dense) was
  15–34% faster while it fit in L2, but its larger buckets made lookups 43–91%
  slower from 65,536 to 1,048,576 entries, so it was rejected.
- `maphash.Comparable` reaches the runtime hasher through an indirect call that
  cannot be inlined; on the benchmark machine it cost about 5.6 ns per `int`
  key, against about 1.8 ns for the inline construction.
- Go's
  [GC optimization guide](https://go.dev/doc/gc-guide#Optimization_guide)
  recommends reducing pointer-rich representations only after measuring the
  complete application tradeoff. A local `map[K]*entry` prototype improved
  isolated lookup, but was rejected: at roughly one million live `int` entries,
  it added about 36 MiB of scannable heap, made a forced GC about 100 times
  slower, and made construction 18–23% slower in repeated runs.

The implementation remains pure Go and does not depend on `unsafe`,
`linkname`, or architecture-specific assembly. The inline hasher multiplies
64-bit words into 128-bit products, and `maphash.Comparable` calls the
underlying hasher twice on 32-bit targets, so the `amd64` lookup results below
do not transfer directly to 32-bit targets.

## Complexity and allocation behavior

Lookup, replacement, insertion, deletion, movement, neighbor access, and
front/back access and removal are O(1) expected, for any key. Deletion never
leaves work behind, so mixed insert and delete traffic at a stable size never
rebuilds or resizes the index. Growing the arena or the index is O(n) and
amortized; both double. `Clear` empties only the slots of live entries when
they are few relative to the index, and otherwise clears it in one pass.
Ordered traversal is O(n).

With the benchmarked scalar and string keys, populated-map reads, replacements,
moves, traversal, and delete/reinsert churn with the same or different keys
perform no heap allocation. A map initialized with a capacity hint never grows
or allocates while at most that many entries are live. Stable pointer keys
also avoid allocation, but an ephemeral stack-pointer key (or a comparable
value containing one) can escape through `maphash.Comparable`; that lookup may
allocate.

A correctly hinted initial fill allocates the arena and index backing arrays
once each. An unhinted fill allocates each time either array doubles.

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
`AllFromFront`, and Lorenzo `Range`. Each traversal runs in a helper function:
the compiler does not inline calls made directly in a `b.Loop` body, and the
helpers let callbacks compile as they do in ordinary code. The headline
traversal starts fresh and therefore exercises Hypermap's dense arena path.
Headline delete/set reinserts the same key; the different-key workload
separately exposes index maintenance under churn. Capacity hints are used
where available; Lorenzo does not expose a hinted constructor. The move
workload performs two effective relinks per iteration.

A callback that cannot be inlined, such as a method value stored in a
variable, pays a call per entry; that case is about 10% slower than the
previous two-loop traversal, which in turn could never inline its callback.

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
and workloads. The 4,096-entry headline wins are not universal. The built-in
map's small-map path, which skips hashing, wins integer hits at 8 entries.
Elliotchance's list chases pointers, while a fragmented Hypermap converts each
link index to an address, so that list wins fragmented traversal by 27–68% and
four-entry early stops by about 55%. Every map also receives a random hash
seed, so probe distributions can widen repeated-sample confidence intervals.
`Replace` is also sensitive to memory placement on the benchmark CPU: with the
seed and index fixed, relocating only the arena changed a 4,096-entry
sequential replace loop between about 3.7 and 7.3 ns per operation. Arena
bases share their low 12 address bits there, so this is not 4K aliasing; the
cause was not isolated without hardware performance counters. Report
replacement medians across several processes.
Changes are therefore also checked with model-based tests, the race detector,
`go vet`, and 32-bit builds.
