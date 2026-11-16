# edonr

A pure-Go implementation of the **Edon-R′ 512** hash function, the function
behind the OpenZFS `checksum=edonr` property, with the keyed (salted) form ZFS
computes for a pool that has a checksum salt.

BSD-3-Clause. Since v0.2.0 this is a **clean-room** implementation written from
the published specification; v0.1.0 was a port of OpenZFS's CDDL-1.0 code and no
line of it survives (see Provenance).

```go
sum := edonr.Sum(data)              // [64]byte, Edon-R 512

d := edonr.New()                    // streaming, implements hash.Hash
d.Write(block)
digest := d.Sum(nil)

k := edonr.NewKeyed(poolSalt[:])    // ZFS salted checksum
k.Write(block)
blkCksum := k.Sum(nil)[:32]         // ZFS stores the first 32 bytes
```

`Size` is 64, `BlockSize` is 128. The API is that of v0.1.0.

## Which function this is

The 2008 NIST submission ("Edon-R") and the 2009 tweak "Edon-R′" differ by a
feedback term. The OpenZFS output is Edon-R′: the compression step is
`P(i) = R(P(i-1), M(i)) xor P(i-1) xor swapped(M(i))`. This was not known in
advance; the unkeyed digests of the submission document do not match OpenZFS,
the feedback form matches on every vector (273 unkeyed lengths up to 64 KiB, and
millions of fuzzed inputs).

The keyed form is not part of the specification. Black-box queries show that the
digest for a salt `s` and message `m` is the unkeyed digest of
`H(s) || H(H(s)) || m`, where `H` is the unkeyed 64-byte digest (the two digests
fill exactly one block, so a keyed state is one precomputed compression).

## Provenance (clean room)

Written from these documents only; the section numbers are those cited in
`edonr.go`:

- D. Gligoroski, R. S. Odegard, M. Mihova, S. J. Knapskog, L. Kocarev, A. Drapal,
  *Cryptographic Hash Function EDON-R*, supporting documentation, NTNU, October
  2008 (retrieved from the NTNU server through the Internet Archive). Sections
  1.1, 1.4.1, 1.4.3 (Table 1.6), 2.1.1 (Table 2.2), 2.1.3 (Definitions 7 and 8,
  Table 2.5), 2.2 (Table 2.6, Figure 2.1).
- The revised version of the same documentation (chapter "Paper B" of the NTNU
  thesis, UKIM repository 524494_FULLTEXT01.pdf), used to cross-check the
  tables.
- D. Gligoroski et al., *Cryptographic hash function EDON-R′*, 2009 (UKIM
  repository, IEEE 05683046), Table 6: the feedback.

The OpenZFS 2.3.0 implementation was used only as a black box: a compiled
oracle binary that prints a digest for a given input (and salt). No source of
OpenZFS, of v0.1.0 of this module, or of any other Edon-R implementation was
read while writing this code. The oracle settled what the specification leaves
open (which Edon-R variant ZFS uses, which half of the pipe is the digest, the
arrangement of the halves entering R, and the keyed construction).

## Tests

`testdata/openzfs-2.3.0-vectors.txt` holds the oracle's output for lengths 0 to
260, around every block boundary and up to 64 KiB, unkeyed and salted. With the
environment variable `EDONR_ORACLE` set to the oracle binary, `go test` also
runs a differential test over lengths up to 5 MiB with random messages and random
salts, and `FuzzOracle` fuzzes against it. Statement coverage is 100 %.

## Performance

Plain Go, eight quasigroup operations per block expanded in straight-line
generated code, no allocation: about 2.5 GB/s on one Apple M4 Max core (4 KiB
and 128 KiB blocks), measured under a load of about 7. The OpenZFS C code
measured 1.4 to 3.1 GB/s in the same noisy window; a clean comparison and a
SIMD multi-buffer kernel are open work.

## License

BSD 3-Clause, see `LICENSE`.
