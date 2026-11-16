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

import (
	"encoding/binary"
	"math/bits"
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

// state is the double pipe: two vectors of eight 64-bit words.
type state [16]uint64

// initial is P(0) for Edon-R 512 (Table 1.6): the bytes 0x80..0xFF as
// consecutive big-endian-written 64-bit words.
var initial = func() (s state) {
	for i := range s {
		s[i] = 0x8081828384858687 + uint64(i)*0x0808080808080808
	}
	return
}()

// q is the quasigroup operation Z = X * Y of Table 2.2, with X and Y the
// eight words x0..x7 and y0..y7; it returns Z0..Z7.
func q(x0, x1, x2, x3, x4, x5, x6, x7, y0, y1, y2, y3, y4, y5, y6, y7 uint64) (z0, z1, z2, z3, z4, z5, z6, z7 uint64) {
	// Step 1 and 2: the X side, rotations of Table 2.3 (r1,512).
	t0 := constX + x0 + x1 + x2 + x4 + x7
	t1 := bits.RotateLeft64(x0+x1+x3+x4+x7, 5)
	t2 := bits.RotateLeft64(x0+x1+x4+x6+x7, 15)
	t3 := bits.RotateLeft64(x2+x3+x5+x6+x7, 22)
	t4 := bits.RotateLeft64(x1+x2+x3+x5+x6, 31)
	t5 := bits.RotateLeft64(x0+x2+x3+x4+x5, 40)
	t6 := bits.RotateLeft64(x0+x1+x5+x6+x7, 50)
	t7 := bits.RotateLeft64(x2+x3+x4+x5+x6, 59)
	t8 := t3 ^ t5 ^ t6
	t9 := t2 ^ t5 ^ t6
	t10 := t2 ^ t3 ^ t5
	t11 := t0 ^ t1 ^ t4
	t12 := t0 ^ t4 ^ t7
	t13 := t1 ^ t6 ^ t7
	t14 := t2 ^ t3 ^ t4
	t15 := t0 ^ t1 ^ t7
	// Step 3: the Y side, rotations of r2,512.
	t0 = constY + y0 + y1 + y2 + y5 + y7
	t1 = bits.RotateLeft64(y0+y1+y3+y4+y6, 10)
	t2 = bits.RotateLeft64(y0+y1+y2+y3+y5, 19)
	t3 = bits.RotateLeft64(y2+y3+y4+y6+y7, 29)
	t4 = bits.RotateLeft64(y0+y1+y3+y4+y5, 36)
	t5 = bits.RotateLeft64(y2+y4+y5+y6+y7, 44)
	t6 = bits.RotateLeft64(y1+y2+y5+y6+y7, 48)
	t7 = bits.RotateLeft64(y0+y3+y4+y6+y7, 55)
	// Step 4.
	z5 = t8 + (t3 ^ t4 ^ t6)
	z6 = t9 + (t2 ^ t5 ^ t7)
	z7 = t10 + (t4 ^ t6 ^ t7)
	z0 = t11 + (t0 ^ t1 ^ t5)
	z1 = t12 + (t2 ^ t6 ^ t7)
	z2 = t13 + (t0 ^ t1 ^ t3)
	z3 = t14 + (t0 ^ t3 ^ t4)
	z4 = t15 + (t1 ^ t2 ^ t5)
	return
}

// compress absorbs one 128-byte block: P = R(P, M) xor P xor swapped(M).
// R is Definition 8; the double pipe halves (c0, c1) enter the function as
// (P1, P0), which is the arrangement the OpenZFS output requires (the
// specification's figure 2.1 draws the halves of the output swapped).
func (s *state) compress(blk []byte) {
	_ = blk[BlockSize-1]
	var m [16]uint64
	for i := range m {
		m[i] = binary.LittleEndian.Uint64(blk[8*i:])
	}
	// X0(1) = rev(A1) * A0 ; X1(1) = X0(1) * A1
	a00, a01, a02, a03, a04, a05, a06, a07 := m[0], m[1], m[2], m[3], m[4], m[5], m[6], m[7]
	a10, a11, a12, a13, a14, a15, a16, a17 := m[8], m[9], m[10], m[11], m[12], m[13], m[14], m[15]
	u0, u1, u2, u3, u4, u5, u6, u7 := q(a17, a16, a15, a14, a13, a12, a11, a10, a00, a01, a02, a03, a04, a05, a06, a07)
	v0, v1, v2, v3, v4, v5, v6, v7 := q(u0, u1, u2, u3, u4, u5, u6, u7, a10, a11, a12, a13, a14, a15, a16, a17)
	// X0(2) = C0 * X0(1) ; X1(2) = X0(2) * X1(1)   with C0 = P1
	w0, w1, w2, w3, w4, w5, w6, w7 := q(s[8], s[9], s[10], s[11], s[12], s[13], s[14], s[15], u0, u1, u2, u3, u4, u5, u6, u7)
	x0, x1, x2, x3, x4, x5, x6, x7 := q(w0, w1, w2, w3, w4, w5, w6, w7, v0, v1, v2, v3, v4, v5, v6, v7)
	// X0(3) = X0(2) * C1 ; X1(3) = X1(2) * X0(3)   with C1 = P0
	y0, y1, y2, y3, y4, y5, y6, y7 := q(w0, w1, w2, w3, w4, w5, w6, w7, s[0], s[1], s[2], s[3], s[4], s[5], s[6], s[7])
	e0, e1, e2, e3, e4, e5, e6, e7 := q(x0, x1, x2, x3, x4, x5, x6, x7, y0, y1, y2, y3, y4, y5, y6, y7)
	// B0 = rev(A0) * X0(3) ; B1 = B0 * X1(3)
	b0, b1, b2, b3, b4, b5, b6, b7 := q(a07, a06, a05, a04, a03, a02, a01, a00, y0, y1, y2, y3, y4, y5, y6, y7)
	c0, c1, c2, c3, c4, c5, c6, c7 := q(b0, b1, b2, b3, b4, b5, b6, b7, e0, e1, e2, e3, e4, e5, e6, e7)
	// Feedback (Edon-R', Table 6): halves xor the old halves and the swapped message.
	s[0], s[1], s[2], s[3] = b0^s[0]^m[8], b1^s[1]^m[9], b2^s[2]^m[10], b3^s[3]^m[11]
	s[4], s[5], s[6], s[7] = b4^s[4]^m[12], b5^s[5]^m[13], b6^s[6]^m[14], b7^s[7]^m[15]
	s[8], s[9], s[10], s[11] = c0^s[8]^m[0], c1^s[9]^m[1], c2^s[10]^m[2], c3^s[11]^m[3]
	s[12], s[13], s[14], s[15] = c4^s[12]^m[4], c5^s[13]^m[5], c6^s[14]^m[6], c7^s[15]^m[7]
}

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
