package edonr

import (
	"math/rand"
	"testing"
)

// SumN must equal Sum on every message, whatever the mix of lengths per lane
// (padding in one block or two, empty messages, lanes of unequal block counts),
// unkeyed and keyed.
func TestSumNMatchesSum(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	salt := make([]byte, 32)
	rng.Read(salt)
	keyed := NewKeyed(salt)
	special := []int{0, 1, 119, 120, 121, 127, 128, 129, 247, 248, 255, 256, 257, 383, 384, 4096, 16384}
	for round := 0; round < 60; round++ {
		n := 1 + rng.Intn(9)
		msgs := make([][]byte, n)
		for i := range msgs {
			l := rng.Intn(1500)
			if rng.Intn(2) == 0 {
				l = special[rng.Intn(len(special))]
			}
			msgs[i] = make([]byte, l)
			rng.Read(msgs[i])
		}
		for _, d := range []*Digest{New(), keyed} {
			got := make([][Size]byte, n)
			d.SumN(got, msgs)
			for i, m := range msgs {
				c := *d
				c.Reset()
				c.Write(m)
				var want [Size]byte
				c.Sum(want[:0])
				if got[i] != want {
					t.Fatalf("round %d message %d (len %d): SumN differs from Sum", round, i, len(m))
				}
			}
		}
	}
}

func benchSumN(b *testing.B, size, count int) {
	msgs := make([][]byte, count)
	for i := range msgs {
		msgs[i] = make([]byte, size)
	}
	dst := make([][Size]byte, count)
	d := NewKeyed(make([]byte, 32))
	b.SetBytes(int64(size * count))
	for i := 0; i < b.N; i++ {
		d.SumN(dst, msgs)
	}
}

func BenchmarkSumN4K(b *testing.B)   { benchSumN(b, 4096, 64) }
func BenchmarkSumN128K(b *testing.B) { benchSumN(b, 131072, 16) }

func TestSumNParallelMatchesSum(t *testing.T) {
	rng := rand.New(rand.NewSource(9))
	msgs := make([][]byte, 203)
	for i := range msgs {
		msgs[i] = make([]byte, rng.Intn(700))
		rng.Read(msgs[i])
	}
	want := make([][Size]byte, len(msgs))
	for i, m := range msgs {
		want[i] = Sum(m)
	}
	for _, w := range []int{0, 1, 2, 3, 16, 1000} {
		got := make([][Size]byte, len(msgs))
		SumNParallel(got, msgs, w)
		for i := range msgs {
			if got[i] != want[i] {
				t.Fatalf("workers %d, message %d (len %d): differs from Sum", w, i, len(msgs[i]))
			}
		}
	}
	SumNParallel(nil, nil, 4) // no message is not an error
}

func BenchmarkSumNParallel4K(b *testing.B) {
	msgs := make([][]byte, 4096)
	for i := range msgs {
		msgs[i] = make([]byte, 4096)
	}
	dst := make([][Size]byte, len(msgs))
	d := NewKeyed(make([]byte, 32))
	b.SetBytes(int64(4096 * len(msgs)))
	for i := 0; i < b.N; i++ {
		d.SumNParallel(dst, msgs, 0)
	}
}
