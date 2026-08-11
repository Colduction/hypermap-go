package orderedmapbench

import (
	"strconv"
	"testing"

	hypermap "github.com/colduction/hypermap-go"
	elliott "github.com/elliotchance/orderedmap/v3"
	lorenzo "github.com/lorenzosaino/go-orderedmap"
	wk8 "github.com/wk8/go-ordered-map/v2"
)

var (
	workloadSizes      = [...]int{8, 64, 4096}
	churnWorkloadSizes = [...]int{64, 4096, 7 * 1024}
)

var (
	workloadValue int
	workloadOK    bool
	workloadErr   error
	workloadLen   int
)

type workloadState[K comparable] struct {
	name     string
	length   func() int
	get      func(K) (int, bool)
	rangeAll func(func(K, int) bool)
}

// BenchmarkWorkloadGetInt measures integer-key lookup hits and misses.
func BenchmarkWorkloadGetInt(b *testing.B) {
	for _, hit := range []bool{true, false} {
		name := "miss"
		if hit {
			name = "hit"
		}
		b.Run(name, func(b *testing.B) {
			for _, size := range workloadSizes {
				b.Run("size="+strconv.Itoa(size), func(b *testing.B) {
					keys, values := intWorkloadData(size)
					lookups := keys
					if !hit {
						lookups = missingIntWorkloadKeys(size)
					}
					b.Run("hypermap", func(b *testing.B) {
						benchmarkHypermapGetWorkload(b, keys, values, lookups, hit)
					})
					b.Run("wk8", func(b *testing.B) {
						benchmarkWK8GetWorkload(b, keys, values, lookups, hit)
					})
					b.Run("elliotchance", func(b *testing.B) {
						benchmarkElliottGetWorkload(b, keys, values, lookups, hit)
					})
					b.Run("lorenzosaino", func(b *testing.B) {
						benchmarkLorenzoGetWorkload(b, keys, values, lookups, hit)
					})
				})
			}
		})
	}
}

// BenchmarkWorkloadGetString measures string-key lookup hits and misses.
func BenchmarkWorkloadGetString(b *testing.B) {
	for _, hit := range []bool{true, false} {
		name := "miss"
		if hit {
			name = "hit"
		}
		b.Run(name, func(b *testing.B) {
			for _, size := range workloadSizes {
				b.Run("size="+strconv.Itoa(size), func(b *testing.B) {
					keys, values := stringWorkloadData(size, "key/")
					lookups := keys
					if !hit {
						lookups, _ = stringWorkloadData(size, "missing/")
					}
					b.Run("hypermap", func(b *testing.B) {
						benchmarkHypermapGetWorkload(b, keys, values, lookups, hit)
					})
					b.Run("wk8", func(b *testing.B) {
						benchmarkWK8GetWorkload(b, keys, values, lookups, hit)
					})
					b.Run("elliotchance", func(b *testing.B) {
						benchmarkElliottGetWorkload(b, keys, values, lookups, hit)
					})
					b.Run("lorenzosaino", func(b *testing.B) {
						benchmarkLorenzoGetWorkload(b, keys, values, lookups, hit)
					})
				})
			}
		})
	}
}

// BenchmarkWorkloadReplaceString measures value replacement for string keys.
func BenchmarkWorkloadReplaceString(b *testing.B) {
	for _, size := range workloadSizes {
		b.Run("size="+strconv.Itoa(size), func(b *testing.B) {
			keys, values := stringWorkloadData(size, "key/")
			b.Run("hypermap", func(b *testing.B) {
				benchmarkHypermapReplaceStringWorkload(b, keys, values)
			})
			b.Run("wk8", func(b *testing.B) {
				benchmarkWK8ReplaceStringWorkload(b, keys, values)
			})
			b.Run("elliotchance", func(b *testing.B) {
				benchmarkElliottReplaceStringWorkload(b, keys, values)
			})
			b.Run("lorenzosaino", func(b *testing.B) {
				benchmarkLorenzoReplaceStringWorkload(b, keys, values)
			})
		})
	}
}

