package edonr

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"hash"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

var _ hash.Hash = (*Digest)(nil)

// pattern is the message of testdata/openzfs-2.3.0-vectors.txt: byte i is
// the low 8 bits of i*31+7.
func pattern(n int) []byte {
	m := make([]byte, n)
	for i := range m {
		m[i] = byte(i*31 + 7)
	}
	return m
}

func seqSalt() []byte {
	s := make([]byte, 32)
	for i := range s {
		s[i] = byte(i)
	}
	return s
}

func TestOpenZFSVectors(t *testing.T) {
	f, err := os.Open("testdata/openzfs-2.3.0-vectors.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	var unkeyed, salted int
	for sc.Scan() {
		fs := strings.Fields(sc.Text())
		if len(fs) != 3 {
			t.Fatalf("bad line %q", sc.Text())
		}
		n, err := strconv.Atoi(fs[1])
		if err != nil {
			t.Fatal(err)
		}
		want, err := hex.DecodeString(fs[2])
		if err != nil {
			t.Fatal(err)
		}
		msg := pattern(n)
		switch fs[0] {
		case "unkeyed512":
			unkeyed++
			got := Sum(msg)
			if !bytes.Equal(got[:], want) {
				t.Errorf("unkeyed512 len %d: %x want %x", n, got, want)
			}
		case "salted256":
			salted++
			d := NewKeyed(seqSalt())
			d.Write(msg)
			if got := d.Sum(nil)[:32]; !bytes.Equal(got, want) {
				t.Errorf("salted256 len %d: %x want %x", n, got, want)
			}
		default:
			t.Fatalf("unknown mode %q", fs[0])
		}
	}
	if unkeyed != 273 || salted != 273 {
		t.Fatalf("read %d unkeyed and %d salted vectors, want 273 each", unkeyed, salted)
	}
}

func TestMetadata(t *testing.T) {
	d := New()
	if d.Size() != 64 || d.BlockSize() != 128 {
		t.Fatalf("Size %d BlockSize %d", d.Size(), d.BlockSize())
	}
}

// Streaming in arbitrary chunks, repeated Sum and Reset agree with the one-shot digest.
func TestStreaming(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for _, n := range []int{0, 1, 111, 112, 119, 120, 127, 128, 129, 247, 248, 255, 256, 257, 1000, 4096} {
		msg := pattern(n)
		for _, keyed := range []bool{false, true} {
			mk := func() *Digest {
				if keyed {
					return NewKeyed(seqSalt())
				}
				return New()
			}
			ref := mk()
			ref.Write(msg)
			want := ref.Sum(nil)
			d := mk()
			for rest := msg; len(rest) > 0; {
				k := 1 + rng.Intn(200)
				if k > len(rest) {
					k = len(rest)
				}
				if c, err := d.Write(rest[:k]); c != k || err != nil {
					t.Fatalf("Write returned %d, %v", c, err)
				}
				rest = rest[k:]
			}
			if got := d.Sum(nil); !bytes.Equal(got, want) {
				t.Fatalf("n=%d keyed=%v: chunked %x want %x", n, keyed, got, want)
			}
			if got := d.Sum([]byte{9}); !bytes.Equal(got[1:], want) || got[0] != 9 {
				t.Fatalf("Sum not idempotent or not appending")
			}
			d.Reset()
			d.Write(msg)
			if got := d.Sum(nil); !bytes.Equal(got, want) {
				t.Fatalf("n=%d keyed=%v: after Reset %x want %x", n, keyed, got, want)
			}
		}
	}
}

// The keyed digest is the unkeyed digest of H(salt) || H(H(salt)) || msg.
func TestKeyedConstruction(t *testing.T) {
	for _, salt := range [][]byte{nil, {1}, seqSalt(), pattern(300)} {
		h1 := Sum(salt)
		h2 := Sum(h1[:])
		msg := pattern(77)
		want := Sum(append(append(append([]byte{}, h1[:]...), h2[:]...), msg...))
		d := NewKeyed(salt)
		d.Write(msg)
		if got := d.Sum(nil); !bytes.Equal(got, want[:]) {
			t.Fatalf("salt len %d: %x want %x", len(salt), got, want)
		}
	}
}

// oracle runs the black-box OpenZFS binary named by EDONR_ORACLE; tests that
// need it are skipped when it is not set.
func oracle(t testing.TB, msg, salt []byte) []byte {
	t.Helper()
	path := os.Getenv("EDONR_ORACLE")
	if path == "" {
		t.Skip("EDONR_ORACLE not set")
	}
	args := []string{"--bits", "512"}
	if salt != nil {
		args = append(args, "--salt", hex.EncodeToString(salt))
	}
	c := exec.Command(path, args...)
	c.Stdin = bytes.NewReader(msg)
	out, err := c.Output()
	if err != nil {
		t.Fatalf("oracle: %v", err)
	}
	b, err := hex.DecodeString(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestOracleDifferential(t *testing.T) {
	oracle(t, nil, nil) // skips when absent
	rng := rand.New(rand.NewSource(7))
	lengths := []int{0, 1, 2, 63, 64, 65, 111, 112, 119, 120, 127, 128, 129, 239, 240, 247, 248, 255, 256, 257, 1023, 1024, 1025, 4095, 4096, 4097, 65535, 65536, 65537, 1 << 20, 1<<20 + 1, 3<<20 + 77, 5 << 20}
	for i := 0; i < 60; i++ {
		lengths = append(lengths, rng.Intn(2000))
	}
	for _, n := range lengths {
		msg := make([]byte, n)
		rng.Read(msg)
		want := oracle(t, msg, nil)
		if got := Sum(msg); !bytes.Equal(got[:], want) {
			t.Fatalf("unkeyed len %d: %x want %x", n, got, want)
		}
		salt := make([]byte, 32)
		rng.Read(salt)
		wantK := oracle(t, msg, salt)
		d := NewKeyed(salt)
		d.Write(msg)
		if got := d.Sum(nil); !bytes.Equal(got, wantK) {
			t.Fatalf("keyed len %d: %x want %x", n, got, wantK)
		}
	}
}

func FuzzOracle(f *testing.F) {
	f.Add([]byte{}, []byte{})
	f.Add(pattern(200), seqSalt())
	f.Fuzz(func(t *testing.T, msg, salt []byte) {
		want := oracle(t, msg, nil)
		if got := Sum(msg); !bytes.Equal(got[:], want) {
			t.Fatalf("unkeyed %x: %x want %x", msg, got, want)
		}
		{
			wantK := oracle(t, msg, salt)
			d := NewKeyed(salt)
			d.Write(msg)
			if got := d.Sum(nil); !bytes.Equal(got, wantK) {
				t.Fatalf("keyed %x %x: %x want %x", salt, msg, got, wantK)
			}
		}
	})
}

// FuzzStreaming needs no oracle: any split of the input gives the same digest.
func FuzzStreaming(f *testing.F) {
	f.Add(pattern(300), uint8(7))
	f.Fuzz(func(t *testing.T, msg []byte, step uint8) {
		want := Sum(msg)
		d := New()
		k := int(step) + 1
		for rest := msg; len(rest) > 0; {
			c := k
			if c > len(rest) {
				c = len(rest)
			}
			d.Write(rest[:c])
			rest = rest[c:]
		}
		if got := d.Sum(nil); !bytes.Equal(got, want[:]) {
			t.Fatalf("split %d: %x want %x", k, got, want)
		}
	})
}

func BenchmarkSum4K(b *testing.B)   { benchSum(b, 4096) }
func BenchmarkSum128K(b *testing.B) { benchSum(b, 128<<10) }

func benchSum(b *testing.B, n int) {
	msg := pattern(n)
	b.SetBytes(int64(n))
	for i := 0; i < b.N; i++ {
		Sum(msg)
	}
}

// BenchmarkOracle4K and BenchmarkOracle128K report the OpenZFS C oracle's
// speed (EDONR_ORACLE required) under the same ns/op metric as Sum, so that
// abbench and benchstat can compare them interleaved:
//
//	abbench --base e.test --head e.test --base-arg=-test.bench=Oracle4K \
//	  --head-arg=-test.bench=Sum4K --rename-base Oracle4K=Sum4K --max-load 8 ...
func BenchmarkOracle4K(b *testing.B)   { benchOracle(b, 4096) }
func BenchmarkOracle128K(b *testing.B) { benchOracle(b, 128<<10) }

func benchOracle(b *testing.B, n int) {
	path := os.Getenv("EDONR_ORACLE")
	if path == "" {
		b.Skip("EDONR_ORACLE not set")
	}
	out, err := exec.Command(path, "--bench", strconv.Itoa(n)).Output()
	if err != nil {
		b.Fatal(err)
	}
	var mbps float64
	if _, err := fmt.Sscanf(string(out), "%f MB/s", &mbps); err != nil {
		b.Fatalf("oracle output %q: %v", out, err)
	}
	for i := 0; i < b.N; i++ {
	}
	b.ReportMetric(float64(n)/mbps*1e3, "ns/op")
}
