package hypermap

import (
	"math"
	"math/rand"
	"net/url"
	"slices"
	"testing"
)

const benchmarkSize = 4096

var (
	benchmarkValue   int
	benchmarkOK      bool
	benchmarkEncoded string
)

func TestMapSetGetAndOrder(t *testing.T) {
	var m Map[string, int]

	if old, ok := m.Set("b", 2); ok || old != 0 {
		t.Fatalf("Set inserted key returned old=%d ok=%v", old, ok)
	}
	m.Set("a", 1)
	m.Set("c", 3)

	if old, ok := m.Set("a", 11); !ok || old != 1 {
		t.Fatalf("Set replaced key returned old=%d ok=%v", old, ok)
	}
	if old, ok := m.Replace("a", 12); !ok || old != 11 {
		t.Fatalf("Replace returned old=%d ok=%v", old, ok)
	}
	if old, ok := m.Replace("missing", 13); ok || old != 0 {
		t.Fatalf("Replace missing key returned old=%d ok=%v", old, ok)
	}
	if got, ok := m.Get("a"); !ok || got != 12 {
		t.Fatalf("Get returned value=%d ok=%v", got, ok)
	}

	wantKeys := []string{"b", "a", "c"}
	wantValues := []int{2, 12, 3}
	var i int
	for key, value := range m.Range() {
		if i >= len(wantKeys) {
			t.Fatalf("Range yielded extra key=%q value=%d", key, value)
		}
		if key != wantKeys[i] || value != wantValues[i] {
			t.Fatalf("Range[%d] = %q/%d, want %q/%d", i, key, value, wantKeys[i], wantValues[i])
		}
		i++
	}
	if i != len(wantKeys) {
		t.Fatalf("Range yielded %d entries, want %d", i, len(wantKeys))
	}
}

func TestMapEncode(t *testing.T) {
	m := NewQueryMap(4)
	m.Set("b key", []string{"two words", "x+y"})
	m.Set("a", []string{"1"})
	m.Set("empty", nil)
	m.Set("sym", []string{"a&b=c"})
	m.MoveToFront("sym")

	const want = "sym=a%26b%3Dc&b+key=two+words&b+key=x%2By&a=1"
	if got := m.Encode(); got != want {
		t.Fatalf("Encode = %q, want %q", got, want)
	}
}

func TestMapEncodeEmpty(t *testing.T) {
	if got := (*QueryMap)(nil).Encode(); got != "" {
		t.Fatalf("Encode on nil map = %q, want empty string", got)
	}

	var m QueryMap
	if got := m.Encode(); got != "" {
		t.Fatalf("Encode on empty map = %q, want empty string", got)
	}

	m.Set("empty", []string{})
	if got := m.Encode(); got != "" {
		t.Fatalf("Encode with empty value slice = %q, want empty string", got)
	}

	m.Set("blank", []string{""})
	if got := m.Encode(); got != "blank=" {
		t.Fatalf("Encode with blank value = %q, want blank=", got)
	}
}

func TestMapEncodeMatchesQueryEscape(t *testing.T) {
	for i := range 256 {
		s := string([]byte{byte(i)})
		m := NewQueryMap(1)
		m.Set(s, []string{s})

		want := url.QueryEscape(s) + "=" + url.QueryEscape(s)
		if got := m.Encode(); got != want {
			t.Fatalf("Encode byte %d = %q, want %q", i, got, want)
		}
	}
}

func BenchmarkMapGet(b *testing.B) {
	m := newBenchmarkMap(benchmarkSize)

	var (
		value int
		ok    bool
	)
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		value, ok = m.Get(i & (benchmarkSize - 1))
	}

	benchmarkValue = value
	benchmarkOK = ok
}

func BenchmarkMapSetReplace(b *testing.B) {
	m := newBenchmarkMap(benchmarkSize)

	var (
		value int
		ok    bool
	)
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		value, ok = m.Set(i&(benchmarkSize-1), i)
	}

	benchmarkValue = value
	benchmarkOK = ok
}

func BenchmarkMapDeleteSetReuse(b *testing.B) {
	m := newBenchmarkMap(benchmarkSize)

	var (
		value int
		ok    bool
	)
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		key := i & (benchmarkSize - 1)
		value, ok = m.Delete(key)
		m.Set(key, i)
	}

	benchmarkValue = value
	benchmarkOK = ok
}

