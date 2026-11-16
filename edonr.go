/*
 * CDDL HEADER START
 *
 * The contents of this file are subject to the terms of the
 * Common Development and Distribution License (the "License").
 * You may not use this file except in compliance with the License.
 *
 * You can obtain a copy of the license at usr/src/OPENSOLARIS.LICENSE
 * or http://opensource.org/licenses/CDDL-1.0.
 * See the License for the specific language governing permissions
 * and limitations under the License.
 *
 * When distributing Covered Code, include this CDDL HEADER in each
 * file and include the License file at usr/src/OPENSOLARIS.LICENSE.
 * If applicable, add the following below this CDDL HEADER, with the
 * fields enclosed by brackets "[]" replaced with your own identifying
 * information: Portions Copyright [yyyy] [name of copyright owner]
 *
 * CDDL HEADER END
 */

/*
 * Based on Edon-R implementation for SUPERCOP, based on NIST API.
 * Copyright (c) 2009, 2010, Jørn Amundsen <jorn.amundsen@ntnu.no>
 * Copyright (c) 2013 Saso Kiselkov, All rights reserved
 * Copyright (c) 2023 Tino Reichardt <milky-zfs@mcmilk.de>
 *
 * Go port: modified by the go-compressions authors (2026), see the Git
 * history for the changes.
 */

// Package edonr implements the Edon-R 512 hash in pure Go, as OpenZFS uses it
// for the edonr checksum.
//
// It is a port of OpenZFS zfs-2.3.0 module/icp/algs/edonr/edonr.c, itself
// based on the Edon-R SUPERCOP implementation (Copyright (c) 2009, 2010 Jorn
// Amundsen; (c) 2013 Saso Kiselkov; (c) 2023 Tino Reichardt), distributed
// under the CDDL-1.0; see LICENSE. Only the byte-oriented interface is
// ported (the C one also accepts bit lengths, which ZFS never uses), and
// only the little-endian word order.
//
// The ZFS checksum is the first 32 bytes of the 64-byte digest, keyed by
// hashing a 128-byte salt block first (edonr_zfs.c:78-104); see NewKeyed.
package edonr

import (
	"encoding/binary"
	"hash"
	"math/bits"
)

const (
	// BlockSize is EdonR512_BLOCK_SIZE, edonr.h:52.
	BlockSize = 128
	// Size is EdonR512_DIGEST_SIZE, edonr.h:51.
	Size = 64
)

// i512p2 is the initial double chaining pipe, edonr.c:60-69.
var i512p2 = [16]uint64{
	0x8081828384858687, 0x88898a8b8c8d8e8f,
	0x9091929394959697, 0x98999a9b9c9d9e9f,
	0xa0a1a2a3a4a5a6a7, 0xa8a9aaabacadaeaf,
	0xb0b1b2b3b4b5b6b7, 0xb8b9babbbcbdbebf,
	0xc0c1c2c3c4c5c6c7, 0xc8c9cacbcccdcecf,
	0xd0d1d2d3d4d5d6d7, 0xd8d9dadbdcdddedf,
	0xe0e1e2e3e4e5e6e7, 0xe8e9eaebecedeeef,
	0xf0f1f2f3f4f5f6f7, 0xf8f9fafbfcfdfeff,
}

// Digest is an Edon-R 512 state. It implements hash.Hash.
type Digest struct {
	p      [16]uint64 // DoublePipe
	nBytes uint64     // bytes compressed so far (bits_processed / 8)
	last   [BlockSize]byte
	nLast  int

	// start is the state Reset returns to: the initial pipe for New, the
	// state after the salt block for NewKeyed.
	start      [16]uint64
	startBytes uint64
}

var _ hash.Hash = (*Digest)(nil)

// New returns an unkeyed Edon-R 512 digest (EdonRInit).
func New() *Digest {
	d := &Digest{start: i512p2}
	d.Reset()
	return d
}

// NewKeyed returns the ZFS checksum template for a 32-byte pool salt:
// the salt block H(salt) || H(H(salt)) is fed to a fresh state, which then
// serves as the MAC key (abd_checksum_edonr_tmpl_init, edonr_zfs.c:78-104).
// Take the first 32 bytes of Sum as the zio_cksum_t. ZFS salts are 32 bytes
// (zio_cksum_salt_t); other lengths are hashed the same way but match no
// pool. Reset returns to the keyed state.
func NewKeyed(salt []byte) *Digest {
	var sb [BlockSize]byte
	h := Sum(salt)
	copy(sb[:Size], h[:])
	h = Sum(sb[:Size])
	copy(sb[Size:], h[:])
	d := New()
	d.Write(sb[:])
	d.start, d.startBytes = d.p, d.nBytes
	return d
}

// Sum returns the Edon-R 512 digest of data (EdonRHash).
func Sum(data []byte) [Size]byte {
	d := New()
	d.Write(data)
	var out [Size]byte
	d.Sum(out[:0])
	return out
}

// Reset is EdonRInit, edonr.c:211-217.
func (d *Digest) Reset() {
	d.p = d.start
	d.nBytes = d.startBytes
	d.nLast = 0
}

// Size returns 64.
func (d *Digest) Size() int { return Size }

// BlockSize returns 128.
func (d *Digest) BlockSize() int { return BlockSize }

// Clone returns an independent copy of d, the analogue of the ZFS context
// template copy.
func (d *Digest) Clone() *Digest {
	c := *d
	return &c
}

