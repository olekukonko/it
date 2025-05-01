# `it` - Buffered Iterator Utility for Go

The `it` package provides a powerful and flexible iterator for Go, enabling buffered iteration over any `iter.Seq[T]` sequence with advanced features like peeking, seeking, rewinding, and configurable buffer management. It supports various data sources (slices, channels, maps, and IO readers) and is optimized for performance and memory efficiency.

## Features

- **Buffered Iteration**: Store elements for efficient navigation and rewinding.
- **Peek Ahead**: Inspect the next element without advancing the iterator.
- **Relative and Absolute Navigation**: Use `Seek` to move relative positions or `SeekTo` for absolute positions.
- **Rewind Capability**: Reset to the initial state while preserving the buffer.
- **Configurable Buffer Limits**: Set initial and maximum buffer sizes for memory control.
- **Multiple Source Adapters**: Iterate over slices, channels, map keys/values, and IO streams (lines, bytes, runes).
- **Context Support**: Integrate cancellation for IO operations.
- **Efficient Memory Management**: Minimal allocations with optimized buffer growth.
- **Transform Functions**: Apply transformations during iteration or dumping.

## Installation

```bash
go get github.com/olekukonko/it
```

## Quick Start

Here’s a simple example using a Fibonacci sequence to demonstrate the `it` package’s core features:

```go
package main

import (
	"fmt"
	"github.com/olekukonko/it"
	"iter"
)

// Fibonacci generator
func fibonacci() iter.Seq[int] {
	return func(yield func(int) bool) {
		a, b := 0, 1
		for {
			if !yield(a) {
				return
			}
			a, b = b, a+b
		}
	}
}

func main() {
	// Create iterator from Fibonacci sequence
	it, err := it.Wrap(fibonacci())
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	// Iterate over first 5 numbers
	for i := 0; i < 5; i++ {
		v, ok := it.Next()
		if !ok {
			break
		}
		fmt.Printf("Fibonacci[%d]: %d\n", i, v)
	}

	// Peek at the next number
	if v, ok := it.Peek(); ok {
		fmt.Println("Next Fibonacci:", v)
	}

	// Rewind and iterate again
	it.Rewind()
	for i := 0; i < 3; i++ {
		v, ok := it.Next()
		if !ok {
			break
		}
		fmt.Printf("Rewound Fibonacci[%d]: %d\n", i, v)
	}
}
```

Output:
```
Fibonacci[0]: 0
Fibonacci[1]: 1
Fibonacci[2]: 1
Fibonacci[3]: 2
Fibonacci[4]: 3
Next Fibonacci: 5
Rewound Fibonacci[0]: 0
Rewound Fibonacci[1]: 1
Rewound Fibonacci[2]: 1
```

## Key Features and Examples

### Buffered Iteration

Create an iterator from a slice and navigate through elements:

```go
it, err := it.Slice([]string{"a", "b", "c"})
if err != nil {
	fmt.Println("Error:", err)
	return
}
for v, ok := it.Next(); ok; v, ok = it.Next() {
	fmt.Println(v)
}
```

### Seeking and Navigation

Move relative to the current position with `Seek` or to an absolute position with `SeekTo`:

```go
it, err := it.Slice([]string{"a", "b", "c", "d"})
if err != nil {
	fmt.Println("Error:", err)
	return
}
it.Next() // Move to "a"
if v, ok := it.Seek(1); ok { // Skip 1 element, move to "c"
	fmt.Println("After Seek(1):", v) // Output: c
}
if v, ok := it.SeekTo(1); ok { // Move to position 1 ("b")
	fmt.Println("After SeekTo(1):", v) // Output: b
}
```

### IO Operations

Read lines, bytes, or runes from an IO source with configurable buffer sizes:

```go
import "strings"

reader := strings.NewReader("line 1\nline 2\nline 3")
cfg := it.Config().BufferSize(1024).MaxLineLength(8192)
lines, err := cfg.Lines(reader)
if err != nil {
	fmt.Println("Error:", err)
	return
}
for line, ok := lines.Next(); ok; line, ok = lines.Next() {
	fmt.Println(line)
}
```