func BenchmarkMapMoveToFrontBack(b *testing.B) {
	m := newBenchmarkMap(benchmarkSize)

	var ok bool
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		key := i & (benchmarkSize - 1)
		ok = m.MoveToFront(key)
		ok = m.MoveToBack(key) && ok
	}

	benchmarkOK = ok
}

func BenchmarkMapRange(b *testing.B) {
	m := newBenchmarkMap(benchmarkSize)

	var value int
	b.ReportAllocs()
	for b.Loop() {
		for key, item := range m.Range() {
			value += key ^ item
		}
	}

	benchmarkValue = value
}

func BenchmarkMapEncode(b *testing.B) {
	m := NewQueryMap(8)
	m.Set("alpha", []string{"1", "2"})
	m.Set("with space", []string{"two words", "x+y"})
	m.Set("symbol", []string{"a&b=c"})
	m.Set("empty", nil)
	m.Set("bytes", []string{"raw-\xff"})
	m.Set("tail", []string{""})

	var encoded string
	b.ReportAllocs()
	for b.Loop() {
		encoded = m.Encode()
	}

	benchmarkEncoded = encoded
}

func newBenchmarkMap(size int) Map[int, int] {
	m := New[int, int](size)
	for i := range size {
		m.Set(i, i)
	}
	return m
}

func TestMapDeleteReusesSlot(t *testing.T) {
	m := New[int, int](3)
	m.Set(1, 10)
	m.Set(2, 20)
	m.Set(3, 30)

	if got, ok := m.Delete(2); !ok || got != 20 {
		t.Fatalf("Delete returned value=%d ok=%v", got, ok)
	}
	if m.Has(2) {
		t.Fatal("deleted key is present")
	}

	m.Set(4, 40)
	if m.Len() != 3 {
		t.Fatalf("Len = %d, want 3", m.Len())
	}
	if m.Cap() != 3 {
		t.Fatalf("Cap = %d, want 3", m.Cap())
	}

	want := []int{1, 3, 4}
	var i int
	for key, value := range m.Range() {
		if i >= len(want) {
			t.Fatalf("Range yielded extra key=%d value=%d", key, value)
		}
		if key != want[i] {
			t.Fatalf("Range[%d] key = %d, want %d", i, key, want[i])
		}
		i++
	}
	if i != len(want) {
		t.Fatalf("Range yielded %d entries, want %d", i, len(want))
	}
}

func TestMapMoveAndPop(t *testing.T) {
	m := New[int, string](4)
	m.Set(1, "a")
	m.Set(2, "b")
	m.Set(3, "c")
	m.Set(4, "d")

	if !m.MoveToFront(3) {
		t.Fatal("MoveToFront returned false")
	}
	if !m.MoveToBack(1) {
		t.Fatal("MoveToBack returned false")
	}

	if key, value, ok := m.Front(); !ok || key != 3 || value != "c" {
		t.Fatalf("Front = %d/%q/%v, want 3/c/true", key, value, ok)
	}
	if key, value, ok := m.Back(); !ok || key != 1 || value != "a" {
		t.Fatalf("Back = %d/%q/%v, want 1/a/true", key, value, ok)
	}
	if key, value, ok := m.Next(3); !ok || key != 2 || value != "b" {
		t.Fatalf("Next = %d/%q/%v, want 2/b/true", key, value, ok)
	}
	if key, value, ok := m.Prev(1); !ok || key != 4 || value != "d" {
		t.Fatalf("Prev = %d/%q/%v, want 4/d/true", key, value, ok)
	}

	if key, value, ok := m.PopFront(); !ok || key != 3 || value != "c" {
		t.Fatalf("PopFront = %d/%q/%v, want 3/c/true", key, value, ok)
	}
	if key, value, ok := m.PopBack(); !ok || key != 1 || value != "a" {
		t.Fatalf("PopBack = %d/%q/%v, want 1/a/true", key, value, ok)
	}
	if m.Len() != 2 {
		t.Fatalf("Len = %d, want 2", m.Len())
	}
}

