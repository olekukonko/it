package it

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
	"unicode"
)

func TestMain(m *testing.M) {
	// Set a timeout for the entire test suite
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan int)
	go func() {
		done <- m.Run()
	}()
	select {
	case code := <-done:
		os.Exit(code)
	case <-ctx.Done():
		fmt.Println("Test suite timed out")
		os.Exit(1)
	}
}

// TestNew tests the New function for creating iterators.
func TestNew(t *testing.T) {
	// Test nil source
	_, err := New[string](nil, 64, 0)
	if err == nil || err.Error() != "nil source sequence" {
		t.Errorf("New(nil) expected error 'nil source sequence', got %v", err)
	}

	// Test valid creation with default initialCap
	it, err := New(func(yield func(string) bool) { yield("a") }, 0, 0)
	if err != nil {
		t.Errorf("New expected no error, got %v", err)
	}
	if it.Cap() != 64 {
		t.Errorf("New expected capacity 64, got %d", it.Cap())
	}
}

// TestWrap tests the Wrap function.
func TestWrap(t *testing.T) {
	it, err := Wrap(func(yield func(string) bool) { yield("a") })
	if err != nil {
		t.Errorf("Wrap expected no error, got %v", err)
	}
	if it.Cap() != 64 || it.maxCap != 0 {
		t.Errorf("Wrap expected cap=64, maxCap=0, got cap=%d, maxCap=%d", it.Cap(), it.maxCap)
	}
}

// TestMust tests the Must function.
func TestMust(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Must(nil) expected panic, got none")
		}
	}()
	Must[string](nil)
}

// TestNext tests the Next method.
func TestNext(t *testing.T) {
	it, _ := Slice([]string{"a", "b", "c"})
	tests := []struct {
		want    string
		wantOk  bool
		wantPos int
	}{
		{"a", true, 0},
		{"b", true, 1},
		{"c", true, 2},
		{"", false, 2},
	}
	for i, tt := range tests {
		got, ok := it.Next()
		if got != tt.want || ok != tt.wantOk {
			t.Errorf("Next #%d: got %v, %v, want %v, %v", i, got, ok, tt.want, tt.wantOk)
		}
		if it.Position() != tt.wantPos {
			t.Errorf("Next #%d: position got %d, want %d", i, it.Position(), tt.wantPos)
		}
	}
}

// TestPeek tests the Peek method.
func TestPeek(t *testing.T) {
	it, _ := Slice([]string{"a", "b"})
	got, ok := it.Peek()
	if got != "a" || !ok {
		t.Errorf("Peek: got %v, %v, want a, true", got, ok)
	}
	if it.Position() != -1 {
		t.Errorf("Peek: position got %d, want -1", it.Position())
	}
	it.Next()
	got, ok = it.Peek()
	if got != "b" || !ok {
		t.Errorf("Peek after Next: got %v, %v, want b, true", got, ok)
	}
}

// TestSeek tests the Seek method.
func TestSeek(t *testing.T) {
	it, _ := Slice([]string{"a", "b", "c"})
	it.Next()             // Move to "a"
	got, ok := it.Seek(1) // Seek to "c"
	if got != "c" || !ok {
		t.Errorf("Seek(1): got %v, %v, want c, true", got, ok)
	}
	if it.Position() != 2 {
		t.Errorf("Seek(1): position got %d, want 2", it.Position())
	}
}

// TestSeekTo tests the SeekTo method.
func TestSeekTo(t *testing.T) {
	it, _ := Slice([]string{"a", "b", "c"})
	got, ok := it.SeekTo(1)
	if got != "b" || !ok {
		t.Errorf("SeekTo(1): got %v, %v, want b, true", got, ok)
	}
	if it.Position() != 1 {
		t.Errorf("SeekTo(1): position got %d, want 1", it.Position())
	}
	got, ok = it.SeekTo(-1)
	if got != "" || ok {
		t.Errorf("SeekTo(-1): got %v, %v, want '', false", got, ok)
	}
}

