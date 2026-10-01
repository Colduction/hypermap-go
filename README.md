<div align="center">

# hypermap-go

`hypermap-go` provides `hypermap.Map`: a compact, generic, insertion-ordered map for Go.

[![Go Reference](https://pkg.go.dev/badge/github.com/colduction/hypermap-go.svg)](https://pkg.go.dev/github.com/colduction/hypermap-go)
[![Go version](https://img.shields.io/github/go-mod/go-version/colduction/keycheck-go)](./go.mod)
[![Dependencies: none](https://img.shields.io/badge/dependencies-none-brightgreen)](./go.mod)
[![GitHub License](https://img.shields.io/github/license/Colduction/hypermap-go)](./LICENSE)

</div>

> [!TIP]
> Use `hypermap.New[K, V](capacity)` when the maximum live entry count is known: while at most that many entries are live, storage never grows, even under heavy insert/delete churn.

## Install

```sh
go get -u github.com/colduction/hypermap-go@latest
```

## Usage

```go
package main

import (
	"fmt"

	"github.com/colduction/hypermap-go"
)

func main() {
	m := hypermap.New[string, int](3)

	m.Set("b", 2)
	m.Set("a", 1)
	m.Set("c", 3)
	m.MoveToFront("c")

	for key, value := range m.Range() {
		fmt.Println(key, value)
	}
}
```

## URL query encoding

`hypermap.QueryMap` is a `Map[string, []string]` with an `Encode` method that
renders the entries as a URL query string in the current key order. It embeds
`Map`, so every map method (`Set`, `Get`, `MoveToFront`, `Range`, …) is
available on it as well.

```go
m := hypermap.NewQueryMap(3)

m.Set("name", []string{"ada lovelace"})
m.Set("tags", []string{"math", "code"})

fmt.Println(m.Encode()) // name=ada+lovelace&tags=math&tags=code
```

> [!TIP]
> `Encode` allocates once when it emits output. A nil/empty map, or one whose value slices are all empty, returns `""` without that output allocation.

## Features

| Capability          | Behavior                                                                  |
| ------------------- | ------------------------------------------------------------------------- |
| Zero value          | Ready to use without initialization.                                      |
| Ordering            | Preserves insertion order; replacing a value keeps the key in place.      |
| Lookup and mutation | O(1) average for lookup, insert, delete, movement, and front/back access. |
| Churn               | Deletes leave no tombstones, so churn never rebuilds or grows the index.  |
| Replacement         | `Replace` updates only an existing key; `Set` inserts or replaces.        |
| Iteration           | `Range` (`iter.Seq2`) and `RangeFunc` inline into the caller's loop.      |
| Storage reuse       | `Clear` keeps allocated storage, while `Reset` releases it.               |
| Query encoding      | `QueryMap.Encode` renders `string`/`[]string` entries as a query string.  |

> [!IMPORTANT]
> Do not copy a `Map` after initialization or mutation. It does not synchronize access; share one across goroutines only with external synchronization, or shard independent maps by key or worker for write-heavy services.

## Benchmarks

Median time with 4,096 `int`/`int` entries; lower is better. These are the
maintained headline workloads, and the fastest result in each row is bold.

| Operation       |     hypermap |      wk8 | elliotchance | lorenzosaino | vs best rival |
| --------------- | -----------: | -------: | -----------: | -----------: | ------------: |
| Get             | **4.143 ns** | 5.135 ns |     5.219 ns |     5.335 ns |  19.3% faster |
| Replace         | **4.063 ns** | 9.320 ns |     11.88 ns |     7.458 ns |  45.5% faster |
| Delete + set    | **13.85 ns** | 81.99 ns |     57.34 ns |     73.20 ns |  75.8% faster |
| Move front/back | **11.10 ns** | 23.45 ns |            — |     33.67 ns |  52.7% faster |
| Range all       | **1.604 µs** | 9.749 µs |     3.544 µs |     4.866 µs |  54.7% faster |
| Fill new map    | **32.35 µs** | 133.1 µs |     96.74 µs |     254.2 µs |  66.6% faster |

Every operation on an already-populated Hypermap in the table is **0 B/op and
0 allocs/op**.
Filling a capacity-sized map uses 147,456 B and 2 allocations, versus
278,896–492,776 B and 4,114–8,213 allocations for the alternatives. The broader
suite also measures misses, strings, tiny maps, fragmented traversal, early
stop, unhinted construction, and different-key churn.

<details>
<summary>Methodology and reproduction</summary>

Results are medians from 10 one-second samples using Go 1.27.1 on Windows 11
`amd64` and an AMD Ryzen 9 7950X, pinned to logical CPU 2 with `GOMAXPROCS=1`.
Capacity hints are used where supported. `Range all` and `Fill new map` process
all 4,096 entries. Replacement and traversal use each package's fastest
non-allocating API: Hypermap uses `Replace` and `RangeFunc`. Each traversal
runs in a helper function so callbacks compile as in ordinary code; the
compiler does not inline calls made directly in a `b.Loop` body. Timings can
vary with map seed, memory placement, and system clock state; the table reports
ten-sample medians, not universal dominance. Hypermap's `Replace` time depends
on where its arena lands in memory on this CPU: individual maps measured 4.0 to
8.9 ns, so the `Replace` row for Hypermap and Lorenzo reports medians of 50
samples from five processes.

Compared versions:

- [`wk8/go-ordered-map/v2` v2.1.8](https://github.com/wk8/go-ordered-map/tree/v2.1.8)
- [`elliotchance/orderedmap/v3` v3.1.1](https://github.com/elliotchance/orderedmap/tree/v3.1.1)
- [`lorenzosaino/go-orderedmap` cf642d9](https://github.com/lorenzosaino/go-orderedmap/commit/cf642d91fab6)

The source, validation checks, and pinned dependency versions are in
[`benchmarks`](benchmarks). The maintained Windows headline command is:

```powershell
cd benchmarks
$benchProcess = Get-Process -Id $PID
$benchProcess.ProcessorAffinity = [IntPtr]4
$benchProcess.PriorityClass = 'High'
$headline = '^(BenchmarkGet|BenchmarkSetReplace|BenchmarkDeleteSet|BenchmarkMoveToFrontBack|BenchmarkRange|BenchmarkFill)$'
go test -run '^$' -bench $headline -benchmem -benchtime=100ms -count=1 -cpu=1 | Out-Null
go test -run '^$' -bench $headline -benchmem -benchtime=1s -count=10 -cpu=1
```

Run the broader workload matrix with `-bench '^BenchmarkWorkload'`. Hypermap
was fastest in 22 of its 30 cases. A built-in map fast path wins integer hits
at 8 entries, string hits at 64 entries were within sample noise, and
elliotchance's pointer-linked list wins traversal of fragmented maps and
four-entry early stops. The table is not a claim of universal superiority
across all key types, sizes, or machines.
See [the performance design and tradeoffs](docs/PERFORMANCE.md) for the data
layout, research basis, allocation caveats, and complete protocol.

</details>

## License

This project is released under the MIT License. See [LICENSE](LICENSE).