// BenchmarkWorkloadFill measures filling new maps with and without capacity hints.
func BenchmarkWorkloadFill(b *testing.B) {
	for _, hinted := range []bool{true, false} {
		name := "unhinted"
		if hinted {
			name = "hinted"
		}
		b.Run(name, func(b *testing.B) {
			for _, size := range workloadSizes {
				b.Run("size="+strconv.Itoa(size), func(b *testing.B) {
					b.Run("hypermap", func(b *testing.B) {
						benchmarkHypermapFillWorkload(b, size, hinted)
					})
					b.Run("wk8", func(b *testing.B) {
						benchmarkWK8FillWorkload(b, size, hinted)
					})
					b.Run("elliotchance", func(b *testing.B) {
						benchmarkElliottFillWorkload(b, size, hinted)
					})
					if !hinted {
						b.Run("lorenzosaino", func(b *testing.B) {
							benchmarkLorenzoFillWorkload(b, size)
						})
					}
				})
			}
		})
	}
}

// BenchmarkWorkloadChurnDifferentKeys measures constant-size churn where a
// removed key is replaced by a different hash. This exposes tombstone
// maintenance that same-key delete/reinsert workloads do not exercise.
func BenchmarkWorkloadChurnDifferentKeys(b *testing.B) {
	for _, size := range churnWorkloadSizes {
		b.Run("size="+strconv.Itoa(size), func(b *testing.B) {
			keys, values := intWorkloadData(size)
			b.Run("hypermap", func(b *testing.B) {
				m := newHypermapWorkloadMap(keys, values)
				live := append([]int(nil), keys...)
				next, position := size, 0
				var deleted, replaced bool
				b.ReportAllocs()
				for b.Loop() {
					_, deleted = m.Delete(live[position])
					_, replaced = m.Set(next, next)
					live[position] = next
					next++
					position++
					if position == size {
						position = 0
					}
				}
				workloadOK = deleted && !replaced
				validateChurnWorkloadResult(b, hypermapWorkloadState(m), live, next, deleted, !replaced)
			})
			b.Run("wk8", func(b *testing.B) {
				m := newWK8WorkloadMap(keys, values)
				live := append([]int(nil), keys...)
				next, position := size, 0
				var deleted, replaced bool
				b.ReportAllocs()
				for b.Loop() {
					_, deleted = m.Delete(live[position])
					_, replaced = m.Set(next, next)
					live[position] = next
					next++
					position++
					if position == size {
						position = 0
					}
				}
				workloadOK = deleted && !replaced
				validateChurnWorkloadResult(b, wk8WorkloadState(m), live, next, deleted, !replaced)
			})
			b.Run("elliotchance", func(b *testing.B) {
				m := newElliottWorkloadMap(keys, values)
				live := append([]int(nil), keys...)
				next, position := size, 0
				var deleted, inserted bool
				b.ReportAllocs()
				for b.Loop() {
					deleted = m.Delete(live[position])
					inserted = m.Set(next, next)
					live[position] = next
					next++
					position++
					if position == size {
						position = 0
					}
				}
				workloadOK = deleted && inserted
				validateChurnWorkloadResult(b, elliottWorkloadState(m), live, next, deleted, inserted)
			})
			b.Run("lorenzosaino", func(b *testing.B) {
				m := newLorenzoWorkloadMap(keys, values)
				live := append([]int(nil), keys...)
				next, position := size, 0
				var deleted bool
				var err error
				b.ReportAllocs()
				for b.Loop() {
					_, deleted = m.Delete(live[position])
					err = m.PushBack(next, next)
					live[position] = next
					next++
					position++
					if position == size {
						position = 0
					}
				}
				workloadOK, workloadErr = deleted, err
				validateChurnWorkloadResult(b, lorenzoWorkloadState(m), live, next, deleted, err == nil)
			})
		})
	}
}

