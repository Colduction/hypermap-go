// Package hypermap provides compact, generic insertion-ordered maps for
// allocation-conscious Go services.
package hypermap

import (
	"hash/maphash"
	"iter"
	"strings"
)

const queryEscapeHex = "0123456789ABCDEF"

// orderedMap stores its entries in a slice whose first element (index 0) is a
// permanent sentinel. The sentinel anchors a circular doubly-linked list, so
// entries[0].next is the head, entries[0].prev is the tail, and an empty list
// is the sentinel pointing at itself. The hash index stores stable slice
// indices rather than pointers, so entries may relocate without repair.
// Deleted slots are recycled through a singly-linked free chain rooted at free.
type orderedMap[T comparable, T2 any] struct {
	index      hashIndex[T, T2]
	entries    []entries[T, T2]
	free       uint32
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
// movement, and front/back access are O(1) average. Use [New] or [Map.Init]
// with the maximum live entry count to avoid storage growth during the initial
// fill. Heavy churn that replaces deleted keys with different keys may grow
// hash-index storage to preserve amortized write cost.
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
	m.entries = make([]entries[T, T2], 1, capacity+1)
}

// Len reports the number of entries in m.
func (m *Map[T, T2]) Len() int {
	return int(m.index.used)
}

// Cap reports the arena's reserved live-entry capacity. The hash index may grow
// independently during different-key churn.
func (m *Map[T, T2]) Cap() int {
	c := cap(m.entries)
	if c == 0 {
		return 0
	}
	return c - 1
}

// Get returns the value for key and reports whether key is present.
func (m *Map[T, T2]) Get(key T) (T2, bool) {
	if len(m.index.groups) != 0 {
		hash := maphash.Comparable(m.index.seed, key)
		mask := len(m.index.groups) - 1
		groupIndex := int(hash>>7) & mask
		tag := hash & 0x7f
		var step int
		for {
			group := &m.index.groups[groupIndex]
			matches := indexMatchTag(group.control, tag)
			for matches != 0 {
				slot := indexFirstMatch(matches)
				entry := &m.entries[group.refs[slot]]
				if entry.key == key {
					return entry.value, true
				}
				matches &= matches - 1
			}
			if indexMatchEmpty(group.control) != 0 {
				break
			}
			step++
			groupIndex = (groupIndex + step) & mask
		}
	}
	var zero T2
	return zero, false
}

// Has reports whether key is present in m.
func (m *Map[T, T2]) Has(key T) bool {
	return m.index.entry(m.entries, key) != nil
}

// Set stores value for key, returns the previous value, and reports whether key
// was already present.
// It panics if inserting key would exceed the supported maximum size.
func (m *Map[T, T2]) Set(key T, value T2) (T2, bool) {
	if len(m.index.groups) == 0 {
		m.index = makeHashIndex[T, T2](1)
	}
	if len(m.entries) == 0 {
		m.entries = make([]entries[T, T2], 1, 2) // sentinel
	}
	for {
		ref, slot, tag := m.index.findInsert(m.entries, key)
		if ref != 0 {
			entry := &m.entries[ref]
			old := entry.value
			entry.value = value
			return old, true
		}
		groupIndex, groupSlot := indexSplitSlot(slot)
		control := indexControlAt(m.index.groups[groupIndex].control, groupSlot)
		if control == indexEmptyControl && uint64(m.index.filled) >= m.index.maxLoad() {
			m.index.rebuild(m.entries, m.index.growForInsert())
			continue
		}
		ref = m.addSlot(key, value)
		m.index.insert(slot, tag, ref)
		m.linkBack(ref)
		var zero T2
		return zero, false
	}
}

// Replace stores value for key, returns the previous value, and reports whether
// key was present. It leaves m unchanged when key is absent.
func (m *Map[T, T2]) Replace(key T, value T2) (old T2, replaced bool) {
	if len(m.index.groups) != 0 {
		hash := maphash.Comparable(m.index.seed, key)
		mask := len(m.index.groups) - 1
		groupIndex := int(hash>>7) & mask
		tag := hash & 0x7f
		var step int
		for {
			group := &m.index.groups[groupIndex]
			matches := indexMatchTag(group.control, tag)
			for matches != 0 {
				slot := indexFirstMatch(matches)
				entry := &m.entries[group.refs[slot]]
				if entry.key == key {
					old = entry.value
					entry.value = value
					return old, true
				}
				matches &= matches - 1
			}
			if indexMatchEmpty(group.control) != 0 {
				break
			}
			step++
			groupIndex = (groupIndex + step) & mask
		}
	}
	return old, false
}