### Transform Functions

Apply transformations when dumping all elements:

```go
it, err := it.Slice([]int{1, 2, 3})
if err != nil {
	fmt.Println("Error:", err)
	return
}
doubled, err := it.Dump(func(n int) int { return n * 2 })
if err != nil {
	fmt.Println("Error:", err)
	return
}
fmt.Println(doubled) // Output: [2 4 6]
```

## API Reference

### Core Methods

- `Next() (T, bool)`: Advances to the next element, returning its value and whether it exists.
- `Peek() (T, bool)`: Returns the next element without advancing.
- `Seek(n int) (T, bool)`: Moves `n` elements forward (or backward if negative), returning the element at the new position.
- `SeekTo(pos int) (T, bool)`: Jumps to the absolute position `pos`.
- `Rewind()`: Resets the iterator to the initial state.
- `Buffer() []T`: Returns a copy of buffered elements.
- `Dump(...func(T) T) ([]T, error)`: Consumes the iterator and returns all elements, optionally applying transformations.
- `Position() int`: Returns the current position.
- `BufferedLen() int`: Returns the number of buffered elements.
- `Cap() int`: Returns the current buffer capacity.

### Factory Functions

- `New[T](src Seq[T], initialCap, maxCap int) (*Iter[T], error)`: Creates an iterator with custom buffer settings.
- `Wrap[T](src Seq[T]) (*Iter[T], error)`: Creates an iterator with default settings (initialCap=64, maxCap=0).
- `Must[T](src Seq[T]) *Iter[T]`: Creates an iterator or panics on error.
- `Slice[T](s []T) (*Iter[T], error)`: Creates an iterator from a slice.
- `Chan[T](c *C, ch <-chan T) (*Iter[T], error)`: Creates an iterator from a channel.
- `Keys[K, V](m map[K]V) (*Iter[K], error)`: Creates an iterator over map keys.
- `Vals[K, V](m map[K]V) (*Iter[V], error)`: Creates an iterator over map values.
- `Pairs[K, V](m map[K]V) (*Iter[Pair[K, V]], error)`: Creates an iterator over map key-value pairs.
- `Tea(sources ...io.Reader) (*Iter[string], error)`: Creates an iterator over lines from multiple readers.
- `C.Lines(r io.Reader) (*Iter[string], error)`: Creates an iterator over lines from a reader.
- `C.Bytes(r io.Reader) (*Iter[[]byte], error)`: Creates an iterator over byte chunks.
- `C.Runes(r io.Reader) (*Iter[rune], error)`: Creates an iterator over runes.

### Configuration

Customize iterator behavior with the `Config` builder:

```go
cfg := it.Config().
	InitialCap(128).           // Initial buffer capacity
	MaxCap(1024).              // Maximum buffer capacity
	BufferSize(8192).          // IO buffer size
	MaxLineLength(128 * 1024). // Maximum line length
	WithContext(ctx)           // Context for cancellation

it, err := cfg.Lines(reader)
```

## Testing and Reliability

The `it` package is thoroughly tested to ensure reliability across various use cases, including:
- Basic iteration (`TestNext`, `TestSeq`)
- Peeking and seeking (`TestPeek`, `TestSeek`, `TestSeekTo`)
- Buffer management (`TestBuffer`)
- IO operations (`TestLines`, `TestBytes`, `TestRunes`)
- Edge cases (`TestEmptyIterator`, `TestSingleElement`)

Run the tests to verify:

```bash
go test -v
```

## Performance

- **Minimal Allocations**: Optimized buffer growth reduces memory overhead.
- **Configurable Limits**: Set `maxCap` to control memory usage.
- **Efficient Navigation**: Buffered elements enable fast seeking and rewinding.
- **Concurrent Safety**: Channel-based iteration ensures safe sequence consumption.

## Contributing

Contributions are welcome! Please submit issues or pull requests to the GitHub repository: `github.com/olekukonko/it`.

## License

MIT License