// BenchmarkWorkloadRangeFragmented measures full traversal after interleaved deletions.
func BenchmarkWorkloadRangeFragmented(b *testing.B) {
	for _, size := range workloadSizes {
		b.Run("size="+strconv.Itoa(size), func(b *testing.B) {
			b.Run("hypermap", func(b *testing.B) {
				benchmarkHypermapRangeFragmentedWorkload(b, size)
			})
			b.Run("wk8", func(b *testing.B) {
				benchmarkWK8RangeFragmentedWorkload(b, size)
			})
			b.Run("elliotchance", func(b *testing.B) {
				benchmarkElliottRangeFragmentedWorkload(b, size)
			})
			b.Run("lorenzosaino", func(b *testing.B) {
				benchmarkLorenzoRangeFragmentedWorkload(b, size)
			})
		})
	}
}

// BenchmarkWorkloadRangeEarlyStop measures traversal stopped after four entries.
func BenchmarkWorkloadRangeEarlyStop(b *testing.B) {
	for _, size := range workloadSizes {
		b.Run("size="+strconv.Itoa(size), func(b *testing.B) {
			b.Run("hypermap", func(b *testing.B) {
				benchmarkHypermapRangeEarlyStopWorkload(b, size)
			})
			b.Run("wk8", func(b *testing.B) {
				benchmarkWK8RangeEarlyStopWorkload(b, size)
			})
			b.Run("elliotchance", func(b *testing.B) {
				benchmarkElliottRangeEarlyStopWorkload(b, size)
			})
			b.Run("lorenzosaino", func(b *testing.B) {
				benchmarkLorenzoRangeEarlyStopWorkload(b, size)
			})
		})
	}
}

// TestWorkloadAdapters verifies normalized lookup, order, and early-stop behavior.
func TestWorkloadAdapters(t *testing.T) {
	keys, values := intWorkloadData(8)
	hypermapMap := newHypermapWorkloadMap(keys, values)
	wk8Map := newWK8WorkloadMap(keys, values)
	elliottMap := newElliottWorkloadMap(keys, values)
	lorenzoMap := newLorenzoWorkloadMap(keys, values)
	for key := 1; key < len(keys); key += 2 {
		hypermapMap.Delete(key)
		wk8Map.Delete(key)
		elliottMap.Delete(key)
		lorenzoMap.Delete(key)
	}
	keys, values = fragmentedIntWorkloadData(8)
	states := []workloadState[int]{
		hypermapWorkloadState(hypermapMap),
		wk8WorkloadState(wk8Map),
		elliottWorkloadState(elliottMap),
		lorenzoWorkloadState(lorenzoMap),
	}
	for _, state := range states {
		t.Run(state.name, func(t *testing.T) {
			validateWorkloadState(t, state, keys, values)
			var visits int
			state.rangeAll(func(_, _ int) bool {
				visits++
				return visits < 3
			})
			if visits != 3 {
				t.Fatalf("early-stop visits = %d, want 3", visits)
			}
		})
	}
}

func benchmarkHypermapGetWorkload[K comparable](b *testing.B, keys []K, values []int, lookups []K, hit bool) {
	m := newHypermapWorkloadMap(keys, values)
	mask := len(lookups) - 1
	var (
		value int
		ok    bool
		i     int
	)
	b.ReportAllocs()
	for b.Loop() {
		value, ok = m.Get(lookups[i&mask])
		i++
	}
	workloadValue, workloadOK = value, ok
	validateGetWorkloadResult(b, value, ok, i, values, hit)
	validateWorkloadState(b, hypermapWorkloadState(m), keys, values)
}