func TestMapClearAndReset(t *testing.T) {
	m := New[int, int](2)
	m.Set(1, 1)
	m.Set(2, 2)
	m.Clear()

	if m.Len() != 0 {
		t.Fatalf("Len after Clear = %d, want 0", m.Len())
	}
	if m.Cap() != 2 {
		t.Fatalf("Cap after Clear = %d, want 2", m.Cap())
	}
	if _, _, ok := m.Front(); ok {
		t.Fatal("Front after Clear returned ok")
	}

	m.Set(3, 3)
	if got, ok := m.Get(3); !ok || got != 3 {
		t.Fatalf("Get after Clear returned value=%d ok=%v", got, ok)
	}

	m.Reset()
	if m.Len() != 0 {
		t.Fatalf("Len after Reset = %d, want 0", m.Len())
	}
	if m.Cap() != 0 {
		t.Fatalf("Cap after Reset = %d, want 0", m.Cap())
	}
}

func TestMapRejectsInvalidCapacity(t *testing.T) {
	tests := [...]struct {
		name     string
		capacity int
	}{
		{name: "negative", capacity: -1},
		{name: "maximum int", capacity: int(^uint(0) >> 1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("Init(%d) did not panic", test.capacity)
				}
			}()
			var m Map[int, int]
			m.Init(test.capacity)
		})
	}
}

