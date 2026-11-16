package edonr

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"testing"
)

func message(n int) []byte {
	m := make([]byte, n)
	for i := range m {
		m[i] = byte(i*31 + 7)
	}
	return m
}

func ramp(n int) []byte {
	k := make([]byte, n)
	for i := range k {
		k[i] = byte(i)
	}
	return k
}

// TestOpenZFSVectors checks every line of the vector file, produced by
// compiling OpenZFS zfs-2.3.0 module/icp/algs/edonr verbatim (see README).
func TestOpenZFSVectors(t *testing.T) {
	f, err := os.Open("testdata/openzfs-2.3.0-vectors.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	count := map[string]int{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fs := strings.Fields(sc.Text())
		if len(fs) != 3 {
			t.Fatalf("bad line %q", sc.Text())
		}
		n, err := strconv.Atoi(fs[1])
		if err != nil {
			t.Fatal(err)
		}
		var got string
		switch fs[0] {
		case "unkeyed512":
			s := Sum(message(n))
			got = hex.EncodeToString(s[:])
		case "salted256":
			d := NewKeyed(ramp(32))
			d.Write(message(n))
			got = hex.EncodeToString(d.Sum(nil)[:32])
		default:
			t.Fatalf("unknown mode %q", fs[0])
		}
		if got != fs[2] {
			t.Fatalf("%s len %d: got %s want %s", fs[0], n, got, fs[2])
		}
		count[fs[0]]++
	}
	if len(count) != 2 {
		t.Fatalf("modes seen: %v", count)
	}
}

func TestStreamingResetClone(t *testing.T) {
	m := message(1000)
	whole := NewKeyed(ramp(32))
	whole.Write(m)
	want := whole.Sum(nil)
	for _, step := range []int{1, 7, 127, 128, 129, 300} {
		d := NewKeyed(ramp(32))
		for i := 0; i < len(m); i += step {
			e := min(i+step, len(m))
			d.Write(m[i:e])
		}
		if !bytes.Equal(d.Sum(nil), want) {
			t.Fatalf("step %d differs", step)
		}
		if !bytes.Equal(d.Sum(nil), want) {
			t.Fatalf("step %d: Sum not idempotent", step)
		}
		d.Reset()
		d.Write(m)
		if bytes.Equal(d.Sum(nil), want) {
			t.Fatal("Reset must drop the key")
		}
	}
	if whole.Size() != 64 || whole.BlockSize() != 128 {
		t.Fatal("size")
	}
	a := whole.Clone()
	a.Write([]byte("x"))
	if !bytes.Equal(whole.Sum(nil), want) {
		t.Fatal("clone disturbed the original")
	}
	pre := []byte("pre")
	if got := whole.Sum(pre); !bytes.Equal(got[:3], pre) || !bytes.Equal(got[3:], want) {
		t.Fatal("Sum must append")
	}
}

func BenchmarkEdonR_4K(b *testing.B) {
	m := message(4096)
	tmpl := NewKeyed(ramp(32))
	b.SetBytes(4096)
	for i := 0; i < b.N; i++ {
		d := tmpl.Clone()
		d.Write(m)
		d.Sum(nil)
	}
}

func BenchmarkEdonR_128K(b *testing.B) {
	m := message(128 << 10)
	tmpl := NewKeyed(ramp(32))
	b.SetBytes(int64(len(m)))
	for i := 0; i < b.N; i++ {
		d := tmpl.Clone()
		d.Write(m)
		d.Sum(nil)
	}
}