func benchmarkWK8GetWorkload[K comparable](b *testing.B, keys []K, values []int, lookups []K, hit bool) {
	m := newWK8WorkloadMap(keys, values)
	mask := len(lookups) - 1
	var (
		value int
		ok    bool
		i     int
	)
	b.ReportAllocs()
	for b.Loop() {
		value, ok = m.Get(lookups[i&mask])
		i++
	}
	workloadValue, workloadOK = value, ok
	validateGetWorkloadResult(b, value, ok, i, values, hit)
	validateWorkloadState(b, wk8WorkloadState(m), keys, values)
}

func benchmarkElliottGetWorkload[K comparable](b *testing.B, keys []K, values []int, lookups []K, hit bool) {
	m := newElliottWorkloadMap(keys, values)
	mask := len(lookups) - 1
	var (
		value int
		ok    bool
		i     int
	)
	b.ReportAllocs()
	for b.Loop() {
		value, ok = m.Get(lookups[i&mask])
		i++
	}
	workloadValue, workloadOK = value, ok
	validateGetWorkloadResult(b, value, ok, i, values, hit)
	validateWorkloadState(b, elliottWorkloadState(m), keys, values)
}

func benchmarkLorenzoGetWorkload[K comparable](b *testing.B, keys []K, values []int, lookups []K, hit bool) {
	m := newLorenzoWorkloadMap(keys, values)
	mask := len(lookups) - 1
	var (
		value int
		ok    bool
		i     int
	)
	b.ReportAllocs()
	for b.Loop() {
		value, ok = m.Get(lookups[i&mask])
		i++
	}
	workloadValue, workloadOK = value, ok
	validateGetWorkloadResult(b, value, ok, i, values, hit)
	validateWorkloadState(b, lorenzoWorkloadState(m), keys, values)
}

func benchmarkHypermapReplaceStringWorkload(b *testing.B, keys []string, values []int) {
	m := newHypermapWorkloadMap(keys, values)
	mask := len(keys) - 1
	var (
		old      int
		replaced bool
		i        int
	)
	b.ReportAllocs()
	for b.Loop() {
		old, replaced = m.Replace(keys[i&mask], i+len(keys))
		i++
	}
	workloadValue, workloadOK = old, replaced
	if !replaced {
		b.Fatal("last replacement missed an existing key")
	}
	validateWorkloadState(b, hypermapWorkloadState(m), keys, replacementWorkloadValues(len(keys), i))
}

func benchmarkWK8ReplaceStringWorkload(b *testing.B, keys []string, values []int) {
	m := newWK8WorkloadMap(keys, values)
	mask := len(keys) - 1
	var (
		old      int
		replaced bool
		i        int
	)
	b.ReportAllocs()
	for b.Loop() {
		old, replaced = m.Set(keys[i&mask], i+len(keys))
		i++
	}
	workloadValue, workloadOK = old, replaced
	if !replaced {
		b.Fatal("last replacement inserted a new key")
	}
	validateWorkloadState(b, wk8WorkloadState(m), keys, replacementWorkloadValues(len(keys), i))
}

func benchmarkElliottReplaceStringWorkload(b *testing.B, keys []string, values []int) {
	m := newElliottWorkloadMap(keys, values)
	mask := len(keys) - 1
	var (
		inserted bool
		i        int
	)
	b.ReportAllocs()
	for b.Loop() {
		inserted = m.Set(keys[i&mask], i+len(keys))
		i++
	}
	workloadOK = inserted
	if inserted {
		b.Fatal("last replacement inserted a new key")
	}
	validateWorkloadState(b, elliottWorkloadState(m), keys, replacementWorkloadValues(len(keys), i))
}

func benchmarkLorenzoReplaceStringWorkload(b *testing.B, keys []string, values []int) {
	m := newLorenzoWorkloadMap(keys, values)
	mask := len(keys) - 1
	var (
		old int
		err error
		i   int
	)
	b.ReportAllocs()
	for b.Loop() {
		old, err = m.Update(keys[i&mask], i+len(keys))
		i++
	}
	workloadValue, workloadErr = old, err
	if err != nil {
		b.Fatalf("last replacement: %v", err)
	}
	validateWorkloadState(b, lorenzoWorkloadState(m), keys, replacementWorkloadValues(len(keys), i))
}