// TestRewind tests the Rewind method.
func TestRewind(t *testing.T) {
	it, _ := Slice([]string{"a", "b"})
	it.Next()
	it.Next()
	it.Rewind()
	if it.Position() != -1 {
		t.Errorf("Rewind: position got %d, want -1", it.Position())
	}
	got, ok := it.Next()
	if got != "a" || !ok {
		t.Errorf("Next after Rewind: got %v, %v, want a, true", got, ok)
	}
}

// TestCurrent tests the Current method.
func TestCurrent(t *testing.T) {
	it, _ := Slice([]string{"a", "b"})
	got, ok := it.Current()
	if got != "" || ok {
		t.Errorf("Current before Next: got %v, %v, want '', false", got, ok)
	}
	it.Next()
	got, ok = it.Current()
	if got != "a" || !ok {
		t.Errorf("Current after Next: got %v, %v, want a, true", got, ok)
	}
}

// TestBuffer tests the Buffer method.
func TestBuffer(t *testing.T) {
	it, _ := Slice([]string{"a", "b", "c"})
	it.Next()
	it.Next()
	got := it.Buffer()
	want := []string{"a", "b"}
	if len(got) != len(want) {
		t.Errorf("Buffer: got len %d, want %d", len(got), len(want))
	}
	for i, v := range got {
		if v != want[i] {
			t.Errorf("Buffer: got[%d] = %v, want %v", i, v, want[i])
		}
	}
}

// TestSeq tests the Seq method for range loop compatibility.
func TestSeq(t *testing.T) {
	it, _ := Slice([]string{"a", "b", "c"})
	var got []string
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for v := range it.Seq() {
		select {
		case <-ctx.Done():
			t.Fatalf("TestSeq timed out")
		default:
			t.Logf("Seq: got %v", v)
			got = append(got, v)
		}
	}
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Errorf("Seq: got len %d, want %d", len(got), len(want))
	}
	for i, v := range got {
		if v != want[i] {
			t.Errorf("Seq: got[%d] = %v, want %v", i, v, want[i])
		}
	}
}

// TestSlice tests the Slice function.
func TestSlice(t *testing.T) {
	_, err := Slice[string](nil)
	if err == nil || err.Error() != "nil slice" {
		t.Errorf("Slice(nil) expected error 'nil slice', got %v", err)
	}
	it, _ := Slice([]string{"a", "b"})
	var got []string
	for v, ok := it.Next(); ok; v, ok = it.Next() {
		got = append(got, v)
	}
	want := []string{"a", "b"}
	if len(got) != len(want) {
		t.Errorf("Slice: got len %d, want %d", len(got), len(want))
	}
	for i, v := range got {
		if v != want[i] {
			t.Errorf("Slice: got[%d] = %v, want %v", i, v, want[i])
		}
	}
}

// TestChan tests the Chan function.
func TestChan(t *testing.T) {
	_, err := Chan[string](Config(), nil)
	if err == nil || err.Error() != "nil channel" {
		t.Errorf("Chan(nil) expected error 'nil channel', got %v", err)
	}

	ch := make(chan string, 2)
	ch <- "a"
	ch <- "b"
	close(ch)
	it, _ := Chan(Config(), ch)
	var got []string
	for v, ok := it.Next(); ok; v, ok = it.Next() {
		got = append(got, v)
	}
	want := []string{"a", "b"}
	if len(got) != len(want) {
		t.Errorf("Chan: got len %d, want %d", len(got), len(want))
	}
	for i, v := range got {
		if v != want[i] {
			t.Errorf("Chan: got[%d] = %v, want %v", i, v, want[i])
		}
	}
}

// TestChanContext tests the Chan function with context cancellation.
func TestChanContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan string)
	go func() {
		ch <- "a"
		cancel()
	}()
	it, _ := Chan(Config().WithContext(ctx), ch)
	v, ok := it.Next()
	if v != "a" || !ok {
		t.Errorf("Chan with context: first Next got %v, %v, want a, true", v, ok)
	}
	v, ok = it.Next()
	if v != "" || ok {
		t.Errorf("Chan with context: second Next got %v, %v, want '', false", v, ok)
	}
}

