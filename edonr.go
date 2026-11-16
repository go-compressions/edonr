// Package edonr implements the Edon-R' hash function (512-bit variant), the
// function behind the OpenZFS checksum=edonr property, including the keyed
// ("salted") form ZFS computes for a pool with a checksum salt.
//
// This is a clean-room implementation written from the published
// specification (see the README for the provenance); the digest is checked
// against the output of the OpenZFS implementation used as a black box.
//
// Specification, section numbers are those of the Edon-R supporting
// documentation (NTNU, 2008) unless another source is named:
//
//   - 1.4.1 padding: a 1 bit, zero bits up to 960 mod 1024, then the 64-bit
//     message length in bits; 1.1 words are little-endian.
//   - 1.4.3 Table 1.6: the initial double pipe, bytes 0x80..0xFF.
//   - 2.1.1 Table 2.2: the quasigroup operation of order 2^512 (function q).
//   - 2.1.3 Definition 7 and 8, Table 2.5: the function R and the reversed
//     vector.
//   - Edon-R' paper (Gligoroski et al., 2009), Table 6: the feedback
//     P(i) = R(P(i-1), M(i)) xor P(i-1) xor swapped(M(i)).
//
// The keyed form is not in the specification; it was determined from the
// black box and is documented on NewKeyed.
package edonr

//go:generate go run ./internal/gen

import (
	"encoding/binary"
)

const (
	// Size is the length of a digest in bytes (Edon-R 512).
	Size = 64
	// BlockSize is the message block size in bytes (1024 bits).
	BlockSize = 128
)

// The two constants added by the affine maps of the quasigroup (Table 2.2).
const (
	constX = 0xAAAAAAAAAAAAAAAA
	constY = 0x5555555555555555
)

// state is the double pipe: two vectors of eight 64-bit words, and a scratch
// word that compress stores to between the phases of a block (see the
// generator): the store makes the compiler reload message words rather than
// keep them live in registers. It carries no value.
type state [17]uint64

// initial is P(0) for Edon-R 512 (Table 1.6): the bytes 0x80..0xFF as
// consecutive big-endian-written 64-bit words.
var initial = func() (s state) {
	for i := range s {
		s[i] = 0x8081828384858687 + uint64(i)*0x0808080808080808
	}
	return
}()

// Digest is a streaming Edon-R 512 hash. It implements hash.Hash.
type Digest struct {
	s     state
	start state  // state Reset returns to
	n0    uint64 // bytes already absorbed into start
	n     uint64 // total bytes absorbed, including n0
	buf   [BlockSize]byte
	nbuf  int
}

// New returns an unkeyed Edon-R 512 digest.
func New() *Digest {
	d := &Digest{start: initial}
	d.Reset()
	return d
}

// NewKeyed returns the keyed (salted) digest ZFS computes for a pool whose
// checksum salt is salt. The key is not part of the Edon-R specification: it
// was determined by black-box queries and is exact for every length that was
// tried. The digest of a message m is the unkeyed digest of
//
//	H(salt) || H(H(salt)) || m
//
// where H is the unkeyed 64-byte digest, so the two digests make up exactly
// one block and the keyed state is a precomputed template.
func NewKeyed(salt []byte) *Digest {
	h1 := Sum(salt)
	h2 := Sum(h1[:])
	var blk [BlockSize]byte
	copy(blk[:], h1[:])
	copy(blk[Size:], h2[:])
	d := &Digest{start: initial}
	d.start.compress(blk[:])
	d.n0 = BlockSize
	d.Reset()
	return d
}

// Reset restores the digest to its initial (or keyed) state.
func (d *Digest) Reset() {
	d.s = d.start
	d.n = d.n0
	d.nbuf = 0
}

// Size returns Size.
func (d *Digest) Size() int { return Size }

// BlockSize returns BlockSize.
func (d *Digest) BlockSize() int { return BlockSize }

// Write absorbs p; it never fails.
func (d *Digest) Write(p []byte) (int, error) {
	total := len(p)
	d.n += uint64(total)
	if d.nbuf > 0 {
		c := copy(d.buf[d.nbuf:], p)
		d.nbuf += c
		p = p[c:]
		if d.nbuf < BlockSize {
			return total, nil
		}
		d.s.compress(d.buf[:])
		d.nbuf = 0
	}
	for len(p) >= BlockSize {
		d.s.compress(p[:BlockSize])
		p = p[BlockSize:]
	}
	d.nbuf = copy(d.buf[:], p)
	return total, nil
}

// Sum appends the digest to b without changing the state of d.
func (d *Digest) Sum(b []byte) []byte {
	c := *d
	// Padding (1.4.1): a 1 bit, zeros, the length in bits (little-endian).
	var pad [2 * BlockSize]byte
	copy(pad[:], c.buf[:c.nbuf])
	pad[c.nbuf] = 0x80
	end := BlockSize
	if c.nbuf >= BlockSize-8 {
		end = 2 * BlockSize
	}
	binary.LittleEndian.PutUint64(pad[end-8:], c.n<<3)
	c.s.compress(pad[:BlockSize])
	if end == 2*BlockSize {
		c.s.compress(pad[BlockSize:])
	}
	// The digest is the 512 least significant bits: the second half of the pipe.
	var out [Size]byte
	for i := 0; i < 8; i++ {
		binary.LittleEndian.PutUint64(out[8*i:], c.s[8+i])
	}
	return append(b, out[:]...)
}

// Sum returns the Edon-R 512 digest of data.
func Sum(data []byte) [Size]byte {
	d := New()
	d.Write(data)
	var out [Size]byte
	d.Sum(out[:0])
	return out
}
