// Package hypermap provides compact, generic insertion-ordered maps for
// allocation-conscious Go services.
package hypermap

import (
	"encoding/binary"
	"iter"
	"slices"
	"strings"
)

const queryEscapeHex = "0123456789ABCDEF"

type orderedMap[T comparable, T2 any] struct {
	index      hashIndex[T, T2]
	entries    []entries[T, T2]
	free       uint32
	unindexed  uint32
	fragmented bool
}

type entries[T comparable, T2 any] struct {
	key        T
	value      T2
	prev, next uint32
}

// Map is an insertion-ordered hash map.
//
// The zero value is ready to use. Single-key lookup, insertion, deletion,
// movement, and front/back access are O(1) average; deletion leaves no
// tombstones, so churn never rebuilds or grows the index. Use [New] or
// [Map.Init] with the maximum live entry count: while at most that many entries
// are live, storage never grows.
// Map values must not be copied after initialization or mutation.
// A Map may be transferred by value only when the source is no longer used.
// Map does not synchronize access; callers that share a [Map] across goroutines
// need external synchronization.
// Use independent [Map] values per shard or worker when write contention is
// expected.
type Map[T comparable, T2 any] orderedMap[T, T2]

// New returns an initialized [Map] with storage reserved for up to capacity
// live entries.
// New panics if capacity is negative or exceeds the supported maximum.
func New[T comparable, T2 any](capacity int) Map[T, T2] {
	var m Map[T, T2]
	m.Init(capacity)
	return m
}

// Init discards m's contents and backing storage, then reserves space for up to
// capacity live entries.
// Init panics if capacity is negative or exceeds the supported maximum.
func (m *Map[T, T2]) Init(capacity int) {
	if capacity < 0 {
		panic("hypermap: negative ordered map capacity")
	}
	if uint64(capacity) > indexMaxCapacity || capacity == int(^uint(0)>>1) {
		panic("hypermap: ordered map capacity too large")
	}
	var zero Map[T, T2]
	*m = zero
	if capacity == 0 {
		return
	}
	m.index = makeHashIndex[T, T2](capacity)
	// Arena slot 0 is a permanent sentinel anchoring a circular doubly linked
	// list: entries[0].next is the head, entries[0].prev the tail, and an
	// empty list points the sentinel at itself. The index stores arena
	// references rather than pointers, so entries may relocate without
	// repair. Deleted slots form a free chain through next, carry
	// indexFreeRef in prev, and are reused by later inserts. While
	// fragmented is false, slots 1 through len-1 are live and in list order.
	m.entries = make([]entries[T, T2], 1, capacity+1)
}

// Len reports the number of entries in m.
func (m *Map[T, T2]) Len() int {
	return int(m.index.used) + int(m.unindexed)
}

// Cap reports the arena's reserved live-entry capacity.
func (m *Map[T, T2]) Cap() int {
	c := cap(m.entries)
	if c == 0 {
		return 0
	}
	return c - 1
}

// Get returns the value for key and reports whether key is present.
func (m *Map[T, T2]) Get(key T) (value T2, ok bool) {
	if entry := m.find(key); entry != nil {
		return entry.value, true
	}
	return value, false
}

// Has reports whether key is present in m.
func (m *Map[T, T2]) Has(key T) bool {
	return m.find(key) != nil
}

