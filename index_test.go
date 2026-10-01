package hypermap

import (
	"math"
	"math/rand"
	"strconv"
	"strings"
	"testing"
)

type testNamedInt int

type testPairKey struct {
	a int
	b string
}

func checkDistinctKeys[K comparable](t *testing.T, name string, keys []K) {
	t.Helper()
	var m Map[K, int]
	for i, key := range keys {
		if _, ok := m.Set(key, i); ok {
			t.Fatalf("%s: Set(%v) reported an existing key", name, key)
		}
	}
	if m.Len() != len(keys) {
		t.Fatalf("%s: Len = %d, want %d", name, m.Len(), len(keys))
	}
	for i, key := range keys {
		if got, ok := m.Get(key); !ok || got != i {
			t.Fatalf("%s: Get(%v) = (%d, %v), want (%d, true)", name, key, got, ok, i)
		}
	}
	for i := 0; i < len(keys); i += 2 {
		if got, ok := m.Delete(keys[i]); !ok || got != i {
			t.Fatalf("%s: Delete(%v) = (%d, %v), want (%d, true)", name, keys[i], got, ok, i)
		}
	}
	for i, key := range keys {
		got, ok := m.Get(key)
		if want := i%2 == 1; ok != want || (ok && got != i) {
			t.Fatalf("%s: Get(%v) after deletes = (%d, %v), want present=%v", name, key, got, ok, want)
		}
	}
	checkIndexInvariants(t, &m)
}

func TestMapHashesEveryKeyKind(t *testing.T) {
	const n = 1000
	ints := make([]int, n)
	int64s := make([]int64, n)
	int32s := make([]int32, n)
	int16s := make([]int16, n)
	uints := make([]uint, n)
	uint64s := make([]uint64, n)
	uint32s := make([]uint32, n)
	uint16s := make([]uint16, n)
	uintptrs := make([]uintptr, n)
	named := make([]testNamedInt, n)
	floats := make([]float64, n)
	float32s := make([]float32, n)
	pairs := make([]testPairKey, n)
	arrays := make([][3]byte, n)
	pointers := make([]*int, n)
	for i := range n {
		v := i*7919 - n*n
		ints[i] = v
		int64s[i] = int64(v) << 20
		int32s[i] = int32(v)
		int16s[i] = int16(i - n/2)
		uints[i] = uint(i) << 20
		uint64s[i] = uint64(i) * 0x9e3779b97f4a7c15
		uint32s[i] = uint32(i) << 16
		uint16s[i] = uint16(i * 31)
		uintptrs[i] = uintptr(i) << 3
		named[i] = testNamedInt(v)
		floats[i] = float64(v) / 3
		float32s[i] = float32(v) / 7
		pairs[i] = testPairKey{a: i % 7, b: strconv.Itoa(i / 7)}
		arrays[i] = [3]byte{byte(i), byte(i >> 8), 1}
		pointers[i] = new(int)
	}
	int8s := make([]int8, 0, 256)
	uint8s := make([]uint8, 0, 256)
	for i := range 256 {
		int8s = append(int8s, int8(i))
		uint8s = append(uint8s, uint8(i))
	}
	checkDistinctKeys(t, "int", ints)
	checkDistinctKeys(t, "int64", int64s)
	checkDistinctKeys(t, "int32", int32s)
	checkDistinctKeys(t, "int16", int16s)
	checkDistinctKeys(t, "int8", int8s)
	checkDistinctKeys(t, "uint", uints)
	checkDistinctKeys(t, "uint64", uint64s)
	checkDistinctKeys(t, "uint32", uint32s)
	checkDistinctKeys(t, "uint16", uint16s)
	checkDistinctKeys(t, "uint8", uint8s)
	checkDistinctKeys(t, "uintptr", uintptrs)
	checkDistinctKeys(t, "named int", named)
	checkDistinctKeys(t, "float64", floats)
	checkDistinctKeys(t, "float32", float32s)
	checkDistinctKeys(t, "struct", pairs)
	checkDistinctKeys(t, "array", arrays)
	checkDistinctKeys(t, "pointer", pointers)
	checkDistinctKeys(t, "bool", []bool{false, true})
	checkDistinctKeys(t, "interface", []any{
		1, int64(1), int32(1), int16(1), int8(1), uint(1), uint64(1), uint32(1),
		uint16(1), uint8(1), uintptr(1), "1", 1.0, float32(1), testNamedInt(1),
		testPairKey{a: 1}, [3]byte{1}, nil, true, "", 0, "x",
	})
}

