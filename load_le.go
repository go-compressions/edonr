//go:build 386 || amd64 || arm64 || loong64 || ppc64le || wasm

package edonr

import "unsafe"

// words returns the 16 little-endian words of blk. On these little-endian
// architectures that tolerate unaligned loads it is a reinterpretation, with no
// copy and no per-word call: compress is too large for the compiler to inline
// even encoding/binary's accessors (callee budget 20 in a "big" function), and
// one call per word would force every live value onto the stack.
func words(blk []byte) *[16]uint64 {
	_ = blk[BlockSize-1]
	return (*[16]uint64)(unsafe.Pointer(&blk[0]))
}
