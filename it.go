package it

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"sync"
	"unicode"
	"unicode/utf8"
)

// Iter provides buffered iteration over any iter.Seq[T] with:
// - Peek ahead without consumption
// - Relative (Seek) and absolute (SeekTo) navigation
// - Rewind capabilities
// - Configurable buffer limits
type Iter[T any] struct {
	src       iter.Seq[T]  // Source sequence
	buf       []T          // Linear buffer for storage
	pos       int          // Current position (-1 = before start)
	len       int          // Number of valid elements in buffer
	maxCap    int          // Maximum buffer capacity (0 = unlimited)
	exhausted bool         // Flag to indicate if src is exhausted
	ch        chan item[T] // Channel for pulling elements
	once      sync.Once    // Ensures src is started only once
}

type item[T any] struct {
	value T
	ok    bool
}

// Pair represents a key-value pair for map entry iterators.
type Pair[K, V any] struct {
	Key   K
	Value V
}

// C provides a builder for advanced iterator configuration.
type C struct {
	initialCap    int
	maxCap        int
	bufferSize    int
	maxLineLength int
	ctx           context.Context
}

// Config starts a configuration builder for customized iterator creation.
// Defaults: InitialCap=64, MaxCap=0 (unlimited), BufferSize=4KB, MaxLineLength=64KB.
func Config() *C {
	return &C{
		initialCap:    64,
		maxCap:        0,
		bufferSize:    4096,
		maxLineLength: 64 * 1024,
		ctx:           context.Background(),
	}
}

// InitialCap sets the initial buffer capacity (default 64).
func (c *C) InitialCap(n int) *C {
	if n > 0 {
		c.initialCap = n
	}
	return c
}

// MaxCap sets the maximum buffer capacity (default 0, unlimited).
func (c *C) MaxCap(n int) *C {
	if n >= 0 {
		c.maxCap = n
	}
	return c
}

// BufferSize sets the buffer size for Bytes (default 4KB).
func (c *C) BufferSize(n int) *C {
	if n > 0 {
		c.bufferSize = n
	}
	return c
}

// MaxLineLength sets the maximum line length for Lines (default 64KB).
func (c *C) MaxLineLength(n int) *C {
	if n > 0 {
		c.maxLineLength = n
	}
	return c
}

// WithContext sets the context for operations (default context.Background()).
func (c *C) WithContext(ctx context.Context) *C {
	if ctx != nil {
		c.ctx = ctx
	}
	return c
}

// New creates an Iter with configurable buffer behavior.
// initialCap defaults to 64 if <= 0; maxCap of 0 means unlimited.
// Returns an error if the source sequence is nil.
func New[T any](src iter.Seq[T], initialCap, maxCap int) (*Iter[T], error) {
	if src == nil {
		return nil, errors.New("nil source sequence")
	}
	if initialCap <= 0 {
		initialCap = 64
	}
	it := &Iter[T]{
		src:       src,
		buf:       make([]T, 0, initialCap),
		pos:       -1,
		len:       0,
		maxCap:    maxCap,
		exhausted: false,
		ch:        make(chan item[T], 1),
	}
	return it, nil
}

// start begins pulling elements from src into the channel.
func (it *Iter[T]) start() {
	it.once.Do(func() {
		go func() {
			it.src(func(v T) bool {
				it.ch <- item[T]{value: v, ok: true}
				return true
			})
			it.ch <- item[T]{ok: false}
			close(it.ch)
		}()
	})
}

// Wrap creates an Iter with defaults (initialCap=64, maxCap=0).
func Wrap[T any](src iter.Seq[T]) (*Iter[T], error) {
	return New(src, 64, 0)
}

// Must creates an Iter, panicking on error.
func Must[T any](src iter.Seq[T]) *Iter[T] {
	it, err := Wrap(src)
	if err != nil {
		panic(fmt.Errorf("it: Must failed: %w", err))
	}
	return it
}

// Next advances to the next element, growing buffer if needed.
func (it *Iter[T]) Next() (T, bool) {
	// Check if there are more elements in the buffer
	if it.pos+1 < it.len {
		it.pos++
		return it.buf[it.pos], true
	}

	// If exhausted, return false
	if it.exhausted {
		var zero T
		return zero, false
	}

	// Start pulling elements if not already started
	it.start()

	// Fetch the next element from the channel
	item, ok := <-it.ch
	if !ok || !item.ok {
		it.exhausted = true
		var zero T
		return zero, false
	}

	v := item.value

	// Ensure buffer has space (grow if needed)
	it.growIfNeeded()
	if it.len < cap(it.buf) {
		it.buf = append(it.buf, v)
		it.len++
	} else if it.maxCap > 0 && it.len >= it.maxCap {
		// Shift buffer left if at max capacity
		it.buf = append(it.buf[:0], it.buf[1:]...)
		it.buf = append(it.buf, v)
		it.pos--
	} else {
		it.buf = append(it.buf, v)
		it.len++
	}

	it.pos++
	return v, true
}