func TestMapRandomizedAgainstModel(t *testing.T) {
	const steps = 2000
	seeds := [...]int64{1, 0xc0ffee, 0x6a09e667f3bcc909}

	for seedIndex, seed := range seeds {
		rng := rand.New(rand.NewSource(seed))
		var got Map[int, int]
		if seedIndex != 0 {
			got = New[int, int](testMapKeyMax - testMapKeyMin + 1)
		}
		var want mapModel

		for step := range steps {
			key := rng.Intn(testMapKeyMax-testMapKeyMin+1) + testMapKeyMin
			value := rng.Intn(101) - 50
			operation := ""

			switch rng.Intn(32) {
			case 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10:
				operation = "Set"
				wantOld, wantReplaced := want.set(key, value)
				gotOld, gotReplaced := got.Set(key, value)
				if gotOld != wantOld || gotReplaced != wantReplaced {
					t.Fatalf("seed %#x step %d: Set(%d, %d) = (%d, %v), want (%d, %v)", seed, step, key, value, gotOld, gotReplaced, wantOld, wantReplaced)
				}
			case 11:
				operation = "Replace"
				wantOld, wantReplaced := want.get(key)
				gotOld, gotReplaced := got.Replace(key, value)
				if wantReplaced {
					want.values[key] = value
				}
				if gotOld != wantOld || gotReplaced != wantReplaced {
					t.Fatalf("seed %#x step %d: Replace(%d, %d) = (%d, %v), want (%d, %v)", seed, step, key, value, gotOld, gotReplaced, wantOld, wantReplaced)
				}
			case 12, 13, 14, 15:
				operation = "Get"
				wantValue, wantOK := want.get(key)
				gotValue, gotOK := got.Get(key)
				if gotValue != wantValue || gotOK != wantOK {
					t.Fatalf("seed %#x step %d: Get(%d) = (%d, %v), want (%d, %v)", seed, step, key, gotValue, gotOK, wantValue, wantOK)
				}
			case 16, 17, 18:
				operation = "Delete"
				wantValue, wantOK := want.delete(key)
				gotValue, gotOK := got.Delete(key)
				if gotValue != wantValue || gotOK != wantOK {
					t.Fatalf("seed %#x step %d: Delete(%d) = (%d, %v), want (%d, %v)", seed, step, key, gotValue, gotOK, wantValue, wantOK)
				}
			case 19:
				operation = "MoveToFront"
				wantOK := want.moveToFront(key)
				if gotOK := got.MoveToFront(key); gotOK != wantOK {
					t.Fatalf("seed %#x step %d: MoveToFront(%d) = %v, want %v", seed, step, key, gotOK, wantOK)
				}
			case 20:
				operation = "MoveToBack"
				wantOK := want.moveToBack(key)
				if gotOK := got.MoveToBack(key); gotOK != wantOK {
					t.Fatalf("seed %#x step %d: MoveToBack(%d) = %v, want %v", seed, step, key, gotOK, wantOK)
				}
			case 21:
				operation = "PopFront"
				wantKey, wantValue, wantOK := want.popFront()
				gotKey, gotValue, gotOK := got.PopFront()
				if gotKey != wantKey || gotValue != wantValue || gotOK != wantOK {
					t.Fatalf("seed %#x step %d: PopFront() = (%d, %d, %v), want (%d, %d, %v)", seed, step, gotKey, gotValue, gotOK, wantKey, wantValue, wantOK)
				}
			case 22:
				operation = "PopBack"
				wantKey, wantValue, wantOK := want.popBack()
				gotKey, gotValue, gotOK := got.PopBack()
				if gotKey != wantKey || gotValue != wantValue || gotOK != wantOK {
					t.Fatalf("seed %#x step %d: PopBack() = (%d, %d, %v), want (%d, %d, %v)", seed, step, gotKey, gotValue, gotOK, wantKey, wantValue, wantOK)
				}
			case 23:
				operation = "Clear"
				got.Clear()
				want.clear()
			case 24:
				operation = "Reset"
				got.Reset()
				want.reset()
			case 25:
				operation = "Range"
				limit := 0
				if len(want.order) != 0 {
					limit = 1 + rng.Intn(len(want.order))
				}
				checkMapRangePrefix(t, &got, &want, limit, seed, step)
			case 26:
				operation = "Front"
				wantKey, wantValue, wantOK := want.front()
				gotKey, gotValue, gotOK := got.Front()
				if gotKey != wantKey || gotValue != wantValue || gotOK != wantOK {
					t.Fatalf("seed %#x step %d: Front() = (%d, %d, %v), want (%d, %d, %v)", seed, step, gotKey, gotValue, gotOK, wantKey, wantValue, wantOK)
				}
			case 27:
				operation = "Back"
				wantKey, wantValue, wantOK := want.back()
				gotKey, gotValue, gotOK := got.Back()
				if gotKey != wantKey || gotValue != wantValue || gotOK != wantOK {
					t.Fatalf("seed %#x step %d: Back() = (%d, %d, %v), want (%d, %d, %v)", seed, step, gotKey, gotValue, gotOK, wantKey, wantValue, wantOK)
				}
			case 28:
				operation = "Next"
				wantKey, wantValue, wantOK := want.next(key)
				gotKey, gotValue, gotOK := got.Next(key)
				if gotKey != wantKey || gotValue != wantValue || gotOK != wantOK {
					t.Fatalf("seed %#x step %d: Next(%d) = (%d, %d, %v), want (%d, %d, %v)", seed, step, key, gotKey, gotValue, gotOK, wantKey, wantValue, wantOK)
				}
			case 29:
				operation = "Prev"
				wantKey, wantValue, wantOK := want.prev(key)
				gotKey, gotValue, gotOK := got.Prev(key)
				if gotKey != wantKey || gotValue != wantValue || gotOK != wantOK {
					t.Fatalf("seed %#x step %d: Prev(%d) = (%d, %d, %v), want (%d, %d, %v)", seed, step, key, gotKey, gotValue, gotOK, wantKey, wantValue, wantOK)
				}
			case 30:
				operation = "Set zero key"
				key = 0
				wantOld, wantReplaced := want.set(key, value)
				gotOld, gotReplaced := got.Set(key, value)
				if gotOld != wantOld || gotReplaced != wantReplaced {
					t.Fatalf("seed %#x step %d: Set(0, %d) = (%d, %v), want (%d, %v)", seed, step, value, gotOld, gotReplaced, wantOld, wantReplaced)
				}
			case 31:
				operation = "Delete zero key"
				key = 0
				wantValue, wantOK := want.delete(key)
				gotValue, gotOK := got.Delete(key)
				if gotValue != wantValue || gotOK != wantOK {
					t.Fatalf("seed %#x step %d: Delete(0) = (%d, %v), want (%d, %v)", seed, step, gotValue, gotOK, wantValue, wantOK)
				}
			}

			checkMapMatchesModel(t, &got, &want, seed, step, operation, key)
		}
	}
}