// Set stores value for key, returns the previous value, and reports whether key
// was already present.
// It panics if inserting key would exceed the supported maximum size.
func (m *Map[T, T2]) Set(key T, value T2) (T2, bool) {
	index := &m.index
	if len(index.groups) == 0 {
		*index = makeHashIndex[T, T2](1)
		m.entries = make([]entries[T, T2], 1, 2)
	}
	var hash uint64
	switch k := any(key).(type) {
	case int:
		hash = hashInt(uint64(k), index.mixSeed)
	case string:
		hash = hashString(k, index.mixSeed)
	default:
		hash = index.hash(key)
	}
	groups := index.groups
	mask := len(groups) - 1
	groupIndex := int(hash>>7) & mask
	tag := hash&0x7f | indexFullControl
	home := &groups[groupIndex]
	var homeEmpty uint64
	for step := 1; ; step++ {
		group := &groups[groupIndex]
		control := binary.LittleEndian.Uint64(group.control[:])
		for matches := indexMatchTag(control, tag); matches != 0; matches &= matches - 1 {
			if entry := &m.entries[group.refs[indexFirstMatch(matches)]]; entry.key == key {
				old := entry.value
				entry.value = value
				return old, true
			}
		}
		if step == 1 {
			homeEmpty = indexMatchEmpty(control)
		}
		if control>>indexOverflowShift == 0 || step == len(groups) {
			break
		}
		groupIndex = (groupIndex + step) & mask
	}
	// A non-reflexive key such as NaN can never be found again, so it stays
	// out of the index; deleting it later needs no index search.
	indexed := key == key
	if indexed && index.used >= index.maxUsed {
		index.grow(m.entries, m.unindexed != 0)
		homeEmpty = 0
	}
	// Reuse a freed arena slot, or append within capacity.
	ref := m.free
	if ref != 0 {
		entry := &m.entries[ref]
		m.free = entry.next
		entry.key = key
		entry.value = value
	} else {
		if len(m.entries) == cap(m.entries) {
			m.growEntries()
		}
		n := len(m.entries)
		m.entries = m.entries[:n+1]
		m.entries[n] = entries[T, T2]{key: key, value: value}
		ref = uint32(n)
	}
	switch {
	case !indexed:
		m.unindexed++
	case homeEmpty != 0:
		// Most keys land in their home group, whose control word the lookup
		// just loaded; storing there directly skips a second probe.
		slot := indexFirstMatch(homeEmpty)
		control := binary.LittleEndian.Uint64(home.control[:])
		binary.LittleEndian.PutUint64(home.control[:], control|tag<<(slot*8))
		home.refs[slot] = ref
		index.used++
	default:
		index.insert(hash, ref)
	}
	m.linkBack(ref)
	var zero T2
	return zero, false
}

// Replace stores value for key, returns the previous value, and reports whether
// key was present. It leaves m unchanged when key is absent.
func (m *Map[T, T2]) Replace(key T, value T2) (old T2, replaced bool) {
	// Replace repeats find's probe so that, like Get, it costs one call.
	index := &m.index
	if len(index.groups) == 0 {
		return old, false
	}
	var hash uint64
	switch k := any(key).(type) {
	case int:
		hash = hashInt(uint64(k), index.mixSeed)
	case string:
		hash = hashString(k, index.mixSeed)
	default:
		hash = index.hash(key)
	}
	groups := index.groups
	mask := len(groups) - 1
	groupIndex := int(hash>>7) & mask
	tag := hash&0x7f | indexFullControl
	for step := 1; ; step++ {
		group := &groups[groupIndex]
		control := binary.LittleEndian.Uint64(group.control[:])
		for matches := indexMatchTag(control, tag); matches != 0; matches &= matches - 1 {
			if entry := &m.entries[group.refs[indexFirstMatch(matches)]]; entry.key == key {
				old, entry.value = entry.value, value
				return old, true
			}
		}
		if control>>indexOverflowShift == 0 || step == len(groups) {
			return old, false
		}
		groupIndex = (groupIndex + step) & mask
	}
}