func TestMapSignedZeroFloatKeys(t *testing.T) {
	negativeZero := math.Copysign(0, -1)
	var m Map[float64, int]
	m.Set(0, 1)
	if old, ok := m.Set(negativeZero, 2); !ok || old != 1 {
		t.Fatalf("Set(-0) = (%d, %v), want (1, true)", old, ok)
	}
	var a Map[any, int]
	a.Set(0.0, 1)
	if old, ok := a.Set(negativeZero, 2); !ok || old != 1 {
		t.Fatalf("Set(any(-0)) = (%d, %v), want (1, true)", old, ok)
	}
	var f Map[float32, int]
	f.Set(0, 1)
	if old, ok := f.Set(float32(negativeZero), 2); !ok || old != 1 {
		t.Fatalf("Set(float32(-0)) = (%d, %v), want (1, true)", old, ok)
	}
	if m.Len() != 1 || a.Len() != 1 || f.Len() != 1 {
		t.Fatalf("Len = %d, %d, and %d, want 1", m.Len(), a.Len(), f.Len())
	}
}

func TestMapStringKeysOfEveryLength(t *testing.T) {
	var keys []string
	for n := range 300 {
		base := []byte(strings.Repeat("k", n))
		keys = append(keys, string(base))
		for p := range n {
			// Each variant differs from base in one byte, so a hasher that
			// skips any position collides and fails the distinctness check.
			variant := append([]byte(nil), base...)
			variant[p] = 'j'
			keys = append(keys, string(variant))
		}
	}
	checkDistinctKeys(t, "string", keys)

	var m Map[string, int]
	m.Set("seed", 0)
	seen := make(map[uint64]string, len(keys))
	for _, key := range keys {
		hash := m.index.hash(key)
		if other, ok := seen[hash]; ok {
			t.Fatalf("hash(%q) == hash(%q)", key, other)
		}
		seen[hash] = key
		if again := m.index.hash(key); again != hash {
			t.Fatalf("hash(%q) is not deterministic", key)
		}
		copied := strings.Clone(key)
		if got := m.index.hash(copied); got != hash {
			t.Fatalf("hash depends on string storage for %q", key)
		}
	}
}

func TestMapNonReflexiveKeysStayOutOfIndex(t *testing.T) {
	nan := math.NaN()
	var m Map[float64, int]
	m.Set(nan, 1)
	m.Set(nan, 2)
	m.Set(1, 3)
	m.Set(nan, 4)
	if m.Len() != 4 {
		t.Fatalf("Len = %d, want 4", m.Len())
	}
	if _, ok := m.Get(nan); ok || m.Has(nan) {
		t.Fatal("NaN key was found by lookup")
	}
	if _, ok := m.Delete(nan); ok {
		t.Fatal("Delete(NaN) reported a removal")
	}
	if m.index.used != 1 || m.unindexed != 3 {
		t.Fatalf("index used=%d unindexed=%d, want 1 and 3", m.index.used, m.unindexed)
	}
	for key := range 200 {
		m.Set(float64(key+10), key)
	}
	checkIndexInvariants(t, &m)
	if key, value, ok := m.PopFront(); !ok || !math.IsNaN(key) || value != 1 {
		t.Fatalf("PopFront = (%v, %d, %v), want (NaN, 1, true)", key, value, ok)
	}
	m.MoveToBack(1)
	if key, value, ok := m.PopBack(); !ok || key != 1 || value != 3 {
		t.Fatalf("PopBack = (%v, %d, %v), want (1, 3, true)", key, value, ok)
	}
	if m.Len() != 202 {
		t.Fatalf("Len = %d, want 202", m.Len())
	}
	checkIndexInvariants(t, &m)
	m.Clear()
	if m.Len() != 0 || m.unindexed != 0 {
		t.Fatalf("Clear left Len=%d unindexed=%d", m.Len(), m.unindexed)
	}
	checkIndexInvariants(t, &m)

	var a Map[any, int]
	a.Set(nan, 1)
	a.Set("x", 2)
	a.Set(nan, 3)
	if a.Len() != 3 || a.unindexed != 2 {
		t.Fatalf("interface map Len=%d unindexed=%d, want 3 and 2", a.Len(), a.unindexed)
	}
	if key, _, ok := a.PopBack(); !ok || key == key {
		t.Fatalf("PopBack = (%v, %v), want NaN", key, ok)
	}
	checkIndexInvariants(t, &a)
}

