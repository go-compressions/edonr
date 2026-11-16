package main

import "fmt"

// The compression function is built once as a straight-line program over
// 64-bit values (an SSA list); the scalar Go file and the SIMD assembly files
// are printed from it, so every backend computes the same dataflow.

type kind int

const (
	kLoadM   kind = iota // message word idx
	kLoadS               // state word idx
	kAdd                 // a + b
	kAddC                // a + constant (idx 0: constX, 1: constY)
	kXor                 // a ^ b
	kXor3                // a ^ b ^ c
	kRot                 // a rotated left by k
	kStoreS              // state word idx = a
	kBarrier             // loads after it are not shared with loads before it
)

type instr struct {
	k       kind
	a, b, c int // operand value numbers
	n       int // idx or rotation
}

// prog is a list of instructions; the value an instruction defines is its index.
type prog struct{ ins []instr }

func (g *prog) add(i instr) int { g.ins = append(g.ins, i); return len(g.ins) - 1 }

func (g *prog) loadM(i int) int      { return g.add(instr{k: kLoadM, n: i}) }
func (g *prog) loadS(i int) int      { return g.add(instr{k: kLoadS, n: i}) }
func (g *prog) sum(a, b int) int     { return g.add(instr{k: kAdd, a: a, b: b}) }
func (g *prog) addc(a, c int) int    { return g.add(instr{k: kAddC, a: a, n: c}) }
func (g *prog) xor(a, b int) int     { return g.add(instr{k: kXor, a: a, b: b}) }
func (g *prog) xor3(a, b, c int) int { return g.add(instr{k: kXor3, a: a, b: b, c: c}) }
func (g *prog) rot(a, k int) int     { return g.add(instr{k: kRot, a: a, n: k}) }
func (g *prog) storeS(i, a int)      { g.add(instr{k: kStoreS, a: a, n: i}) }
func (g *prog) barrier()             { g.add(instr{k: kBarrier}) }

type cachedPlan struct {
	steps []step
	res   []int
}

var plans = map[string]cachedPlan{}

// linear computes the n-ary sums or xors named by sets (indices into vals)
// with shared partial results. vec chooses the three-input xor of the SIMD
// units for 3-element xor sets instead of a shared two-input plan.
func (g *prog) linear(vals []int, isXor bool, sets [][]int, vec bool) []int {
	if isXor && vec {
		out := make([]int, len(sets))
		for i, s := range sets {
			out[i] = g.xor3(vals[s[0]], vals[s[1]], vals[s[2]])
		}
		return out
	}
	key := fmt.Sprint(len(vals), sets)
	pl, ok := plans[key]
	if !ok {
		s, r := bestPlan(len(vals), sets)
		pl = cachedPlan{s, r}
		plans[key] = pl
	}
	steps, res := pl.steps, pl.res
	nodes := append([]int(nil), vals...)
	for _, st := range steps {
		if isXor {
			nodes = append(nodes, g.xor(nodes[st.a], nodes[st.b]))
		} else {
			nodes = append(nodes, g.sum(nodes[st.a], nodes[st.b]))
		}
	}
	out := make([]int, len(res))
	for i, r := range res {
		out[i] = nodes[r]
	}
	return out
}

// quasigroup appends Z = X * Y (Table 2.2).
func (g *prog) quasigroup(x, y [8]int, vec bool) (z [8]int) {
	xs := [][]int{{0, 1, 2, 4, 7}, {0, 1, 3, 4, 7}, {0, 1, 4, 6, 7}, {2, 3, 5, 6, 7}, {1, 2, 3, 5, 6}, {0, 2, 3, 4, 5}, {0, 1, 5, 6, 7}, {2, 3, 4, 5, 6}}
	rx := []int{0, 5, 15, 22, 31, 40, 50, 59}
	ys := [][]int{{0, 1, 2, 5, 7}, {0, 1, 3, 4, 6}, {0, 1, 2, 3, 5}, {2, 3, 4, 6, 7}, {0, 1, 3, 4, 5}, {2, 4, 5, 6, 7}, {1, 2, 5, 6, 7}, {0, 3, 4, 6, 7}}
	ry := []int{0, 10, 19, 29, 36, 44, 48, 55}
	side := func(v [8]int, sets [][]int, rots []int, c int) []int {
		sv := g.linear(v[:], false, sets, vec)
		t := make([]int, 8)
		for i := range t {
			if i == 0 {
				t[i] = g.addc(sv[i], c)
			} else {
				t[i] = g.rot(sv[i], rots[i])
			}
		}
		return t
	}
	t := side(x, xs, rx, 0)
	u := side(y, ys, ry, 1)
	x3 := [][]int{{3, 5, 6}, {2, 5, 6}, {2, 3, 5}, {0, 1, 4}, {0, 4, 7}, {1, 6, 7}, {2, 3, 4}, {0, 1, 7}}
	zo := []int{5, 6, 7, 0, 1, 2, 3, 4}
	za := [][]int{{3, 4, 6}, {2, 5, 7}, {4, 6, 7}, {0, 1, 5}, {2, 6, 7}, {0, 1, 3}, {0, 3, 4}, {1, 2, 5}}
	tx := g.linear(t, true, x3, vec)
	ux := g.linear(u, true, za, vec)
	for k := range zo {
		z[zo[k]] = g.sum(tx[k], ux[k])
	}
	return
}

func rev(v [8]int) (r [8]int) {
	for i := range r {
		r[i] = v[7-i]
	}
	return
}

// buildCompress builds P = R(P, M) xor P xor swapped(M) for one block
// (Definition 8 expanded into eight quasigroup operations). Message and state
// words are loaded where they are used, and loaded again later rather than
// kept: registers are the scarce resource.
func buildCompress(vec bool) *prog {
	g := &prog{}
	loadM := func(lo int) (r [8]int) {
		g.barrier()
		for i := range r {
			r[i] = g.loadM(lo + i)
		}
		return
	}
	loadS := func(lo int) (r [8]int) {
		g.barrier()
		for i := range r {
			r[i] = g.loadS(lo + i)
		}
		return
	}
	a0, a1 := loadM(0), loadM(8)
	xa := g.quasigroup(rev(a1), a0, vec)
	a1 = loadM(8)
	xb := g.quasigroup(xa, a1, vec)
	xc := g.quasigroup(loadS(8), xa, vec)
	xd := g.quasigroup(xc, xb, vec)
	xe := g.quasigroup(xc, loadS(0), vec)
	xf := g.quasigroup(xd, xe, vec)
	a0 = loadM(0)
	ba := g.quasigroup(rev(a0), xe, vec)
	bb := g.quasigroup(ba, xf, vec)
	a0, a1 = loadM(0), loadM(8)
	s0, s1 := loadS(0), loadS(8)
	var o0, o1 [8]int
	for i := 0; i < 8; i++ {
		o0[i] = g.xor3(ba[i], s0[i], a1[i])
		o1[i] = g.xor3(bb[i], s1[i], a0[i])
	}
	for i := 0; i < 8; i++ {
		g.storeS(i, o0[i])
		g.storeS(8+i, o1[i])
	}
	return g
}

func vname(v int) string { return fmt.Sprintf("v%d", v) }
