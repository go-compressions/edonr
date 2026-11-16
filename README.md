# edonr

Pure-Go Edon-R 512, as used by the ZFS `edonr` checksum. It is a port of
OpenZFS zfs-2.3.0 `module/icp/algs/edonr/edonr.c` (Jorn Amundsen, Saso
Kiselkov, Tino Reichardt; CDDL-1.0, see `LICENSE`), byte-oriented and
little-endian only.

```go
d := edonr.NewKeyed(salt) // salt: the 32-byte org.illumos:checksum_salt
d.Write(block)
sum := d.Sum(nil)[:32]    // the zio_cksum_t
```

`NewKeyed` reproduces `abd_checksum_edonr_tmpl_init` (edonr_zfs.c:78-104):
the state after absorbing `H(salt) || H(H(salt))` is the key. `New`/`Sum` are
the plain hash. `Clone` copies a state, the analogue of the ZFS context
template copy.

## Verification

`testdata/openzfs-2.3.0-vectors.txt` holds 546 vectors (message lengths
0..260 plus 383..65536; unkeyed 512-bit digest and salted 256-bit checksum)
produced by compiling the OpenZFS C sources verbatim behind a small shim;
every line is checked by `go test`.

## Performance

Single core, Apple M4 Max, keyed, from a cloned template, OpenZFS C compiled
with `clang -O2`: C 3.5-3.7 GB/s, this package 1.2-1.7 GB/s. The gap is open
(tracked in an issue); the checksum is verified on read only.