func TestMapZeroKeyAndStorageReuse(t *testing.T) {
	got := New[int, int](3)
	var want mapModel
	capacity := got.Cap()

	got.Set(0, 10)
	want.set(0, 10)
	got.Set(-1, -10)
	want.set(-1, -10)
	got.Set(1, 11)
	want.set(1, 11)

	if value, ok := got.Delete(0); !ok || value != 10 {
		t.Fatalf("Delete(0) = (%d, %v), want (10, true)", value, ok)
	}
	want.delete(0)
	if value, ok := got.Delete(-1); !ok || value != -10 {
		t.Fatalf("Delete(-1) = (%d, %v), want (-10, true)", value, ok)
	}
	want.delete(-1)

	got.Set(2, 22)
	want.set(2, 22)
	got.Set(0, 20)
	want.set(0, 20)
	if got.Cap() != capacity {
		t.Fatalf("Cap after deleted-slot reuse = %d, want %d", got.Cap(), capacity)
	}
	checkMapMatchesModel(t, &got, &want, 0, 0, "deleted-slot reuse", 0)

	got.Clear()
	want.clear()
	got.Set(0, 30)
	want.set(0, 30)
	got.Set(3, 33)
	want.set(3, 33)
	if got.Cap() != capacity {
		t.Fatalf("Cap after Clear reuse = %d, want %d", got.Cap(), capacity)
	}
	checkMapMatchesModel(t, &got, &want, 0, 1, "Clear reuse", 0)

	got.Reset()
	want.reset()
	got.Set(0, 40)
	want.set(0, 40)
	checkMapMatchesModel(t, &got, &want, 0, 2, "Reset reuse", 0)
}

func TestMapRangeStopsWhenYieldReturnsFalse(t *testing.T) {
	got := New[int, int](4)
	var want mapModel
	for key := range 4 {
		got.Set(key, key*10)
		want.set(key, key*10)
	}

	checkMapRangePrefix(t, &got, &want, 1, 0, 0)
	checkMapRangePrefix(t, &got, &want, 2, 0, 1)
	checkMapMatchesModel(t, &got, &want, 0, 2, "stopped Range", 0)

	var empty Map[int, int]
	var emptyModel mapModel
	checkMapRangePrefix(t, &empty, &emptyModel, 0, 0, 3)
}

func TestMapRangeFuncStopsWhenYieldReturnsFalse(t *testing.T) {
	m := New[int, int](4)
	for key := range 4 {
		m.Set(key, key*10)
	}

	var keys []int
	m.RangeFunc(func(key, value int) bool {
		keys = append(keys, key)
		return len(keys) < 2
	})
	if !slices.Equal(keys, []int{0, 1}) {
		t.Fatalf("RangeFunc keys = %v, want [0 1]", keys)
	}
}

func TestMapGrowthReindexesNonReflexiveKeys(t *testing.T) {
	var m Map[float64, int]
	m.Set(math.NaN(), -1)
	for key := range 512 {
		m.Set(float64(key), key)
	}

	if m.Len() != 513 {
		t.Fatalf("Len after growth = %d, want 513", m.Len())
	}
	count := 0
	m.RangeFunc(func(key float64, value int) bool {
		if count == 0 && (!math.IsNaN(key) || value != -1) {
			t.Fatalf("first entry after growth = (%v, %d), want (NaN, -1)", key, value)
		}
		count++
		return true
	})
	if count != m.Len() {
		t.Fatalf("RangeFunc yielded %d entries, Len reports %d", count, m.Len())
	}
	key, value, ok := m.PopFront()
	if !ok || !math.IsNaN(key) || value != -1 {
		t.Fatalf("PopFront after growth = (%v, %d, %v), want (NaN, -1, true)", key, value, ok)
	}
	if m.Len() != 512 {
		t.Fatalf("Len after popping NaN = %d, want 512", m.Len())
	}
}

func TestMapSwissSteadyChurnDoesNotAllocate(t *testing.T) {
	const size = 112
	m := New[int, int](size)
	keys := make([]int, size)
	for key := range size {
		keys[key] = key
		m.Set(key, key)
	}

	next, position := size, 0
	churnBatch := func() {
		for range 10_000 {
			old := keys[position]
			if _, ok := m.Delete(old); !ok {
				panic("existing churn key missing")
			}
			m.Set(next, next)
			keys[position] = next
			next++
			position = (position + 37) % size
		}
	}
	initialGroups := len(m.index.groups)
	churnBatch() // Allow the index to add tombstone headroom when needed.
	if len(m.index.groups) <= initialGroups {
		t.Fatalf("churn left %d index groups, want more than initial %d", len(m.index.groups), initialGroups)
	}
	allocations := testing.AllocsPerRun(1, churnBatch)
	if allocations != 0 {
		t.Fatalf("10,000-operation steady churn batch allocated %.0f times, want 0", allocations)
	}
	if m.Len() != size {
		t.Fatalf("Len after churn = %d, want %d", m.Len(), size)
	}
}