func benchmarkHypermapFillWorkload(b *testing.B, size int, hinted bool) {
	keys, values := intWorkloadData(size)
	var m hypermap.Map[int, int]
	b.ReportAllocs()
	if hinted {
		for b.Loop() {
			m.Init(size)
			for i := range size {
				m.Set(i, i)
			}
		}
	} else {
		for b.Loop() {
			m.Init(0)
			for i := range size {
				m.Set(i, i)
			}
		}
	}
	workloadLen = m.Len()
	validateWorkloadState(b, hypermapWorkloadState(&m), keys, values)
}

func benchmarkWK8FillWorkload(b *testing.B, size int, hinted bool) {
	keys, values := intWorkloadData(size)
	var m *wk8.OrderedMap[int, int]
	b.ReportAllocs()
	if hinted {
		for b.Loop() {
			m = wk8.New[int, int](size)
			for i := range size {
				m.Set(i, i)
			}
		}
	} else {
		for b.Loop() {
			m = wk8.New[int, int]()
			for i := range size {
				m.Set(i, i)
			}
		}
	}
	workloadLen = m.Len()
	validateWorkloadState(b, wk8WorkloadState(m), keys, values)
}

func benchmarkElliottFillWorkload(b *testing.B, size int, hinted bool) {
	keys, values := intWorkloadData(size)
	var m *elliott.OrderedMap[int, int]
	b.ReportAllocs()
	if hinted {
		for b.Loop() {
			m = elliott.NewOrderedMapWithCapacity[int, int](size)
			for i := range size {
				m.Set(i, i)
			}
		}
	} else {
		for b.Loop() {
			m = elliott.NewOrderedMap[int, int]()
			for i := range size {
				m.Set(i, i)
			}
		}
	}
	workloadLen = m.Len()
	validateWorkloadState(b, elliottWorkloadState(m), keys, values)
}

func benchmarkLorenzoFillWorkload(b *testing.B, size int) {
	keys, values := intWorkloadData(size)
	var (
		m   *lorenzo.OrderedMap[int, int]
		err error
	)
	b.ReportAllocs()
	for b.Loop() {
		m = lorenzo.New[int, int]()
		for i := range size {
			err = m.PushBack(i, i)
		}
	}
	workloadLen, workloadErr = m.Len(), err
	if err != nil {
		b.Fatalf("last insertion: %v", err)
	}
	validateWorkloadState(b, lorenzoWorkloadState(m), keys, values)
}

func benchmarkHypermapRangeFragmentedWorkload(b *testing.B, size int) {
	keys, values := intWorkloadData(size)
	m := newHypermapWorkloadMap(keys, values)
	deleteOddHypermapWorkloadKeys(m, size)
	keys, values = fragmentedIntWorkloadData(size)
	var sum int
	yield := func(key, value int) bool {
		sum += key + value
		return true
	}
	b.ReportAllocs()
	for b.Loop() {
		m.RangeFunc(yield)
	}
	workloadValue = sum
	validateRangeWorkloadResult(b, sum, keys, values)
	validateWorkloadState(b, hypermapWorkloadState(m), keys, values)
}

func benchmarkWK8RangeFragmentedWorkload(b *testing.B, size int) {
	keys, values := intWorkloadData(size)
	m := newWK8WorkloadMap(keys, values)
	deleteOddWK8WorkloadKeys(m, size)
	keys, values = fragmentedIntWorkloadData(size)
	var sum int
	b.ReportAllocs()
	for b.Loop() {
		for pair := m.Oldest(); pair != nil; pair = pair.Next() {
			sum += pair.Key + pair.Value
		}
	}
	workloadValue = sum
	validateRangeWorkloadResult(b, sum, keys, values)
	validateWorkloadState(b, wk8WorkloadState(m), keys, values)
}