func TestMapClearSparseAndDenseIndexes(t *testing.T) {
	m := New[int, int](4096)
	for key := range 10 {
		m.Set(key, key)
	}
	m.Clear()
	checkIndexInvariants(t, &m)
	checkIndexEmpty(t, &m)
	for key := range 4096 {
		if _, ok := m.Set(key, key); ok {
			t.Fatalf("Set(%d) after Clear found a stale entry", key)
		}
	}
	checkIndexInvariants(t, &m)
	for key := 0; key < 4096; key += 3 {
		m.Delete(key)
	}
	checkIndexInvariants(t, &m)
	m.Clear()
	checkIndexInvariants(t, &m)
	checkIndexEmpty(t, &m)

	var zero Map[string, int]
	zero.Clear()
	checkIndexInvariants(t, &zero)
}

func TestMapOverflowCountsSurviveCollidingChurn(t *testing.T) {
	// A one-group index forces every key past the full home group once it
	// grows, exercising overflow increments and their exact reversal.
	m := New[int, int](6)
	for key := range 6 {
		m.Set(key, key)
	}
	checkIndexInvariants(t, &m)
	next := 6
	for round := range 2000 {
		m.Set(next, next)
		next++
		oldest, _, _ := m.Front()
		m.Delete(oldest)
		if round%50 == 0 {
			checkIndexInvariants(t, &m)
		}
	}
	for m.Len() != 0 {
		m.PopBack()
	}
	checkIndexInvariants(t, &m)
	checkIndexEmpty(t, &m)
}

func checkIndexEmpty[T comparable, T2 any](t *testing.T, m *Map[T, T2]) {
	t.Helper()
	for i, group := range m.index.groups {
		if group.control != ([8]byte{}) {
			t.Fatalf("group %d keeps control %v after removing every key", i, group.control)
		}
	}
}

func TestMapDenseArenaTruncation(t *testing.T) {
	m := New[int, int](4)
	for key := 1; key <= 4; key++ {
		m.Set(key, key*10)
	}
	if key, value, ok := m.PopBack(); !ok || key != 4 || value != 40 {
		t.Fatalf("PopBack = (%d, %d, %v), want (4, 40, true)", key, value, ok)
	}
	if m.fragmented || len(m.entries) != 4 {
		t.Fatalf("PopBack of last slot left fragmented=%v len=%d", m.fragmented, len(m.entries))
	}
	if _, ok := m.Delete(3); !ok || m.fragmented {
		t.Fatalf("Delete of last slot left fragmented=%v", m.fragmented)
	}
	m.Set(5, 50)
	checkIndexInvariants(t, &m)
	checkRangeKeys(t, &m, []int{1, 2, 5})
	m.PopFront()
	if !m.fragmented {
		t.Fatal("PopFront of the first slot left the arena dense")
	}
	checkRangeKeys(t, &m, []int{2, 5})
	checkIndexInvariants(t, &m)
}