const (
	testMapKeyMin = -8
	testMapKeyMax = 8
)

// mapModel is a behavioral oracle that does not mirror Map slot storage.
type mapModel struct {
	values map[int]int
	order  []int
}

func (m *mapModel) set(key, value int) (int, bool) {
	old, replaced := m.values[key]
	if replaced {
		m.values[key] = value
		return old, true
	}
	if m.values == nil {
		m.values = make(map[int]int)
	}
	m.values[key] = value
	m.order = append(m.order, key)
	return 0, false
}

func (m *mapModel) get(key int) (int, bool) {
	value, ok := m.values[key]
	return value, ok
}

func (m *mapModel) delete(key int) (int, bool) {
	value, ok := m.values[key]
	if !ok {
		return 0, false
	}
	delete(m.values, key)
	position := m.position(key)
	copy(m.order[position:], m.order[position+1:])
	m.order = m.order[:len(m.order)-1]
	return value, true
}

func (m *mapModel) moveToFront(key int) bool {
	position := m.position(key)
	if position < 0 {
		return false
	}
	copy(m.order[1:position+1], m.order[:position])
	m.order[0] = key
	return true
}

func (m *mapModel) moveToBack(key int) bool {
	position := m.position(key)
	if position < 0 {
		return false
	}
	copy(m.order[position:], m.order[position+1:])
	m.order[len(m.order)-1] = key
	return true
}

func (m *mapModel) popFront() (int, int, bool) {
	if len(m.order) == 0 {
		return 0, 0, false
	}
	key := m.order[0]
	value := m.values[key]
	delete(m.values, key)
	copy(m.order, m.order[1:])
	m.order = m.order[:len(m.order)-1]
	return key, value, true
}

func (m *mapModel) popBack() (int, int, bool) {
	if len(m.order) == 0 {
		return 0, 0, false
	}
	last := len(m.order) - 1
	key := m.order[last]
	value := m.values[key]
	delete(m.values, key)
	m.order = m.order[:last]
	return key, value, true
}

func (m *mapModel) clear() {
	clear(m.values)
	m.order = m.order[:0]
}

func (m *mapModel) reset() {
	m.values = nil
	m.order = nil
}

func (m *mapModel) front() (int, int, bool) {
	if len(m.order) == 0 {
		return 0, 0, false
	}
	key := m.order[0]
	return key, m.values[key], true
}

func (m *mapModel) back() (int, int, bool) {
	if len(m.order) == 0 {
		return 0, 0, false
	}
	key := m.order[len(m.order)-1]
	return key, m.values[key], true
}

func (m *mapModel) next(key int) (int, int, bool) {
	position := m.position(key)
	if position < 0 || position == len(m.order)-1 {
		return 0, 0, false
	}
	nextKey := m.order[position+1]
	return nextKey, m.values[nextKey], true
}

func (m *mapModel) prev(key int) (int, int, bool) {
	position := m.position(key)
	if position <= 0 {
		return 0, 0, false
	}
	previousKey := m.order[position-1]
	return previousKey, m.values[previousKey], true
}

func (m *mapModel) position(key int) int {
	for position, candidate := range m.order {
		if candidate == key {
			return position
		}
	}
	return -1
}

func checkMapRangePrefix(t *testing.T, got *Map[int, int], want *mapModel, limit int, seed int64, step int) {
	t.Helper()
	calls := 0
	got.Range()(func(key, value int) bool {
		if calls >= limit {
			t.Fatalf("seed %#x step %d: Range called yield after it returned false; call %d exceeds limit %d", seed, step, calls+1, limit)
		}
		wantKey := want.order[calls]
		wantValue := want.values[wantKey]
		if key != wantKey || value != wantValue {
			t.Fatalf("seed %#x step %d: Range prefix[%d] = (%d, %d), want (%d, %d)", seed, step, calls, key, value, wantKey, wantValue)
		}
		calls++
		return calls < limit
	})
	if calls != limit {
		t.Fatalf("seed %#x step %d: Range prefix yielded %d entries, want %d", seed, step, calls, limit)
	}
}