// Write is EdonRUpdate, edonr.c:219-255, as plain buffering: whole blocks go
// to the compression function, the rest is kept.
func (d *Digest) Write(p []byte) (int, error) {
	n := len(p)
	if d.nLast > 0 {
		k := copy(d.last[d.nLast:], p)
		d.nLast += k
		p = p[k:]
		if d.nLast < BlockSize {
			return n, nil
		}
		d.compress(d.last[:])
		d.nLast = 0
	}
	if full := len(p) &^ (BlockSize - 1); full > 0 {
		d.compress(p[:full])
		p = p[full:]
	}
	d.nLast = copy(d.last[:], p)
	return n, nil
}

// Sum appends the 64-byte digest to in without changing d (EdonRFinal,
// edonr.c:257-307, on a copy).
func (d *Digest) Sum(in []byte) []byte {
	c := *d
	numBits := (c.nBytes + uint64(c.nLast)) * 8
	// Pad byte right after the data: with a whole number of bytes the C
	// formula (edonr.c:266-269) has PadOnePosition = 7, i.e. 0x01 << 7.
	c.last[c.nLast] = 0x80
	var blk [2 * BlockSize]byte
	copy(blk[:], c.last[:c.nLast+1])
	// C: unprocessed_bits < 960 means nLast < 120 -> one block, else two.
	if c.nLast < 120 {
		binary.LittleEndian.PutUint64(blk[BlockSize-8:], numBits)
		c.compress(blk[:BlockSize])
	} else {
		binary.LittleEndian.PutUint64(blk[2*BlockSize-8:], numBits)
		c.compress(blk[:])
	}
	for i := 0; i < Size/8; i++ {
		in = binary.LittleEndian.AppendUint64(in, c.p[8+i])
	}
	return in
}

func rotl(x uint64, n int) uint64 { return bits.RotateLeft64(x, n) }

// step is LS1_512(x...); LS2_512(y...); QEF_512(r...) of edonr.c:71-111 fused
// into one function, so the 24 temporaries stay in registers.
func step(x, y *[8]uint64) (r [8]uint64) {
	x0, x1, x2, x3, x4, x5, x6, x7 := x[0], x[1], x[2], x[3], x[4], x[5], x[6], x[7]
	y0, y1, y2, y3, y4, y5, y6, y7 := y[0], y[1], y[2], y[3], y[4], y[5], y[6], y[7]

	// LS1_512
	z1 := x0 + x4
	z2 := x1 + x7
	z5 := z1 + z2
	s0 := 0xaaaaaaaaaaaaaaaa + z5 + x2
	s1 := rotl(z5+x3, 5)
	s2 := rotl(z5+x6, 15)
	z3 := x2 + x3
	s5 := rotl(z1+z3+x5, 40)
	z4 := x5 + x6
	s6 := rotl(z2+z4+x0, 50)
	z6 := z3 + z4
	s3 := rotl(z6+x7, 22)
	s4 := rotl(z6+x1, 31)
	s7 := rotl(z6+x4, 59)

	// LS2_512
	z1 = y0 + y1
	z2 = y2 + y5
	z6 = z1 + z2
	t0 := ^uint64(0xaaaaaaaaaaaaaaaa) + z6 + y7
	t2 := rotl(z6+y3, 19)
	z3 = y3 + y4
	z5 = z1 + z3
	t1 := rotl(z5+y6, 10)
	t4 := rotl(z5+y5, 36)
	z4 = y6 + y7
	z8 := z3 + z4
	t3 := rotl(z8+y2, 29)
	t7 := rotl(z8+y0, 55)
	z7 := z2 + z4
	t5 := rotl(z7+y4, 44)
	t6 := rotl(z7+y1, 48)

	// QEF_512
	z1 = s0 ^ s4
	z5 = t0 ^ t1
	r[0] = (z1 ^ s1) + (z5 ^ t5)
	z8 = t6 ^ t7
	r[1] = (z1 ^ s7) + (t2 ^ z8)
	z3 = s2 ^ s3
	r[7] = (z3 ^ s5) + (t4 ^ z8)
	z7 = t3 ^ t4
	r[3] = (z3 ^ s4) + (t0 ^ z7)
	z4 = s5 ^ s6
	r[5] = (s3 ^ z4) + (z7 ^ t6)
	z6 = t2 ^ t5
	r[6] = (s2 ^ z4) + (z6 ^ t7)
	z2 = s1 ^ s7
	r[4] = (s0 ^ z2) + (t1 ^ z6)
	r[2] = (z2 ^ s6) + (z5 ^ t3)
	return r
}

// compress is Q512, edonr.c:113-209, over whole 128-byte blocks.
func (d *Digest) compress(data []byte) {
	p := &d.p
	for ; len(data) >= BlockSize; data = data[BlockSize:] {
		var dd [16]uint64
		for j := range dd {
			dd[j] = binary.LittleEndian.Uint64(data[8*j:])
		}
		lo := (*[8]uint64)(dd[0:8])
		hi := (*[8]uint64)(dd[8:16])
		var rev [8]uint64 // d(15)..d(8)
		var rlo [8]uint64 // d(7)..d(0)
		for j := 0; j < 8; j++ {
			rev[j] = dd[15-j]
			rlo[j] = dd[7-j]
		}
		pLo := (*[8]uint64)(p[0:8])
		pHi := (*[8]uint64)(p[8:16])

		// First row of quasigroup e-transformations
		a := step(&rev, lo)
		b := step(&a, hi)
		// Second row
		a = step(pHi, &a)
		b = step(&a, &b)
		// Third row
		a = step(&a, pLo)
		b = step(&b, &a)
		// Fourth row
		a = step(&rlo, &a)
		b = step(&a, &b)

		// Edon-R tweak on the original SHA-3 submission.
		for j := 0; j < 8; j++ {
			p[j] ^= dd[8+j] ^ a[j]
			p[8+j] ^= dd[j] ^ b[j]
		}
		d.nBytes += BlockSize
	}
}