func TestMapRangeYieldMayDeleteCurrent(t *testing.T) {
	for _, fragmented := range []bool{false, true} {
		m := New[int, int](64)
		for key := range 64 {
			m.Set(key, key)
		}
		var want []int
		if fragmented {
			m.MoveToFront(63)
			want = append(want, 63)
			for key := range 63 {
				want = append(want, key)
			}
		} else {
			for key := range 64 {
				want = append(want, key)
			}
		}
		var got []int
		for key := range m.Range() {
			got = append(got, key)
			m.Delete(key)
		}
		if !slicesEqual(got, want) {
			t.Fatalf("fragmented=%v: visited %v, want %v", fragmented, got, want)
		}
		if m.Len() != 0 {
			t.Fatalf("fragmented=%v: Len = %d after deleting every entry", fragmented, m.Len())
		}
		checkIndexInvariants(t, &m)
	}
}

func TestMapRandomizedWideKeysAgainstModel(t *testing.T) {
	const (
		steps   = 20000
		keySpan = 600
	)
	for _, seed := range []int64{7, 0x5eed, 0xdecafbad} {
		rng := rand.New(rand.NewSource(seed))
		var got Map[int, int]
		if seed != 7 {
			got = New[int, int](keySpan / 4)
		}
		want := make(map[int]int)
		var order []int
		remove := func(key int) {
			for i, candidate := range order {
				if candidate == key {
					order = append(order[:i], order[i+1:]...)
					return
				}
			}
		}
		for step := range steps {
			key := rng.Intn(keySpan)
			switch op := rng.Intn(100); {
			case op < 45:
				_, existed := want[key]
				if _, ok := got.Set(key, step); ok != existed {
					t.Fatalf("seed %d step %d: Set(%d) existed=%v, want %v", seed, step, key, ok, existed)
				}
				if !existed {
					order = append(order, key)
				}
				want[key] = step
			case op < 80:
				value, existed := want[key]
				if got, ok := got.Delete(key); ok != existed || got != value {
					t.Fatalf("seed %d step %d: Delete(%d) = (%d, %v), want (%d, %v)", seed, step, key, got, ok, value, existed)
				}
				delete(want, key)
				remove(key)
			case op < 88:
				if _, ok := want[key]; ok {
					got.MoveToFront(key)
					remove(key)
					order = append([]int{key}, order...)
				}
			case op < 94:
				if k, _, ok := got.PopFront(); ok {
					if k != order[0] {
						t.Fatalf("seed %d step %d: PopFront key %d, want %d", seed, step, k, order[0])
					}
					delete(want, k)
					order = order[1:]
				}
			case op < 99:
				if k, _, ok := got.PopBack(); ok {
					if k != order[len(order)-1] {
						t.Fatalf("seed %d step %d: PopBack key %d, want %d", seed, step, k, order[len(order)-1])
					}
					delete(want, k)
					order = order[:len(order)-1]
				}
			default:
				got.Clear()
				clear(want)
				order = order[:0]
			}
			if step%97 == 0 || step == steps-1 {
				checkIndexInvariants(t, &got)
				checkRangeKeys(t, &got, order)
				for k := range keySpan {
					value, ok := got.Get(k)
					wantValue, wantOK := want[k]
					if ok != wantOK || value != wantValue {
						t.Fatalf("seed %d step %d: Get(%d) = (%d, %v), want (%d, %v)", seed, step, k, value, ok, wantValue, wantOK)
					}
				}
			}
		}
	}
}

func TestMapDifferentKeyChurnKeepsInvariants(t *testing.T) {
	for _, size := range []int{7, 56, 7 * 64} {
		m := New[int, int](size)
		for key := range size {
			m.Set(key, key)
		}
		next := size
		for round := range 20 {
			for range size * 3 {
				oldest, _, _ := m.Front()
				m.Delete(oldest)
				m.Set(next, next)
				next++
			}
			if m.Len() != size {
				t.Fatalf("size %d round %d: Len = %d", size, round, m.Len())
			}
			checkIndexInvariants(t, &m)
		}
	}
}