func checkMapMatchesModel(t *testing.T, got *Map[int, int], want *mapModel, seed int64, step int, operation string, key int) {
	t.Helper()
	if got.Len() != len(want.order) {
		t.Fatalf("seed %#x step %d after %s(%d): Len = %d, want %d; model order %v", seed, step, operation, key, got.Len(), len(want.order), want.order)
	}

	position := 0
	for gotKey, gotValue := range got.Range() {
		if position >= len(want.order) {
			t.Fatalf("seed %#x step %d after %s(%d): Range yielded extra entry (%d, %d) at %d; model order %v", seed, step, operation, key, gotKey, gotValue, position, want.order)
		}
		wantKey := want.order[position]
		wantValue := want.values[wantKey]
		if gotKey != wantKey || gotValue != wantValue {
			t.Fatalf("seed %#x step %d after %s(%d): Range[%d] = (%d, %d), want (%d, %d); model order %v", seed, step, operation, key, position, gotKey, gotValue, wantKey, wantValue, want.order)
		}
		position++
	}
	if position != len(want.order) {
		t.Fatalf("seed %#x step %d after %s(%d): Range yielded %d entries, want %d; model order %v", seed, step, operation, key, position, len(want.order), want.order)
	}

	gotKey, gotValue, gotOK := got.Front()
	wantKey, wantValue, wantOK := want.front()
	if gotKey != wantKey || gotValue != wantValue || gotOK != wantOK {
		t.Fatalf("seed %#x step %d after %s(%d): Front = (%d, %d, %v), want (%d, %d, %v); model order %v", seed, step, operation, key, gotKey, gotValue, gotOK, wantKey, wantValue, wantOK, want.order)
	}

	gotKey, gotValue, gotOK = got.Back()
	wantKey, wantValue, wantOK = want.back()
	if gotKey != wantKey || gotValue != wantValue || gotOK != wantOK {
		t.Fatalf("seed %#x step %d after %s(%d): Back = (%d, %d, %v), want (%d, %d, %v); model order %v", seed, step, operation, key, gotKey, gotValue, gotOK, wantKey, wantValue, wantOK, want.order)
	}

	for candidate := testMapKeyMin - 1; candidate <= testMapKeyMax+1; candidate++ {
		gotValue, gotOK = got.Get(candidate)
		wantValue, wantOK = want.get(candidate)
		if gotValue != wantValue || gotOK != wantOK {
			t.Fatalf("seed %#x step %d after %s(%d): Get(%d) = (%d, %v), want (%d, %v); model order %v", seed, step, operation, key, candidate, gotValue, gotOK, wantValue, wantOK, want.order)
		}
		if got.Has(candidate) != wantOK {
			t.Fatalf("seed %#x step %d after %s(%d): Has(%d) = %v, want %v; model order %v", seed, step, operation, key, candidate, got.Has(candidate), wantOK, want.order)
		}

		gotKey, gotValue, gotOK = got.Next(candidate)
		wantKey, wantValue, wantOK = want.next(candidate)
		if gotKey != wantKey || gotValue != wantValue || gotOK != wantOK {
			t.Fatalf("seed %#x step %d after %s(%d): Next(%d) = (%d, %d, %v), want (%d, %d, %v); model order %v", seed, step, operation, key, candidate, gotKey, gotValue, gotOK, wantKey, wantValue, wantOK, want.order)
		}

		gotKey, gotValue, gotOK = got.Prev(candidate)
		wantKey, wantValue, wantOK = want.prev(candidate)
		if gotKey != wantKey || gotValue != wantValue || gotOK != wantOK {
			t.Fatalf("seed %#x step %d after %s(%d): Prev(%d) = (%d, %d, %v), want (%d, %d, %v); model order %v", seed, step, operation, key, candidate, gotKey, gotValue, gotOK, wantKey, wantValue, wantOK, want.order)
		}
	}
}
