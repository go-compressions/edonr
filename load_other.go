//go:build !(386 || amd64 || arm64 || loong64 || ppc64le || wasm)

package edonr

import "encoding/binary"

// words decodes the 16 little-endian words of blk.
func words(blk []byte) (w [16]uint64) {
	_ = blk[BlockSize-1]
	for i := range w {
		w[i] = binary.LittleEndian.Uint64(blk[8*i:])
	}
	return
}