// TestKeys tests the Keys function.
func TestKeys(t *testing.T) {
	_, err := Keys[string, int](nil)
	if err == nil || err.Error() != "nil map" {
		t.Errorf("Keys(nil) expected error 'nil map', got %v", err)
	}
	m := map[string]int{"a": 1, "b": 2}
	it, _ := Keys(m)
	got := make(map[string]bool)
	for k, ok := it.Next(); ok; k, ok = it.Next() {
		got[k] = true
	}
	for k := range m {
		if !got[k] {
			t.Errorf("Keys: missing key %v", k)
		}
	}
}

// TestVals tests the Vals function.
func TestVals(t *testing.T) {
	_, err := Vals[string, int](nil)
	if err == nil || err.Error() != "nil map" {
		t.Errorf("Vals(nil) expected error 'nil map', got %v", err)
	}
	m := map[string]int{"a": 1, "b": 2}
	it, _ := Vals(m)
	got := make(map[int]bool)
	for v, ok := it.Next(); ok; v, ok = it.Next() {
		got[v] = true
	}
	for _, v := range m {
		if !got[v] {
			t.Errorf("Vals: missing value %v", v)
		}
	}
}

// TestPairs tests the Pairs function.
func TestPairs(t *testing.T) {
	_, err := Pairs[string, int](nil)
	if err == nil || err.Error() != "nil map" {
		t.Errorf("Pairs(nil) expected error 'nil map', got %v", err)
	}
	m := map[string]int{"a": 1, "b": 2}
	it, _ := Pairs(m)
	got := make(map[string]int)
	for p, ok := it.Next(); ok; p, ok = it.Next() {
		got[p.Key] = p.Value
	}
	for k, v := range m {
		if got[k] != v {
			t.Errorf("Pairs: got %v=%v, want %v=%v", k, got[k], k, v)
		}
	}
}

// TestTea tests the Tea function.
func TestTea(t *testing.T) {
	// Test empty sources
	it, err := Tea()
	if err != nil {
		t.Errorf("Tea(): unexpected error %v", err)
	}
	_, ok := it.Next()
	if ok {
		t.Errorf("Tea(): Next on empty expected false, got true")
	}

	// Test nil reader
	_, err = Tea(strings.NewReader("a"), nil)
	if err == nil || err.Error() != "nil reader in sources" {
		t.Errorf("Tea(nil reader) expected error 'nil reader in sources', got %v", err)
	}

	// Test multiple readers
	r1 := strings.NewReader("line 1\nline 2\n")
	r2 := strings.NewReader("line 3\n")
	it, _ = Tea(r1, r2)
	var got []string
	for v := range it.Seq() {
		got = append(got, v)
	}
	want := []string{"line 1", "line 2", "line 3"}
	if len(got) != len(want) {
		t.Errorf("Tea: got len %d, want %d", len(got), len(want))
	}
	for i, v := range got {
		if v != want[i] {
			t.Errorf("Tea: got[%d] = %v, want %v", i, v, want[i])
		}
	}
}

// TestLines tests the Lines function.
func TestLines(t *testing.T) {
	_, err := Config().Lines(nil)
	if err == nil || err.Error() != "nil reader" {
		t.Errorf("Lines(nil) expected error 'nil reader', got %v", err)
	}

	r := strings.NewReader("line 1\nline 2\n")
	it, _ := Config().Lines(r)
	var got []string
	for v := range it.Seq() {
		got = append(got, v)
	}
	want := []string{"line 1", "line 2"}
	if len(got) != len(want) {
		t.Errorf("Lines: got len %d, want %d", len(got), len(want))
	}
	for i, v := range got {
		if v != want[i] {
			t.Errorf("Lines: got[%d] = %v, want %v", i, v, want[i])
		}
	}
}

