package main

import (
	"container/heap"
	"math/bits"
	"sort"
)

// Merge je jedno sloučení: dvojice A + B se nahradí novým symbolem New.
type Merge struct {
	A, B, New string
	Count     int // #(AB): kolikrát se dvojice vyskytuje vedle sebe
	CountA    int // #(A) a #(B) v okamžiku výběru (jen WordPiece a SentencePiece)
	CountB    int
}

// Result je slovník a postup jeho vzniku.
type Result struct {
	Initial   []string // počáteční slovník: jednotlivé znaky (seřazené)
	Tokens    []string // celý slovník: počáteční znaky a pak nové symboly v pořadí vzniku
	Merges    []Merge
	Exhausted bool // slovník nelze dále zvětšit
}

// ErrTooSmall: požadovaná velikost je menší než počet jednotlivých znaků.
type ErrTooSmall struct{ Chars int }

func (e ErrTooSmall) Error() string {
	return "požadovaná velikost je menší než počet jednotlivých znaků"
}

var _ error = ErrTooSmall{}

// InitialSymbols vrátí seřazené různé znaky korpusu.
func InitialSymbols(c *Corpus) []rune {
	seen := map[rune]struct{}{}
	for _, w := range c.Words {
		for _, r := range w {
			seen[r] = struct{}{}
		}
	}
	out := make([]rune, 0, len(seen))
	for r := range seen {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func pairKey(a, b int32) uint64 { return uint64(uint32(a))<<32 | uint64(uint32(b)) }
func pairParts(p uint64) (int32, int32) {
	return int32(uint32(p >> 32)), int32(uint32(p))
}

// ---------- haldy ----------

// intHeap je minimální halda čísel slov (líně se čistí od zastaralých položek).
type intHeap []int32

func (h *intHeap) push(v int32) {
	*h = append(*h, v)
	a := *h
	i := len(a) - 1
	for i > 0 {
		p := (i - 1) / 2
		if a[p] <= a[i] {
			break
		}
		a[p], a[i] = a[i], a[p]
		i = p
	}
}

func (h *intHeap) pop() int32 {
	a := *h
	top := a[0]
	n := len(a) - 1
	a[0] = a[n]
	a = a[:n]
	i := 0
	for {
		l, r, m := 2*i+1, 2*i+2, i
		if l < n && a[l] < a[m] {
			m = l
		}
		if r < n && a[r] < a[m] {
			m = r
		}
		if m == i {
			break
		}
		a[i], a[m] = a[m], a[i]
		i = m
	}
	*h = a
	return top
}

// entry je položka hlavní haldy: dvojice se skóre c/den a nejmenším číslem slova, které ji obsahuje.
type entry struct {
	c    uint64
	den  uint64
	mw   int32
	pair uint64
}

// scoreCmp porovná skóre c1/den1 a c2/den2 přesně (128bitovým násobením, bez zaokrouhlení).
func scoreCmp(c1, d1, c2, d2 uint64) int {
	h1, l1 := bits.Mul64(c1, d2)
	h2, l2 := bits.Mul64(c2, d1)
	switch {
	case h1 != h2:
		if h1 > h2 {
			return 1
		}
		return -1
	case l1 != l2:
		if l1 > l2 {
			return 1
		}
		return -1
	}
	return 0
}

type entryHeap []entry

func (h entryHeap) Len() int { return len(h) }
func (h entryHeap) Less(i, j int) bool {
	if s := scoreCmp(h[i].c, h[i].den, h[j].c, h[j].den); s != 0 {
		return s > 0 // větší skóre dřív
	}
	if h[i].mw != h[j].mw {
		return h[i].mw < h[j].mw // dřív se vyskytující slovo
	}
	return h[i].pair < h[j].pair
}
func (h entryHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *entryHeap) Push(x any)   { *h = append(*h, x.(entry)) }
func (h *entryHeap) Pop() any {
	a := *h
	x := a[len(a)-1]
	*h = a[:len(a)-1]
	return x
}

// ---------- trénink ----------

type pairInfo struct {
	count    int
	words    intHeap // slova, která dvojici obsahují (zastaralá čísla se mažou líně)
	lastWord int32   // naposledy přidané slovo (proti zbytečným duplicitám)
	// naposledy odeslaná hodnota v hlavní haldě
	pushed   bool
	pc, pden uint64
	pmw      int32
}

type trainer struct {
	alg   Algorithm
	ratio bool

	symStr  []string
	symID   map[string]int32
	symCnt  []int // #(A): počet výskytů symbolu v aktuálním rozkladu (s vahami)
	seqs    [][]int32
	weights []int

	pairs    map[uint64]*pairInfo
	symPairs map[int32]map[uint64]struct{} // jen pro poměr: dvojice obsahující daný symbol
	main     entryHeap
}

func (t *trainer) intern(s string) (int32, bool) {
	if id, ok := t.symID[s]; ok {
		return id, true
	}
	id := int32(len(t.symStr))
	t.symStr = append(t.symStr, s)
	t.symCnt = append(t.symCnt, 0)
	t.symID[s] = id
	return id, false
}

func hasPair(seq []int32, a, b int32) bool {
	for j := 0; j+1 < len(seq); j++ {
		if seq[j] == a && seq[j+1] == b {
			return true
		}
	}
	return false
}

// minWord vrátí nejmenší číslo slova, které dvojici skutečně obsahuje (nebo -1).
func (t *trainer) minWord(p uint64, info *pairInfo) int32 {
	a, b := pairParts(p)
	for len(info.words) > 0 {
		w := info.words[0]
		if hasPair(t.seqs[w], a, b) {
			return w
		}
		info.words.pop()
	}
	return -1
}

func (t *trainer) den(p uint64) uint64 {
	if !t.ratio {
		return 1
	}
	a, b := pairParts(p)
	return uint64(t.symCnt[a]) * uint64(t.symCnt[b])
}

func (t *trainer) addPair(p uint64, w int, word int32) {
	info := t.pairs[p]
	if info == nil {
		info = &pairInfo{lastWord: -1}
		t.pairs[p] = info
	}
	if info.count == 0 && t.ratio {
		a, b := pairParts(p)
		t.linkSym(a, p)
		t.linkSym(b, p)
	}
	info.count += w
	if info.lastWord != word {
		info.words.push(word)
		info.lastWord = word
	}
}

func (t *trainer) linkSym(s int32, p uint64) {
	m := t.symPairs[s]
	if m == nil {
		m = map[uint64]struct{}{}
		t.symPairs[s] = m
	}
	m[p] = struct{}{}
}

func (t *trainer) subPair(p uint64, w int) {
	info := t.pairs[p]
	info.count -= w
	if info.count == 0 && t.ratio {
		a, b := pairParts(p)
		delete(t.symPairs[a], p)
		delete(t.symPairs[b], p)
	}
}

// refresh pošle do hlavní haldy aktuální hodnotu dvojice, pokud se změnila.
func (t *trainer) refresh(p uint64) {
	info := t.pairs[p]
	if info == nil {
		return
	}
	if info.count <= 0 {
		delete(t.pairs, p)
		return
	}
	mw := t.minWord(p, info)
	c, den := uint64(info.count), t.den(p)
	if info.pushed && info.pc == c && info.pden == den && info.pmw == mw {
		return
	}
	info.pushed, info.pc, info.pden, info.pmw = true, c, den, mw
	heap.Push(&t.main, entry{c: c, den: den, mw: mw, pair: p})
}

func (t *trainer) valid(e entry) bool {
	info := t.pairs[e.pair]
	if info == nil || uint64(info.count) != e.c || t.den(e.pair) != e.den {
		return false
	}
	return t.minWord(e.pair, info) == e.mw
}

// popBest vybere dvojici s největším skóre. Při shodě vyhrává dvojice, která se v aktuálním
// rozkladu vyskytne dřív (nejdřív podle čísla slova, pak podle pozice ve slově).
func (t *trainer) popBest() (uint64, bool) {
	for t.main.Len() > 0 {
		e := heap.Pop(&t.main).(entry)
		if !t.valid(e) {
			continue
		}
		group := []entry{e}
		for t.main.Len() > 0 {
			n := t.main[0]
			if !t.valid(n) {
				heap.Pop(&t.main)
				continue
			}
			if n.mw != e.mw || scoreCmp(n.c, n.den, e.c, e.den) != 0 {
				break
			}
			heap.Pop(&t.main)
			if n.pair != e.pair {
				group = append(group, n)
			}
		}
		if len(group) == 1 {
			return e.pair, true
		}
		inGroup := make(map[uint64]struct{}, len(group))
		for _, g := range group {
			inGroup[g.pair] = struct{}{}
		}
		seq := t.seqs[e.mw]
		var winner uint64
		for j := 0; j+1 < len(seq); j++ {
			p := pairKey(seq[j], seq[j+1])
			if _, ok := inGroup[p]; ok {
				winner = p
				break
			}
		}
		for _, g := range group {
			if g.pair != winner {
				heap.Push(&t.main, g)
			}
		}
		return winner, true
	}
	return 0, false
}

// merge sloučí všechny výskyty dvojice p do nového symbolu x a průběžně opraví četnosti.
func (t *trainer) merge(p uint64, x int32) {
	a, b := pairParts(p)
	info := t.pairs[p]
	dirty := map[uint64]struct{}{}

	var affected []int32
	for len(info.words) > 0 {
		w := info.words.pop()
		if len(affected) == 0 || affected[len(affected)-1] != w {
			affected = append(affected, w)
		}
	}
	for _, wi := range affected {
		old := t.seqs[wi]
		w := t.weights[wi]
		var nw []int32
		var newSites []int
		lastOld := -2
		changed := false
		for j := 0; j < len(old); j++ {
			if j+1 < len(old) && old[j] == a && old[j+1] == b {
				if !changed {
					changed = true
					nw = append(make([]int32, 0, len(old)), old[:j]...)
				}
				// zaniknou dvojice (j-1,j), (j,j+1), (j+1,j+2) ve starém rozkladu
				for i := j - 1; i <= j+1; i++ {
					if i >= 0 && i+1 < len(old) && i > lastOld {
						q := pairKey(old[i], old[i+1])
						t.subPair(q, w)
						dirty[q] = struct{}{}
						lastOld = i
					}
				}
				t.symCnt[a] -= w
				t.symCnt[b] -= w
				t.symCnt[x] += w
				newSites = append(newSites, len(nw))
				nw = append(nw, x)
				j++
			} else if changed {
				nw = append(nw, old[j])
			}
		}
		if !changed {
			continue
		}
		t.seqs[wi] = nw
		lastNew := -2
		for _, k := range newSites {
			for i := k - 1; i <= k; i++ {
				if i >= 0 && i+1 < len(nw) && i > lastNew {
					q := pairKey(nw[i], nw[i+1])
					t.addPair(q, w, wi)
					dirty[q] = struct{}{}
					lastNew = i
				}
			}
		}
	}
	if t.ratio {
		// počty #(A), #(B) a #(AB) se změnily, takže se změnilo skóre všech dvojic s těmito symboly
		for _, s := range []int32{a, b, x} {
			for q := range t.symPairs[s] {
				dirty[q] = struct{}{}
			}
		}
	}
	delete(t.pairs, p)
	for q := range dirty {
		t.refresh(q)
	}
}

// Train sestaví slovník o požadované velikosti (počítá se i počáteční slovník znaků).
func Train(c *Corpus, alg Algorithm, size int) (*Result, error) {
	chars := InitialSymbols(c)
	if size < len(chars) {
		return nil, ErrTooSmall{Chars: len(chars)}
	}
	t := &trainer{
		alg:      alg,
		ratio:    alg.usesRatio(),
		symID:    map[string]int32{},
		pairs:    map[uint64]*pairInfo{},
		symPairs: map[int32]map[uint64]struct{}{},
	}
	res := &Result{}
	for _, r := range chars {
		s := string(r)
		t.intern(s)
		res.Initial = append(res.Initial, s)
		res.Tokens = append(res.Tokens, s)
	}
	t.seqs = make([][]int32, len(c.Words))
	t.weights = c.Freq
	for i, w := range c.Words {
		seq := make([]int32, len(w))
		for j, r := range w {
			id := t.symID[string(r)]
			seq[j] = id
			t.symCnt[id] += c.Freq[i]
		}
		t.seqs[i] = seq
	}
	for i, seq := range t.seqs {
		for j := 0; j+1 < len(seq); j++ {
			t.addPair(pairKey(seq[j], seq[j+1]), c.Freq[i], int32(i))
		}
	}
	initialPairs := make([]uint64, 0, len(t.pairs))
	for p := range t.pairs {
		initialPairs = append(initialPairs, p)
	}
	for _, p := range initialPairs {
		t.refresh(p)
	}

	for len(res.Tokens) < size {
		p, ok := t.popBest()
		if !ok {
			res.Exhausted = true
			break
		}
		a, b := pairParts(p)
		info := t.pairs[p]
		m := Merge{A: t.symStr[a], B: t.symStr[b], Count: info.count}
		if t.ratio {
			m.CountA, m.CountB = t.symCnt[a], t.symCnt[b]
		}
		m.New = m.A + m.B
		x, dup := t.intern(m.New)
		t.merge(p, x)
		if !dup {
			res.Tokens = append(res.Tokens, m.New)
		}
		res.Merges = append(res.Merges, m)
	}
	return res, nil
}