func benchmarkElliottRangeFragmentedWorkload(b *testing.B, size int) {
	keys, values := intWorkloadData(size)
	m := newElliottWorkloadMap(keys, values)
	deleteOddElliottWorkloadKeys(m, size)
	keys, values = fragmentedIntWorkloadData(size)
	var sum int
	yield := func(key, value int) bool {
		sum += key + value
		return true
	}
	b.ReportAllocs()
	for b.Loop() {
		m.AllFromFront()(yield)
	}
	workloadValue = sum
	validateRangeWorkloadResult(b, sum, keys, values)
	validateWorkloadState(b, elliottWorkloadState(m), keys, values)
}

func benchmarkLorenzoRangeFragmentedWorkload(b *testing.B, size int) {
	keys, values := intWorkloadData(size)
	m := newLorenzoWorkloadMap(keys, values)
	deleteOddLorenzoWorkloadKeys(m, size)
	keys, values = fragmentedIntWorkloadData(size)
	var sum int
	yield := func(key, value int) bool {
		sum += key + value
		return true
	}
	b.ReportAllocs()
	for b.Loop() {
		m.Range(yield)
	}
	workloadValue = sum
	validateRangeWorkloadResult(b, sum, keys, values)
	validateWorkloadState(b, lorenzoWorkloadState(m), keys, values)
}

func benchmarkHypermapRangeEarlyStopWorkload(b *testing.B, size int) {
	const limit = 4
	keys, values := intWorkloadData(size)
	m := newHypermapWorkloadMap(keys, values)
	var visits, sum int
	yield := func(key, value int) bool {
		visits++
		sum += key + value
		return visits%limit != 0
	}
	b.ReportAllocs()
	for b.Loop() {
		m.RangeFunc(yield)
	}
	workloadValue = sum
	validateEarlyStopWorkloadResult(b, visits, sum, keys[:limit], values[:limit])
	validateWorkloadState(b, hypermapWorkloadState(m), keys, values)
}

func benchmarkWK8RangeEarlyStopWorkload(b *testing.B, size int) {
	const limit = 4
	keys, values := intWorkloadData(size)
	m := newWK8WorkloadMap(keys, values)
	var visits, sum int
	b.ReportAllocs()
	for b.Loop() {
		seen := 0
		for pair := m.Oldest(); pair != nil; pair = pair.Next() {
			visits++
			sum += pair.Key + pair.Value
			seen++
			if seen == limit {
				break
			}
		}
	}
	workloadValue = sum
	validateEarlyStopWorkloadResult(b, visits, sum, keys[:limit], values[:limit])
	validateWorkloadState(b, wk8WorkloadState(m), keys, values)
}

func benchmarkElliottRangeEarlyStopWorkload(b *testing.B, size int) {
	const limit = 4
	keys, values := intWorkloadData(size)
	m := newElliottWorkloadMap(keys, values)
	var visits, sum int
	yield := func(key, value int) bool {
		visits++
		sum += key + value
		return visits%limit != 0
	}
	b.ReportAllocs()
	for b.Loop() {
		m.AllFromFront()(yield)
	}
	workloadValue = sum
	validateEarlyStopWorkloadResult(b, visits, sum, keys[:limit], values[:limit])
	validateWorkloadState(b, elliottWorkloadState(m), keys, values)
}

func benchmarkLorenzoRangeEarlyStopWorkload(b *testing.B, size int) {
	const limit = 4
	keys, values := intWorkloadData(size)
	m := newLorenzoWorkloadMap(keys, values)
	var visits, sum int
	yield := func(key, value int) bool {
		visits++
		sum += key + value
		return visits%limit != 0
	}
	b.ReportAllocs()
	for b.Loop() {
		m.Range(yield)
	}
	workloadValue = sum
	validateEarlyStopWorkloadResult(b, visits, sum, keys[:limit], values[:limit])
	validateWorkloadState(b, lorenzoWorkloadState(m), keys, values)
}