func TestMapSteadyStateOperationsDoNotAllocate(t *testing.T) {
	ints := New[int, int](1024)
	strs := New[string, int](1024)
	keys := make([]string, 1024)
	for i := range 1024 {
		keys[i] = "key/" + strconv.Itoa(i)
		ints.Set(i, i)
		strs.Set(keys[i], i)
	}
	var sum int
	operations := map[string]func(){
		"Get": func() {
			for i := range 1024 {
				v, _ := ints.Get(i)
				w, _ := strs.Get(keys[i])
				sum += v + w
			}
		},
		"Set existing": func() {
			for i := range 1024 {
				ints.Set(i, i)
				strs.Set(keys[i], i)
			}
		},
		"Replace": func() {
			for i := range 1024 {
				ints.Replace(i, i)
				strs.Replace(keys[i], i)
			}
		},
		"Delete and Set": func() {
			for i := range 1024 {
				ints.Delete(i)
				ints.Set(i, i)
				strs.Delete(keys[i])
				strs.Set(keys[i], i)
			}
		},
		"Move": func() {
			for i := range 1024 {
				ints.MoveToFront(i)
				strs.MoveToBack(keys[i])
			}
		},
		"Next and Prev": func() {
			for i := range 1024 {
				_, v, _ := ints.Next(i)
				_, w, _ := strs.Prev(keys[i])
				sum += v + w
			}
		},
		"Pop and Set": func() {
			for range 1024 {
				k, v, _ := ints.PopFront()
				ints.Set(k, v)
				s, w, _ := strs.PopBack()
				strs.Set(s, w)
			}
		},
		"Range": func() {
			for k, v := range ints.Range() {
				sum += k + v
			}
			strs.RangeFunc(func(_ string, v int) bool {
				sum += v
				return true
			})
		},
		"Clear and refill": func() {
			ints.Clear()
			strs.Clear()
			for i := range 1024 {
				ints.Set(i, i)
				strs.Set(keys[i], i)
			}
		},
	}
	for name, operation := range operations {
		operation()
		if allocations := testing.AllocsPerRun(5, operation); allocations != 0 {
			t.Errorf("%s allocated %.1f times per run, want 0", name, allocations)
		}
	}
	benchmarkValue = sum
}

func checkRangeKeys(t *testing.T, m *Map[int, int], want []int) {
	t.Helper()
	var got []int
	m.RangeFunc(func(key, _ int) bool {
		got = append(got, key)
		return true
	})
	if !slicesEqual(got, want) {
		t.Fatalf("range keys = %v, want %v", got, want)
	}
}

func slicesEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func checkIndexInvariants[T comparable, T2 any](t *testing.T, m *Map[T, T2]) {
	t.Helper()
	index := &m.index
	groups := index.groups
	mask := len(groups) - 1
	overflow := make([]int, len(groups))
	var used uint32
	for groupIndex := range groups {
		group := &groups[groupIndex]
		for slot := range indexGroupSlots {
			control := group.control[slot]
			if control == 0 {
				continue
			}
			if control&indexFullControl == 0 {
				t.Fatalf("group %d slot %d has invalid control %#x", groupIndex, slot, control)
			}
			used++
			ref := group.refs[slot]
			if ref == 0 || int(ref) >= len(m.entries) {
				t.Fatalf("group %d slot %d holds invalid ref %d", groupIndex, slot, ref)
			}
			entry := &m.entries[ref]
			if entry.prev == indexFreeRef || entry.key != entry.key {
				t.Fatalf("group %d slot %d references unindexable slot %d", groupIndex, slot, ref)
			}
			hash := index.hash(entry.key)
			if want := byte(hash&0x7f | indexFullControl); control != want {
				t.Fatalf("group %d slot %d tag %#x, want %#x", groupIndex, slot, control, want)
			}
			if found := m.find(entry.key); found != entry {
				t.Fatalf("key %v at ref %d is not reachable by probing", entry.key, ref)
			}
			// Every group before this one on the key's probe sequence
			// counted the key's passage.
			probe := int(hash>>7) & mask
			for step := 1; probe != groupIndex; step++ {
				if step > len(groups) {
					t.Fatalf("group %d is not on the probe sequence of key %v", groupIndex, entry.key)
				}
				overflow[probe]++
				probe = (probe + step) & mask
			}
		}
	}
	for groupIndex := range groups {
		got := int(groups[groupIndex].control[indexOverflowByte])
		if want := overflow[groupIndex]; got != want && !(got == indexOverflowMax && want >= indexOverflowMax) {
			t.Fatalf("group %d overflow count %d, want %d", groupIndex, got, want)
		}
	}
	if used != index.used {
		t.Fatalf("index has %d full slots, used = %d", used, index.used)
	}
	if len(groups) != 0 && index.used > index.maxUsed {
		t.Fatalf("used %d exceeds maxUsed %d", index.used, index.maxUsed)
	}
	var live, unindexed int
	for ref := 1; ref < len(m.entries); ref++ {
		entry := &m.entries[ref]
		if entry.prev == indexFreeRef {
			continue
		}
		live++
		if entry.key != entry.key {
			unindexed++
		}
	}
	if live != m.Len() || unindexed != int(m.unindexed) {
		t.Fatalf("arena has %d live and %d unindexed entries, Len = %d, unindexed = %d", live, unindexed, m.Len(), m.unindexed)
	}
	if len(m.entries) == 0 {
		return
	}
	visited, previous := 0, uint32(0)
	for ref := m.entries[0].next; ref != 0; ref = m.entries[ref].next {
		if m.entries[ref].prev != previous {
			t.Fatalf("ref %d has prev %d, want %d", ref, m.entries[ref].prev, previous)
		}
		if !m.fragmented && int(ref) != visited+1 {
			t.Fatalf("dense arena visits ref %d at position %d", ref, visited)
		}
		previous = ref
		visited++
		if visited > live {
			t.Fatal("list cycle detected")
		}
	}
	if visited != live || m.entries[0].prev != previous {
		t.Fatalf("list holds %d entries ending at %d, want %d ending at %d", visited, previous, live, m.entries[0].prev)
	}
}