// growIfNeeded handles buffer expansion with optimized growth strategy.
func (it *Iter[T]) growIfNeeded() {
	if it.len < cap(it.buf) {
		return
	}
	if it.maxCap > 0 && cap(it.buf) >= it.maxCap {
		return
	}

	newCap := cap(it.buf) * 2
	if newCap == 0 {
		newCap = 64
	}
	if it.maxCap > 0 && newCap > it.maxCap {
		newCap = it.maxCap
	}

	newBuf := make([]T, it.len, newCap)
	copy(newBuf, it.buf[:it.len])
	it.buf = newBuf
}

// Peek returns the next element without advancing.
func (it *Iter[T]) Peek() (T, bool) {
	if it.pos+1 < it.len {
		return it.buf[it.pos+1], true
	}
	if it.exhausted {
		var zero T
		return zero, false
	}
	// Temporarily fetch the next element without advancing pos
	v, ok := it.Next()
	if !ok {
		return v, false
	}
	it.pos--
	return v, true
}

// Seek moves n positions relative to current location, treating n as the number of elements to skip.
func (it *Iter[T]) Seek(n int) (T, bool) {
	if n < 0 {
		return it.seekToPosition(it.pos + n)
	}
	// Treat n as the number of elements to skip, so move n+1 positions forward
	return it.seekToPosition(it.pos + n + 1)
}

// SeekTo jumps to absolute position.
func (it *Iter[T]) SeekTo(pos int) (T, bool) {
	return it.seekToPosition(pos)
}

func (it *Iter[T]) seekToPosition(target int) (T, bool) {
	if target < -1 {
		target = -1
	}

	// Fetch elements until target is in range or src is exhausted
	for target >= it.len && !it.exhausted {
		_, ok := it.Next()
		if !ok {
			break
		}
	}

	// If target is before start or buffer is empty, return zero value
	if target < 0 || it.len == 0 {
		it.pos = target
		var zero T
		return zero, false
	}

	// Clamp target to the last valid position
	if target >= it.len {
		target = it.len - 1
	}

	it.pos = target
	return it.buf[it.pos], true
}

// Rewind resets to initial state while preserving buffer.
func (it *Iter[T]) Rewind() {
	it.pos = -1
}

// Current returns the element at current position.
func (it *Iter[T]) Current() (T, bool) {
	if it.pos >= 0 && it.pos < it.len {
		return it.buf[it.pos], true
	}
	var zero T
	return zero, false
}

// Buffer returns a copy of all buffered elements.
func (it *Iter[T]) Buffer() []T {
	result := make([]T, it.len)
	copy(result, it.buf[:it.len])
	return result
}

// Dump consumes the iterator and returns all elements as a slice.
// Similar to io.ReadAll, it includes buffered and remaining source elements.
func (it *Iter[T]) Dump(fs ...func(value T) T) ([]T, error) {
	result := make([]T, 0, it.len)
	// Copy buffered elements
	for i := 0; i < it.len; i++ {
		v := it.buf[i]
		if len(fs) > 0 {
			for _, f := range fs {
				v = f(v)
			}
		}
		result = append(result, v)
	}
	// Consume remaining elements
	for v, ok := it.Next(); ok; v, ok = it.Next() {
		if len(fs) > 0 {
			for _, f := range fs {
				v = f(v)
			}
		}
		result = append(result, v)
	}
	it.Rewind()
	return result, nil
}

// Position returns current index (-1 = before start).
func (it *Iter[T]) Position() int {
	return it.pos
}

// BufferedLen returns number of elements available for rewinding.
func (it *Iter[T]) BufferedLen() int {
	return it.len
}

// Cap returns current buffer capacity.
func (it *Iter[T]) Cap() int {
	return cap(it.buf)
}

// Seq returns the iterator as iter.Seq[T] for range loops.
func (it *Iter[T]) Seq() iter.Seq[T] {
	return func(yield func(T) bool) {
		for v, ok := it.Next(); ok; v, ok = it.Next() {
			if !yield(v) {
				return
			}
		}
	}
}

// Slice creates an Iter from []T.
// Preserves order and buffers all elements.
// Returns an error for nil slices.
func Slice[T any](s []T) (*Iter[T], error) {
	if s == nil {
		return nil, errors.New("nil slice")
	}
	return Wrap(func(yield func(T) bool) {
		for _, v := range s {
			if !yield(v) {
				return
			}
		}
	})
}