func newHypermapWorkloadMap[K comparable](keys []K, values []int) *hypermap.Map[K, int] {
	m := hypermap.New[K, int](len(keys))
	for i, key := range keys {
		m.Set(key, values[i])
	}
	return &m
}

func newWK8WorkloadMap[K comparable](keys []K, values []int) *wk8.OrderedMap[K, int] {
	m := wk8.New[K, int](len(keys))
	for i, key := range keys {
		m.Set(key, values[i])
	}
	return m
}

func newElliottWorkloadMap[K comparable](keys []K, values []int) *elliott.OrderedMap[K, int] {
	m := elliott.NewOrderedMapWithCapacity[K, int](len(keys))
	for i, key := range keys {
		m.Set(key, values[i])
	}
	return m
}

func newLorenzoWorkloadMap[K comparable](keys []K, values []int) *lorenzo.OrderedMap[K, int] {
	m := lorenzo.New[K, int]()
	for i, key := range keys {
		if err := m.PushBack(key, values[i]); err != nil {
			panic(err)
		}
	}
	return m
}

func hypermapWorkloadState[K comparable](m *hypermap.Map[K, int]) workloadState[K] {
	return workloadState[K]{
		name:     "hypermap",
		length:   m.Len,
		get:      m.Get,
		rangeAll: m.RangeFunc,
	}
}

func wk8WorkloadState[K comparable](m *wk8.OrderedMap[K, int]) workloadState[K] {
	return workloadState[K]{
		name:   "wk8",
		length: m.Len,
		get:    m.Get,
		rangeAll: func(yield func(K, int) bool) {
			for pair := m.Oldest(); pair != nil; pair = pair.Next() {
				if !yield(pair.Key, pair.Value) {
					return
				}
			}
		},
	}
}

func elliottWorkloadState[K comparable](m *elliott.OrderedMap[K, int]) workloadState[K] {
	return workloadState[K]{
		name:     "elliotchance",
		length:   m.Len,
		get:      m.Get,
		rangeAll: m.AllFromFront(),
	}
}

func lorenzoWorkloadState[K comparable](m *lorenzo.OrderedMap[K, int]) workloadState[K] {
	return workloadState[K]{
		name:     "lorenzosaino",
		length:   m.Len,
		get:      m.Get,
		rangeAll: m.Range,
	}
}

func validateWorkloadState[K comparable](tb testing.TB, state workloadState[K], keys []K, values []int) {
	tb.Helper()
	if got := state.length(); got != len(keys) {
		tb.Fatalf("%s length = %d, want %d", state.name, got, len(keys))
	}
	for i, key := range keys {
		value, ok := state.get(key)
		if !ok || value != values[i] {
			tb.Fatalf("%s Get(%v) = (%d, %t), want (%d, true)", state.name, key, value, ok, values[i])
		}
	}
	position := 0
	state.rangeAll(func(key K, value int) bool {
		if position >= len(keys) {
			tb.Errorf("%s range returned extra key %v", state.name, key)
			return false
		}
		if key != keys[position] || value != values[position] {
			tb.Errorf(
				"%s range[%d] = (%v, %d), want (%v, %d)",
				state.name,
				position,
				key,
				value,
				keys[position],
				values[position],
			)
			return false
		}
		position++
		return true
	})
	if position != len(keys) {
		tb.Fatalf("%s range length = %d, want %d", state.name, position, len(keys))
	}
}

func validateGetWorkloadResult(b *testing.B, value int, ok bool, iterations int, values []int, hit bool) {
	b.Helper()
	if ok != hit {
		b.Fatalf("last lookup presence = %t, want %t", ok, hit)
	}
	if hit {
		want := values[(iterations-1)&(len(values)-1)]
		if value != want {
			b.Fatalf("last lookup value = %d, want %d", value, want)
		}
	}
}

