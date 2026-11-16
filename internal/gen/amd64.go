package main

import "fmt"

func ptrRegs(n int) (r []string) {
	for i := 0; i < n; i++ {
		r = append(r, fmt.Sprintf("R%d", 8+i))
	}
	return
}

func amd64Prologue(lanes int, consts func(imm, reg string) []string) func() []string {
	return func() []string {
		l := []string{"MOVQ s+0(FP), DI", "MOVQ p+8(FP), BX", "MOVQ n+16(FP), DX", "MOVQ scratch+24(FP), SI"}
		for i, r := range ptrRegs(lanes) {
			l = append(l, fmt.Sprintf("MOVQ %d(BX), %s", 8*i, r))
		}
		l = append(l, consts("0xAAAAAAAAAAAAAAAA", "X")...)
		l = append(l, consts("0x5555555555555555", "Y")...)
		return l
	}
}

func amd64Advance(lanes int) func() []string {
	return func() (l []string) {
		for _, r := range ptrRegs(lanes) {
			l = append(l, fmt.Sprintf("ADDQ $128, %s", r))
		}
		return append(l, "DECQ DX", "JNZ loop", "VZEROUPPER", "RET")
	}
}

// avx2 is four messages at a time. A rotation is two shifts and an or through
// the temporary Y13; the constants are Y14 and Y15.
func avx2() *isa {
	y := func(r int) string { return fmt.Sprintf("Y%d", r) }
	return &isa{
		name: "avx2", build: "//go:build amd64 && !purego", nregs: 13, vb: 32, lanes: 4,
		state: "DI", scratch: "SI", fn: "compress4", frame: "$0-32",
		sig: "// func compress4(s *state2, p *[4]*byte, n int, scratch *[avx2Scratch]byte)",
		ld:  func(base string, off, r int) string { return fmt.Sprintf("VMOVDQU %d(%s), Y%d", off, base, r) },
		st:  func(r int, base string, off int) string { return fmt.Sprintf("VMOVDQU Y%d, %d(%s)", r, off, base) },
		op: func(k kind, d int, s []int, n int) []string {
			switch k {
			case kAdd:
				return []string{fmt.Sprintf("VPADDQ %s, %s, %s", y(s[1]), y(s[0]), y(d))}
			case kAddC:
				return []string{fmt.Sprintf("VPADDQ Y%d, %s, %s", 14+n, y(s[0]), y(d))}
			case kXor:
				return []string{fmt.Sprintf("VPXOR %s, %s, %s", y(s[1]), y(s[0]), y(d))}
			case kXor3:
				return []string{fmt.Sprintf("VPXOR %s, %s, Y13", y(s[1]), y(s[0])), fmt.Sprintf("VPXOR %s, Y13, %s", y(s[2]), y(d))}
			case kRot:
				return []string{fmt.Sprintf("VPSLLQ $%d, %s, Y13", n, y(s[0])), fmt.Sprintf("VPSRLQ $%d, %s, %s", 64-n, y(s[0]), y(d)), fmt.Sprintf("VPOR Y13, %s, %s", y(d), y(d))}
			}
			panic("avx2 op")
		},
		prologue: amd64Prologue(4, func(imm, c string) []string {
			reg := map[string]string{"X": "14", "Y": "15"}[c]
			return []string{"MOVQ $" + imm + ", AX", "VMOVQ AX, X" + reg, "VPBROADCASTQ X" + reg + ", Y" + reg}
		}),
		stage: func() (l []string) {
			p := ptrRegs(4)
			for g := 0; g < 4; g++ {
				for i := 0; i < 4; i++ {
					l = append(l, fmt.Sprintf("VMOVDQU %d(%s), Y%d", 32*g, p[i], i))
				}
				l = append(l, "VPUNPCKLQDQ Y1, Y0, Y4", "VPUNPCKHQDQ Y1, Y0, Y5", "VPUNPCKLQDQ Y3, Y2, Y6", "VPUNPCKHQDQ Y3, Y2, Y7",
					"VPERM2I128 $0x20, Y6, Y4, Y8", "VPERM2I128 $0x20, Y7, Y5, Y10", "VPERM2I128 $0x31, Y6, Y4, Y9", "VPERM2I128 $0x31, Y7, Y5, Y11")
				// columns: Y8 = word 4g, Y10 = 4g+1, Y9 = 4g+2, Y11 = 4g+3
				for j, c := range []int{8, 10, 9, 11} {
					l = append(l, fmt.Sprintf("VMOVDQU Y%d, %d(SI)", c, 32*(4*g+j)))
				}
			}
			return
		},
		advance: amd64Advance(4),
	}
}