// Delete removes key, returns the removed value, and reports whether key was
// present.
func (m *Map[T, T2]) Delete(key T) (T2, bool) {
	ref, slot, ok := m.index.lookup(m.entries, key)
	if !ok {
		var zero T2
		return zero, false
	}
	entry := &m.entries[ref]
	old := entry.value
	m.index.deleteSlot(slot)
	m.deleteEntry(ref, entry)
	return old, true
}

// Clear removes all entries while keeping allocated storage for reuse.
func (m *Map[T, T2]) Clear() {
	m.index.clear()
	if len(m.entries) != 0 {
		clear(m.entries)
		m.entries = m.entries[:1]
	}
	m.free = 0
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
	if len(m.entries) == 0 {
		var (
			zeroKey   T
			zeroValue T2
		)
		return zeroKey, zeroValue, false
	}
	ref := m.entries[0].next
	if ref == 0 {
		var (
			zeroKey   T
			zeroValue T2
		)
		return zeroKey, zeroValue, false
	}
	entry := &m.entries[ref]
	return entry.key, entry.value, true
}

// Back returns the last key and value in insertion order and reports whether m
// is non-empty.
func (m *Map[T, T2]) Back() (T, T2, bool) {
	if len(m.entries) == 0 {
		var (
			zeroKey   T
			zeroValue T2
		)
		return zeroKey, zeroValue, false
	}
	ref := m.entries[0].prev
	if ref == 0 {
		var (
			zeroKey   T
			zeroValue T2
		)
		return zeroKey, zeroValue, false
	}
	entry := &m.entries[ref]
	return entry.key, entry.value, true
}

// Next returns the key and value after key in insertion order and reports
// whether key has a successor.
func (m *Map[T, T2]) Next(key T) (T, T2, bool) {
	current := m.index.entry(m.entries, key)
	if current == nil {
		var (
			zeroKey   T
			zeroValue T2
		)
		return zeroKey, zeroValue, false
	}
	next := current.next
	if next == 0 {
		var (
			zeroKey   T
			zeroValue T2
		)
		return zeroKey, zeroValue, false
	}
	entry := &m.entries[next]
	return entry.key, entry.value, true
}

// Prev returns the key and value before key in insertion order and reports
// whether key has a predecessor.
func (m *Map[T, T2]) Prev(key T) (T, T2, bool) {
	current := m.index.entry(m.entries, key)
	if current == nil {
		var (
			zeroKey   T
			zeroValue T2
		)
		return zeroKey, zeroValue, false
	}
	prev := current.prev
	if prev == 0 {
		var (
			zeroKey   T
			zeroValue T2
		)
		return zeroKey, zeroValue, false
	}
	entry := &m.entries[prev]
	return entry.key, entry.value, true
}

// MoveToFront moves key to the front of m and reports whether key was present.
func (m *Map[T, T2]) MoveToFront(key T) bool {
	ref := m.index.get(m.entries, key)
	if ref == 0 {
		return false
	}
	var (
		e     = m.entries
		head  = e[0].next
		entry = &e[ref]
	)
	if head == ref {
		return true
	}
	m.fragmented = true
	prev, next := entry.prev, entry.next
	e[prev].next = next
	e[next].prev = prev
	entry.prev = 0
	entry.next = head
	e[head].prev = ref
	e[0].next = ref
	return true
}

// MoveToBack moves key to the back of m and reports whether key was present.
func (m *Map[T, T2]) MoveToBack(key T) bool {
	ref := m.index.get(m.entries, key)
	if ref == 0 {
		return false
	}
	var (
		e     = m.entries
		tail  = e[0].prev
		entry = &e[ref]
	)
	if tail == ref {
		return true
	}
	m.fragmented = true
	prev, next := entry.prev, entry.next
	e[prev].next = next
	e[next].prev = prev
	entry.next = 0
	entry.prev = tail
	e[tail].next = ref
	e[0].prev = ref
	return true
}