// TestBytes tests the Bytes function.
func TestBytes(t *testing.T) {
	_, err := Config().Bytes(nil)
	if err == nil || err.Error() != "nil reader" {
		t.Errorf("Bytes(nil) expected error 'nil reader', got %v", err)
	}

	r := strings.NewReader("abc")
	it, _ := Config().BufferSize(2).Bytes(r)
	var got []byte
	for b := range it.Seq() {
		got = append(got, b...)
	}
	want := []byte("abc")
	if len(got) != len(want) {
		t.Errorf("Bytes: got len %d, want %d", len(got), len(want))
	}
	for i, v := range got {
		if v != want[i] {
			t.Errorf("Bytes: got[%d] = %v, want %v", i, v, want[i])
		}
	}
}

// TestRunes tests the Runes function.
func TestRunes(t *testing.T) {
	_, err := Config().Runes(nil)
	if err == nil || err.Error() != "nil reader" {
		t.Errorf("Runes(nil) expected error 'nil reader', got %v", err)
	}

	// Test valid UTF-8
	r1 := strings.NewReader("a😊b")
	it, _ := Config().Runes(r1)
	var got []rune
	for r := range it.Seq() {
		got = append(got, r)
	}
	want := []rune{'a', '😊', 'b'}
	if len(got) != len(want) {
		t.Errorf("Runes: got len %d, want %d", len(got), len(want))
	}
	for i, v := range got {
		if v != want[i] {
			t.Errorf("Runes: got[%d] = %v, want %v", i, v, want[i])
		}
	}

	// Test invalid UTF-8
	r2 := bytes.NewReader([]byte{0xff})
	it, _ = Config().Runes(r2)
	rune, ok := it.Next()
	if rune != unicode.ReplacementChar || !ok {
		t.Errorf("Runes invalid UTF-8: got %v, %v, want %v, true", rune, ok, unicode.ReplacementChar)
	}
}

// TestConfig tests the configuration builder.
func TestConfig(t *testing.T) {
	c := Config().InitialCap(128).MaxCap(256).BufferSize(1024).MaxLineLength(2048).WithContext(context.Background())
	if c.initialCap != 128 {
		t.Errorf("Config.InitialCap: got %d, want 128", c.initialCap)
	}
	if c.maxCap != 256 {
		t.Errorf("Config.MaxCap: got %d, want 256", c.maxCap)
	}
	if c.bufferSize != 1024 {
		t.Errorf("Config.BufferSize: got %d, want 1024", c.bufferSize)
	}
	if c.maxLineLength != 2048 {
		t.Errorf("Config.MaxLineLength: got %d, want 2048", c.maxLineLength)
	}
	if c.ctx == nil {
		t.Errorf("Config.WithContext: got nil, want non-nil context")
	}
}

func TestDump(t *testing.T) {
	it, _ := Slice([]string{"a", "b", "c"})
	it.Next()
	got, err := it.Dump()
	if err != nil {
		t.Fatalf("Dump: unexpected error %v", err)
	}
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Errorf("Dump: got len %d, want %d", len(got), len(want))
	}
	for i, v := range got {
		if v != want[i] {
			t.Errorf("Dump: got[%d] = %v, want %v", i, v, want[i])
		}
	}
	if it.Position() != -1 {
		t.Errorf("Dump: position got %d, want -1", it.Position())
	}
}

func TestEmptyIterator(t *testing.T) {
	it, _ := Slice([]string{})
	v, ok := it.Next()
	if ok || v != "" {
		t.Errorf("Empty iterator: got %v, %v, want '', false", v, ok)
	}
}

func TestSingleElement(t *testing.T) {
	it, _ := Slice([]string{"a"})
	v, ok := it.Next()
	if v != "a" || !ok {
		t.Errorf("Single element: got %v, %v, want a, true", v, ok)
	}
	v, ok = it.Next()
	if ok || v != "" {
		t.Errorf("Single element: got %v, %v, want '', false", v, ok)
	}
}

func TestSeekMultiple(t *testing.T) {
	it, _ := Slice([]string{"a", "b", "c", "d"})
	it.Next()             // Move to "a", pos=0
	got, ok := it.Seek(2) // Skip 2 elements, expect "d", pos=3
	if got != "d" || !ok || it.Position() != 3 {
		t.Errorf("Seek(2): got %v, %v, pos=%d, want d, true, pos=3", got, ok, it.Position())
	}
}