// Delete removes key, returns the removed value, and reports whether key was
// present.
func (m *Map[T, T2]) Delete(key T) (T2, bool) {
	index := &m.index
	if len(index.groups) != 0 {
		var hash uint64
		switch k := any(key).(type) {
		case int:
			hash = hashInt(uint64(k), index.mixSeed)
		case string:
			hash = hashString(k, index.mixSeed)
		default:
			hash = index.hash(key)
		}
		groups := index.groups
		mask := len(groups) - 1
		groupIndex := int(hash>>7) & mask
		tag := hash&0x7f | indexFullControl
		for step := 1; ; step++ {
			group := &groups[groupIndex]
			control := binary.LittleEndian.Uint64(group.control[:])
			for matches := indexMatchTag(control, tag); matches != 0; matches &= matches - 1 {
				slot := indexFirstMatch(matches)
				if ref := group.refs[slot]; m.entries[ref].key == key {
					index.remove(hash, step, group, slot)
					return m.deleteEntry(ref), true
				}
			}
			if control>>indexOverflowShift == 0 || step == len(groups) {
				break
			}
			groupIndex = (groupIndex + step) & mask
		}
	}
	var zero T2
	return zero, false
}

// Clear removes all entries while keeping allocated storage for reuse.
func (m *Map[T, T2]) Clear() {
	m.index.clear(m.entries, m.unindexed != 0)
	if len(m.entries) != 0 {
		clear(m.entries)
		m.entries = m.entries[:1]
	}
	m.free = 0
	m.unindexed = 0
	m.fragmented = false
}

// Reset removes all entries and releases m's backing storage.
func (m *Map[T, T2]) Reset() {
	var zero Map[T, T2]
	*m = zero
}

// Front returns the first key and value in insertion order and reports whether
// m is non-empty.
func (m *Map[T, T2]) Front() (T, T2, bool) {
	if len(m.entries) != 0 {
		if ref := m.entries[0].next; ref != 0 {
			entry := &m.entries[ref]
			return entry.key, entry.value, true
		}
	}
	var (
		zeroKey   T
		zeroValue T2
	)
	return zeroKey, zeroValue, false
}

// Back returns the last key and value in insertion order and reports whether m
// is non-empty.
func (m *Map[T, T2]) Back() (T, T2, bool) {
	if len(m.entries) != 0 {
		if ref := m.entries[0].prev; ref != 0 {
			entry := &m.entries[ref]
			return entry.key, entry.value, true
		}
	}
	var (
		zeroKey   T
		zeroValue T2
	)
	return zeroKey, zeroValue, false
}

// Next returns the key and value after key in insertion order and reports
// whether key has a successor.
func (m *Map[T, T2]) Next(key T) (T, T2, bool) {
	if entry := m.find(key); entry != nil {
		if next := entry.next; next != 0 {
			entry := &m.entries[next]
			return entry.key, entry.value, true
		}
	}
	var (
		zeroKey   T
		zeroValue T2
	)
	return zeroKey, zeroValue, false
}

// Prev returns the key and value before key in insertion order and reports
// whether key has a predecessor.
func (m *Map[T, T2]) Prev(key T) (T, T2, bool) {
	if entry := m.find(key); entry != nil {
		if prev := entry.prev; prev != 0 {
			entry := &m.entries[prev]
			return entry.key, entry.value, true
		}
	}
	var (
		zeroKey   T
		zeroValue T2
	)
	return zeroKey, zeroValue, false
}

// MoveToFront moves key to the front of m and reports whether key was present.
func (m *Map[T, T2]) MoveToFront(key T) bool {
	return m.move(key, true)
}

// MoveToBack moves key to the back of m and reports whether key was present.
func (m *Map[T, T2]) MoveToBack(key T) bool {
	return m.move(key, false)
}

// PopFront removes and returns the first key and value in insertion order and
// reports whether an entry is removed.
func (m *Map[T, T2]) PopFront() (T, T2, bool) {
	if len(m.entries) != 0 {
		if ref := m.entries[0].next; ref != 0 {
			key := m.entries[ref].key
			m.unindex(key, ref)
			return key, m.deleteEntry(ref), true
		}
	}
	var (
		zeroKey   T
		zeroValue T2
	)
	return zeroKey, zeroValue, false
}

// PopBack removes and returns the last key and value in insertion order and
// reports whether an entry is removed.
func (m *Map[T, T2]) PopBack() (T, T2, bool) {
	if len(m.entries) != 0 {
		if ref := m.entries[0].prev; ref != 0 {
			key := m.entries[ref].key
			m.unindex(key, ref)
			return key, m.deleteEntry(ref), true
		}
	}
	var (
		zeroKey   T
		zeroValue T2
	)
	return zeroKey, zeroValue, false
}