// Chan creates an Iter from a channel with configuration and context.
// Returns an error for nil channels or context cancellation.
func Chan[T any](c *C, ch <-chan T) (*Iter[T], error) {
	if ch == nil {
		return nil, errors.New("nil channel")
	}
	return New(func(yield func(T) bool) {
		for {
			select {
			case v, ok := <-ch:
				if !ok {
					return
				}
				if !yield(v) {
					return
				}
			case <-c.ctx.Done():
				return
			}
		}
	}, c.initialCap, c.maxCap)
}

// Keys creates an Iter from map keys.
// Order is random (Go map iteration order).
// Returns an error for nil maps.
func Keys[K comparable, V any](m map[K]V) (*Iter[K], error) {
	if m == nil {
		return nil, errors.New("nil map")
	}
	return Wrap(func(yield func(K) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	})
}

// Vals creates an Iter from map values.
// Order is random (Go map iteration order).
// Returns an error for nil maps.
func Vals[K comparable, V any](m map[K]V) (*Iter[V], error) {
	if m == nil {
		return nil, errors.New("nil map")
	}
	return Wrap(func(yield func(V) bool) {
		for _, v := range m {
			if !yield(v) {
				return
			}
		}
	})
}

// Pairs creates an Iter from map entries.
// Order is random (Go map iteration order).
// Returns an error for nil maps.
func Pairs[K comparable, V any](m map[K]V) (*Iter[Pair[K, V]], error) {
	if m == nil {
		return nil, errors.New("nil map")
	}
	return Wrap(func(yield func(Pair[K, V]) bool) {
		for k, v := range m {
			if !yield(Pair[K, V]{Key: k, Value: v}) {
				return
			}
		}
	})
}

// Tea creates an Iter yielding lines from multiple io.Reader sources sequentially.
// Uses configuration from Config() with 64KB max line length.
// Returns an error for nil readers or if any reader is nil.
func Tea(sources ...io.Reader) (*Iter[string], error) {
	if len(sources) == 0 {
		return New(func(yield func(string) bool) {}, 64, 0)
	}
	for _, r := range sources {
		if r == nil {
			return nil, errors.New("nil reader in sources")
		}
	}
	c := Config()
	return c.Lines(&readerChain{sources: sources})
}

// readerChain implements io.Reader by reading from a sequence of readers.
type readerChain struct {
	sources []io.Reader
	current int
}

func (rc *readerChain) Read(p []byte) (int, error) {
	for rc.current < len(rc.sources) {
		n, err := rc.sources[rc.current].Read(p)
		if err == io.EOF {
			rc.current++
			continue
		}
		return n, err
	}
	return 0, io.EOF
}

// Lines creates an Iter from io.Reader (line by line) with configuration.
// Returns an error for nil readers or scanning errors.
func (c *C) Lines(r io.Reader) (*Iter[string], error) {
	if r == nil {
		return nil, errors.New("nil reader")
	}
	return New(func(yield func(string) bool) {
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, c.bufferSize), c.maxLineLength)
		for scanner.Scan() {
			select {
			case <-c.ctx.Done():
				return
			default:
				if !yield(scanner.Text()) {
					return
				}
			}
		}
		if err := scanner.Err(); err != nil {
			return
		}
	}, c.initialCap, c.maxCap)
}

// Bytes creates an Iter from io.Reader (chunks) with configuration.
// Returns an error for nil readers or read errors (except io.EOF).
func (c *C) Bytes(r io.Reader) (*Iter[[]byte], error) {
	if r == nil {
		return nil, errors.New("nil reader")
	}
	return New(func(yield func([]byte) bool) {
		buf := make([]byte, c.bufferSize)
		for {
			select {
			case <-c.ctx.Done():
				return
			default:
				n, err := r.Read(buf)
				if n > 0 {
					chunk := make([]byte, n)
					copy(chunk, buf[:n])
					if !yield(chunk) {
						return
					}
				}
				if err != nil {
					if err != io.EOF {
						return
					}
					return
				}
			}
		}
	}, c.initialCap, c.maxCap)
}

// Runes creates an Iter from io.Reader (runes) with configuration.
// Handles UTF-8 decoding, yielding unicode.ReplacementChar for invalid sequences.
// Returns an error for nil readers or read errors (except io.EOF).
func (c *C) Runes(r io.Reader) (*Iter[rune], error) {
	if r == nil {
		return nil, errors.New("nil reader")
	}
	return New(func(yield func(rune) bool) {
		reader := bufio.NewReader(r)
		for {
			select {
			case <-c.ctx.Done():
				return
			default:
				r, _, err := reader.ReadRune()
				if err != nil {
					if err != io.EOF {
						return
					}
					return
				}
				if r == utf8.RuneError {
					r = unicode.ReplacementChar
				}
				if !yield(r) {
					return
				}
			}
		}
	}, c.initialCap, c.maxCap)
}