// PopFront removes and returns the first key and value in insertion order and
// reports whether an entry is removed.
func (m *Map[T, T2]) PopFront() (T, T2, bool) {
	if len(m.entries) == 0 {
		var (
			zeroKey   T
			zeroValue T2
		)
		return zeroKey, zeroValue, false
	}
	ref := m.entries[0].next
	if ref == 0 {
		var (
			zeroKey   T
			zeroValue T2
		)
		return zeroKey, zeroValue, false
	}
	var (
		entry      = &m.entries[ref]
		key, value = entry.key, entry.value
	)
	m.index.deleteRef(m.entries, key, ref)
	m.deleteEntry(ref, entry)
	return key, value, true
}

// PopBack removes and returns the last key and value in insertion order and
// reports whether an entry is removed.
func (m *Map[T, T2]) PopBack() (T, T2, bool) {
	if len(m.entries) == 0 {
		var (
			zeroKey   T
			zeroValue T2
		)
		return zeroKey, zeroValue, false
	}
	ref := m.entries[0].prev
	if ref == 0 {
		var (
			zeroKey   T
			zeroValue T2
		)
		return zeroKey, zeroValue, false
	}
	var (
		entry      = &m.entries[ref]
		key, value = entry.key, entry.value
	)
	m.index.deleteRef(m.entries, key, ref)
	m.deleteEntry(ref, entry)
	return key, value, true
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
	e := m.entries
	if len(e) == 0 {
		return
	}
	if !m.fragmented {
		for ref := 1; ref < len(e); ref++ {
			entry := &e[ref]
			if !yield(entry.key, entry.value) {
				return
			}
		}
		return
	}
	for ref := e[0].next; ref != 0; {
		var (
			entry = &e[ref]
			next  = entry.next
		)
		if !yield(entry.key, entry.value) {
			return
		}
		ref = next
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

// queryEscape[c] reports whether byte c must be percent-escaped in a query
// component. Precomputing it turns the per-byte classification in the encode
// hot loops into a single branch-free load. Indexing a [256]bool with a byte
// needs no bounds check.
var queryEscape = func() (t [256]bool) {
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

// addSlot reserves a slot for key/value and returns its slice index, reusing a
// freed slot when one is available. The caller links the returned slot into the
// list.
func (m *Map[T, T2]) addSlot(key T, value T2) uint32 {
	if m.free != 0 {
		var (
			ref   = m.free
			entry = &m.entries[ref]
		)
		m.free = entry.next
		entry.key = key
		entry.value = value
		entry.prev = 0
		entry.next = 0
		return ref
	}
	if uint64(len(m.entries)) > uint64(^uint32(0)) {
		panic("hypermap: ordered map capacity too large")
	}
	ref := uint32(len(m.entries))
	m.entries = append(m.entries, entries[T, T2]{key: key, value: value})
	return ref
}

// linkBack appends ref to the tail of the circular list.
func (m *Map[T, T2]) linkBack(ref uint32) {
	var (
		tail  = m.entries[0].prev
		entry = &m.entries[ref]
	)
	entry.prev = tail
	entry.next = 0
	m.entries[tail].next = ref
	m.entries[0].prev = ref
}

// unlink removes entry from the list. The circular sentinel makes both the
// head and tail cases fall through to the same two writes, so no branching is
// needed.
func (m *Map[T, T2]) unlink(entry *entries[T, T2]) {
	m.entries[entry.prev].next = entry.next
	m.entries[entry.next].prev = entry.prev
}

// deleteEntry removes entry from the list and pushes its slot onto the free
// chain for reuse by a later insert. Its index slot must already be removed.
func (m *Map[T, T2]) deleteEntry(ref uint32, entry *entries[T, T2]) {
	m.unlink(entry)
	m.fragmented = true
	var (
		zeroKey   T
		zeroValue T2
	)
	entry.key = zeroKey
	entry.value = zeroValue
	entry.prev = 0
	entry.next = m.free
	m.free = ref
}