// Range returns an [iter.Seq2] over each key and value in insertion order.
// Range leaves remaining iteration behavior unspecified when yield mutates m.
func (m *Map[T, T2]) Range() iter.Seq2[T, T2] {
	return m.RangeFunc
}

// RangeFunc calls yield for each key and value in insertion order until yield
// returns false. RangeFunc leaves remaining iteration behavior unspecified when
// yield mutates m.
func (m *Map[T, T2]) RangeFunc(yield func(T, T2) bool) {
	// A single yield call site keeps RangeFunc and the Range method value
	// inlinable, so callback and range loops compile to a direct loop. A
	// dense arena advances by index, which avoids the dependent load of each
	// successor link; the successor is read before yield runs, so yield may
	// delete the current entry.
	e := m.entries
	ref := 1
	if m.fragmented {
		ref = int(e[0].next)
	}
	for ref > 0 && ref < len(e) {
		entry := &e[ref]
		ref++
		if m.fragmented {
			ref = int(entry.next)
		}
		if !yield(entry.key, entry.value) {
			return
		}
	}
}

// QueryMap is a [Map] specialized for URL query parameters. It maps string keys
// to repeated string values and adds [QueryMap.Encode]. Every [Map] method is
// promoted for ordered string keys and []string values.
type QueryMap struct {
	Map[string, []string]
}

// NewQueryMap returns an initialized [QueryMap] with storage reserved for up to
// capacity live entries.
// NewQueryMap panics if capacity is negative or exceeds the supported maximum.
func NewQueryMap(capacity int) QueryMap {
	return QueryMap{New[string, []string](capacity)}
}

// Encode returns qm encoded as URL query parameters in key insertion order.
// Values for each key are encoded in slice order. A nil receiver or empty
// [QueryMap] encodes to an empty string.
func (qm *QueryMap) Encode() string {
	if qm == nil || qm.Len() == 0 {
		return ""
	}
	var (
		e    = qm.entries
		size int
	)
	for ref := e[0].next; ref != 0; {
		var (
			entry  = &e[ref]
			next   = entry.next
			values = entry.value
		)
		if len(values) == 0 {
			ref = next
			continue
		}
		keySize := queryEscapedLen(entry.key)
		for _, value := range values {
			if size > 0 {
				size++
			}
			size += keySize + 1 + queryEscapedLen(value)
		}
		ref = next
	}
	if size == 0 {
		return ""
	}
	var sb strings.Builder
	sb.Grow(size)
	for ref := e[0].next; ref != 0; {
		var (
			entry = &e[ref]
			next  = entry.next
		)
		for _, value := range entry.value {
			if sb.Len() > 0 {
				sb.WriteByte('&')
			}
			appendQueryEscaped(&sb, entry.key)
			sb.WriteByte('=')
			appendQueryEscaped(&sb, value)
		}
		ref = next
	}
	return sb.String()
}

var queryEscape = func() (t [256]bool) {
	// queryEscape[c] reports whether byte c must be percent-escaped in a
	// query component. Precomputing it turns the per-byte classification in
	// the encode hot loops into a single branch-free load. Indexing a
	// [256]bool with a byte needs no bounds check.
	for c := range len(t) {
		t[c] = shouldEscapeQuery(byte(c))
	}
	return t
}()

func queryEscapedLen(s string) int {
	n := len(s)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != ' ' && queryEscape[c] {
			n += 2
		}
	}
	return n
}

func appendQueryEscaped(sb *strings.Builder, s string) {
	var start int
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !queryEscape[c] {
			continue
		}
		if start < i {
			sb.WriteString(s[start:i])
		}
		if c == ' ' {
			sb.WriteByte('+')
		} else {
			sb.WriteByte('%')
			sb.WriteByte(queryEscapeHex[c>>4])
			sb.WriteByte(queryEscapeHex[c&15])
		}
		start = i + 1
	}
	if start < len(s) {
		sb.WriteString(s[start:])
	}
}