func TestMapSaturatedOverflowCountsStayCorrect(t *testing.T) {
	m := New[int, int](600)
	for key := range 500 {
		m.Set(key, key)
	}
	// Saturated counts are never decremented; lookups merely probe past
	// them. Force saturation everywhere and check behavior against a model.
	for i := range m.index.groups {
		m.index.groups[i].control[indexOverflowByte] = indexOverflowMax
	}
	want := make(map[int]bool)
	for key := range 500 {
		want[key] = true
	}
	for key := 0; key < 500; key += 2 {
		if _, ok := m.Delete(key); !ok {
			t.Fatalf("Delete(%d) missed a present key", key)
		}
		delete(want, key)
	}
	for key := 500; key < 550; key++ {
		m.Set(key, key)
		want[key] = true
	}
	for key := -10; key < 600; key++ {
		if _, ok := m.Get(key); ok != want[key] {
			t.Fatalf("Get(%d) presence = %v, want %v", key, ok, want[key])
		}
	}
	for i := range m.index.groups {
		if got := m.index.groups[i].control[indexOverflowByte]; got != indexOverflowMax {
			t.Fatalf("group %d saturated count changed to %d", i, got)
		}
	}
	for m.Len() != 0 {
		m.PopFront()
	}
	if _, ok := m.Get(1); ok {
		t.Fatal("Get found a key in an emptied map")
	}
}

func TestMapLookupTerminatesWhenEveryGroupOverflowed(t *testing.T) {
	m := New[int, int](60)
	for key := range 40 {
		m.Set(key, key)
	}
	for i := range m.index.groups {
		m.index.groups[i].control[indexOverflowByte] = 1
	}
	for key := 1000; key < 1100; key++ {
		if m.Has(key) {
			t.Fatalf("Has(%d) found an absent key", key)
		}
		if _, ok := m.Replace(key, key); ok {
			t.Fatalf("Replace(%d) replaced an absent key", key)
		}
		if m.MoveToFront(key) {
			t.Fatalf("MoveToFront(%d) moved an absent key", key)
		}
		if _, ok := m.Delete(key); ok {
			t.Fatalf("Delete(%d) removed an absent key", key)
		}
	}
	for key := range 40 {
		if got, ok := m.Get(key); !ok || got != key {
			t.Fatalf("Get(%d) = (%d, %v), want (%d, true)", key, got, ok, key)
		}
	}
}