// avx512 is eight messages at a time: VPROLQ rotates, and VPTERNLOGQ is the
// three-input xor. The constants are Z30 and Z31.
func avx512() *isa {
	z := func(r int) string { return fmt.Sprintf("Z%d", r) }
	return &isa{
		name: "avx512", build: "//go:build amd64 && !purego", nregs: 30, vb: 64, lanes: 8,
		state: "DI", scratch: "SI", fn: "compress8", frame: "$0-32",
		sig: "// func compress8(s *state2, p *[8]*byte, n int, scratch *[avx512Scratch]byte)",
		ld:  func(base string, off, r int) string { return fmt.Sprintf("VMOVDQU64 %d(%s), Z%d", off, base, r) },
		st:  func(r int, base string, off int) string { return fmt.Sprintf("VMOVDQU64 Z%d, %d(%s)", r, off, base) },
		op: func(k kind, d int, s []int, n int) []string {
			switch k {
			case kAdd:
				return []string{fmt.Sprintf("VPADDQ %s, %s, %s", z(s[1]), z(s[0]), z(d))}
			case kAddC:
				return []string{fmt.Sprintf("VPADDQ Z%d, %s, %s", 30+n, z(s[0]), z(d))}
			case kXor:
				return []string{fmt.Sprintf("VPXORQ %s, %s, %s", z(s[1]), z(s[0]), z(d))}
			case kXor3:
				// d = d ^ b ^ c: d must hold one operand
				a, b, c := s[0], s[1], s[2]
				var l []string
				switch d {
				case b:
					a, b = b, a
				case c:
					a, c = c, a
				}
				if d != a {
					l = append(l, fmt.Sprintf("VMOVDQA64 %s, %s", z(a), z(d)))
				}
				return append(l, fmt.Sprintf("VPTERNLOGQ $0x96, %s, %s, %s", z(c), z(b), z(d)))
			case kRot:
				return []string{fmt.Sprintf("VPROLQ $%d, %s, %s", n, z(s[0]), z(d))}
			}
			panic("avx512 op")
		},
		prologue: amd64Prologue(8, func(imm, c string) []string {
			reg := map[string]string{"X": "30", "Y": "31"}[c]
			return []string{"MOVQ $" + imm + ", AX", "VPBROADCASTQ AX, Z" + reg}
		}),
		stage: func() (l []string) {
			p := ptrRegs(8)
			for h := 0; h < 2; h++ {
				for i := 0; i < 8; i++ {
					l = append(l, fmt.Sprintf("VMOVDQU64 %d(%s), Z%d", 64*h, p[i], i))
				}
				l = append(l,
					"VPUNPCKLQDQ Z1, Z0, Z8", "VPUNPCKHQDQ Z1, Z0, Z9",
					"VPUNPCKLQDQ Z3, Z2, Z10", "VPUNPCKHQDQ Z3, Z2, Z11",
					"VPUNPCKLQDQ Z5, Z4, Z12", "VPUNPCKHQDQ Z5, Z4, Z13",
					"VPUNPCKLQDQ Z7, Z6, Z14", "VPUNPCKHQDQ Z7, Z6, Z15",
					"VSHUFI64X2 $0x88, Z10, Z8, Z16", "VSHUFI64X2 $0xDD, Z10, Z8, Z17",
					"VSHUFI64X2 $0x88, Z14, Z12, Z18", "VSHUFI64X2 $0xDD, Z14, Z12, Z19",
					"VSHUFI64X2 $0x88, Z11, Z9, Z20", "VSHUFI64X2 $0xDD, Z11, Z9, Z21",
					"VSHUFI64X2 $0x88, Z15, Z13, Z22", "VSHUFI64X2 $0xDD, Z15, Z13, Z23",
					// words 0..7 of this half
					"VSHUFI64X2 $0x88, Z18, Z16, Z0", "VSHUFI64X2 $0xDD, Z18, Z16, Z4",
					"VSHUFI64X2 $0x88, Z19, Z17, Z2", "VSHUFI64X2 $0xDD, Z19, Z17, Z6",
					"VSHUFI64X2 $0x88, Z22, Z20, Z1", "VSHUFI64X2 $0xDD, Z22, Z20, Z5",
					"VSHUFI64X2 $0x88, Z23, Z21, Z3", "VSHUFI64X2 $0xDD, Z23, Z21, Z7")
				for w := 0; w < 8; w++ {
					l = append(l, fmt.Sprintf("VMOVDQU64 Z%d, %d(SI)", w, 64*(8*h+w)))
				}
			}
			return
		},
		advance: amd64Advance(8),
	}
}