func validateRangeWorkloadResult(b *testing.B, sum int, keys, values []int) {
	b.Helper()
	want := b.N * workloadPairSum(keys, values)
	if sum != want {
		b.Fatalf("range sum = %d, want %d", sum, want)
	}
}

func validateEarlyStopWorkloadResult(b *testing.B, visits, sum int, keys, values []int) {
	b.Helper()
	if want := b.N * len(keys); visits != want {
		b.Fatalf("range visits = %d, want %d", visits, want)
	}
	validateRangeWorkloadResult(b, sum, keys, values)
}

func validateChurnWorkloadResult(
	b *testing.B,
	state workloadState[int],
	live []int,
	next int,
	deleted bool,
	inserted bool,
) {
	b.Helper()
	if !deleted || !inserted {
		b.Fatalf("last churn operation deleted=%v inserted=%v, want true/true", deleted, inserted)
	}
	if got := state.length(); got != len(live) {
		b.Fatalf("length after churn = %d, want %d", got, len(live))
	}
	latest := next - 1
	if value, ok := state.get(latest); !ok || value != latest {
		b.Fatalf("Get(latest=%d) = (%d, %v), want (%d, true)", latest, value, ok, latest)
	}
	want := make(map[int]struct{}, len(live))
	for _, key := range live {
		want[key] = struct{}{}
		if value, ok := state.get(key); !ok || value != key {
			b.Fatalf("Get(%d) after churn = (%d, %v), want (%d, true)", key, value, ok, key)
		}
	}
	state.rangeAll(func(key, value int) bool {
		if _, ok := want[key]; !ok || value != key {
			b.Errorf("unexpected ranged entry after churn: (%d, %d)", key, value)
			return false
		}
		delete(want, key)
		return true
	})
	if len(want) != 0 {
		b.Fatalf("range missed %d live entries after churn", len(want))
	}
}

func workloadPairSum(keys, values []int) int {
	var sum int
	for i, key := range keys {
		sum += key + values[i]
	}
	return sum
}

func intWorkloadData(size int) ([]int, []int) {
	keys := make([]int, size)
	values := make([]int, size)
	for i := range size {
		keys[i] = i
		values[i] = i
	}
	return keys, values
}

func missingIntWorkloadKeys(size int) []int {
	keys := make([]int, size)
	for i := range size {
		keys[i] = size + i
	}
	return keys
}

func stringWorkloadData(size int, prefix string) ([]string, []int) {
	keys := make([]string, size)
	values := make([]int, size)
	for i := range size {
		keys[i] = prefix + strconv.Itoa(i)
		values[i] = i
	}
	return keys, values
}

func fragmentedIntWorkloadData(size int) ([]int, []int) {
	keys := make([]int, 0, size/2)
	values := make([]int, 0, size/2)
	for i := 0; i < size; i += 2 {
		keys = append(keys, i)
		values = append(values, i)
	}
	return keys, values
}

func replacementWorkloadValues(size, iterations int) []int {
	values := make([]int, size)
	for i := range size {
		values[i] = i
		if i < iterations {
			last := i + ((iterations-1-i)/size)*size
			values[i] = last + size
		}
	}
	return values
}

func deleteOddHypermapWorkloadKeys(m *hypermap.Map[int, int], size int) {
	for key := 1; key < size; key += 2 {
		m.Delete(key)
	}
}

func deleteOddWK8WorkloadKeys(m *wk8.OrderedMap[int, int], size int) {
	for key := 1; key < size; key += 2 {
		m.Delete(key)
	}
}

func deleteOddElliottWorkloadKeys(m *elliott.OrderedMap[int, int], size int) {
	for key := 1; key < size; key += 2 {
		m.Delete(key)
	}
}

func deleteOddLorenzoWorkloadKeys(m *lorenzo.OrderedMap[int, int], size int) {
	for key := 1; key < size; key += 2 {
		m.Delete(key)
	}
}