func shouldEscapeQuery(c byte) bool {
	switch {
	case 'a' <= c && c <= 'z':
		return false
	case 'A' <= c && c <= 'Z':
		return false
	case '0' <= c && c <= '9':
		return false
	}
	switch c {
	case '-', '_', '.', '~':
		return false
	}
	return true
}

func (m *Map[T, T2]) growEntries() {
	// Doubling matches the index's growth and halves the copying of append's
	// gentler large-slice growth. Capacity stays at most indexFreeRef, so
	// every reference fits in uint32 and differs from the free-slot marker.
	n := uint64(len(m.entries))
	limit := uint64(indexFreeRef)
	if n >= limit {
		panic("hypermap: ordered map capacity too large")
	}
	m.entries = slices.Grow(m.entries, int(min(n, limit-n)))
	if uint64(cap(m.entries)) > limit {
		m.entries = m.entries[:n:int(limit)]
	}
}

func (m *Map[T, T2]) move(key T, front bool) bool {
	// move repeats find's probe so that, like Get, each exported move costs
	// one call.
	index := &m.index
	if len(index.groups) == 0 {
		return false
	}
	var hash uint64
	switch k := any(key).(type) {
	case int:
		hash = hashInt(uint64(k), index.mixSeed)
	case string:
		hash = hashString(k, index.mixSeed)
	default:
		hash = index.hash(key)
	}
	groups := index.groups
	mask := len(groups) - 1
	groupIndex := int(hash>>7) & mask
	tag := hash&0x7f | indexFullControl
	for step := 1; ; step++ {
		group := &groups[groupIndex]
		control := binary.LittleEndian.Uint64(group.control[:])
		for matches := indexMatchTag(control, tag); matches != 0; matches &= matches - 1 {
			ref := group.refs[indexFirstMatch(matches)]
			e := m.entries
			entry := &e[ref]
			if entry.key != key {
				continue
			}
			prev, next := entry.prev, entry.next
			if front {
				if prev == 0 {
					return true
				}
				e[prev].next = next
				e[next].prev = prev
				head := e[0].next
				entry.prev = 0
				entry.next = head
				e[head].prev = ref
				e[0].next = ref
			} else {
				if next == 0 {
					return true
				}
				e[prev].next = next
				e[next].prev = prev
				tail := e[0].prev
				entry.next = 0
				entry.prev = tail
				e[tail].next = ref
				e[0].prev = ref
			}
			m.fragmented = true
			return true
		}
		if control>>indexOverflowShift == 0 || step == len(groups) {
			return false
		}
		groupIndex = (groupIndex + step) & mask
	}
}

func (m *Map[T, T2]) linkBack(ref uint32) {
	e := m.entries
	tail := e[0].prev
	entry := &e[ref]
	entry.prev = tail
	entry.next = 0
	e[tail].next = ref
	e[0].prev = ref
}

func (m *Map[T, T2]) unindex(key T, ref uint32) {
	if m.unindexed != 0 && key != key {
		m.unindexed--
		return
	}
	m.index.removeRef(key, ref)
}

func (m *Map[T, T2]) deleteEntry(ref uint32) T2 {
	// The caller has already removed ref from the index. The circular
	// sentinel makes head and tail unlinking the same two writes.
	e := m.entries
	entry := &e[ref]
	value := entry.value
	e[entry.prev].next = entry.next
	e[entry.next].prev = entry.prev
	if !m.fragmented && int(ref) == len(e)-1 {
		// Dropping the last slot of a dense arena keeps it dense.
		*entry = entries[T, T2]{}
		m.entries = e[:ref]
		return value
	}
	*entry = entries[T, T2]{prev: indexFreeRef, next: m.free}
	m.free = ref
	m.fragmented = true
	return value
}
