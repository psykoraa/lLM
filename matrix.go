package main

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strconv"
)

// Matice společného výskytu: M[w][c] říká, kolikrát se token c vyskytl nejvýše o Δ pozic
// před nebo za tokenem w (samotná pozice se nepočítá). Číslo tokenu ve slovníku je číslo
// řádku (w) a sloupce (c).

const (
	maxMatrixCells = 32 << 20 // nejvýše 32 milionů buněk (128 MB), tj. slovník do asi 5 600 tokenů
	maxDelta       = 1000
)

// Matrix je hotová matice (po řádcích) se slovníkem, ze kterého vznikla.
type Matrix struct {
	Tokens     []string
	Delta      int
	Cells      []uint32 // len(Tokens)² buněk, M[w][c] = Cells[w*len(Tokens)+c]
	TextTokens int      // počet tokenů v textu
	Nonzero    int      // počet nenulových buněk
	Total      uint64   // součet všech buněk
	RowSum     []uint64 // součet řádku = kolikrát se token vyskytl v okolí jiných tokenů
}

// TokenSequence rozloží celý text (v původním pořadí) na tokeny podle výsledku tréninku.
func TokenSequence(c *Corpus, res *Result) ([]int32, error) {
	if len(c.Order) == 0 || len(res.Seg) != len(c.Words) {
		return nil, fmt.Errorf("chybí pořadí slov v textu")
	}
	n := 0
	for _, w := range c.Order {
		n += len(res.Seg[w])
	}
	seq := make([]int32, 0, n)
	for _, w := range c.Order {
		seq = append(seq, res.Seg[w]...)
	}
	return seq, nil
}

// BuildMatrix projde posloupnost tokenů a pro každou pozici t zvýší M[w_t][c] pro token c
// na pozicích t−Δ … t−1 a t+1 … t+Δ. Každou dvojici pozic (t, u) stačí projít jednou:
// z pozice t přibude M[w_t][w_u], z pozice u přibude M[w_u][w_t].
func BuildMatrix(seq []int32, tokens []string, delta int) (*Matrix, error) {
	v := len(tokens)
	if delta < 1 || delta > maxDelta {
		return nil, fmt.Errorf("Δ musí být celé číslo od 1 do %d", maxDelta)
	}
	if v == 0 {
		return nil, fmt.Errorf("slovník je prázdný")
	}
	if v*v > maxMatrixCells {
		return nil, fmt.Errorf("slovník má %d tokenů, to je na matici příliš (nejvýše %d)", v, isqrt(maxMatrixCells))
	}
	m := &Matrix{Tokens: tokens, Delta: delta, Cells: make([]uint32, v*v), TextTokens: len(seq)}
	n := len(seq)
	for t := 0; t < n; t++ {
		w := int(seq[t])
		hi := t + delta
		if hi > n-1 {
			hi = n - 1
		}
		for u := t + 1; u <= hi; u++ {
			c := int(seq[u])
			m.Cells[w*v+c]++
			m.Cells[c*v+w]++
		}
	}
	m.RowSum = make([]uint64, v)
	for w := 0; w < v; w++ {
		row := m.Cells[w*v : (w+1)*v]
		for _, x := range row {
			if x != 0 {
				m.Nonzero++
				m.RowSum[w] += uint64(x)
			}
		}
		m.Total += m.RowSum[w]
	}
	return m, nil
}

func isqrt(n int) int {
	r := 0
	for (r+1)*(r+1) <= n {
		r++
	}
	return r
}

// Pick vybere k tokenů k zobrazení (jejich čísla od 0, vzestupně): buď prvních k podle čísla,
// nebo k nejčastějších v okolí jiných tokenů.
func (m *Matrix) Pick(k int, byFreq bool) []int {
	v := len(m.Tokens)
	if k > v {
		k = v
	}
	if k < 0 {
		k = 0
	}
	idx := make([]int, v)
	for i := range idx {
		idx[i] = i
	}
	if byFreq {
		sort.SliceStable(idx, func(a, b int) bool { return m.RowSum[idx[a]] > m.RowSum[idx[b]] })
	}
	idx = idx[:k]
	sort.Ints(idx)
	return idx
}

// WriteTSV zapíše celou matici: první řádek je hlavička s tokeny, každý další řádek začíná
// tokenem a pak následují počty. Pořadí odpovídá číslům tokenů ve slovníku. Oddělovač je
// tabulátor, na začátku je značka UTF-8 pro Excel (stejně jako u ostatních exportů).
func (m *Matrix) WriteTSV(w io.Writer) error {
	bw := bufio.NewWriterSize(w, 1<<20)
	v := len(m.Tokens)
	bw.WriteString("\xef\xbb\xbftoken")
	for _, t := range m.Tokens {
		bw.WriteByte('\t')
		bw.WriteString(t)
	}
	bw.WriteByte('\n')
	buf := make([]byte, 0, 16)
	for i := 0; i < v; i++ {
		bw.WriteString(m.Tokens[i])
		row := m.Cells[i*v : (i+1)*v]
		for _, x := range row {
			bw.WriteByte('\t')
			buf = strconv.AppendUint(buf[:0], uint64(x), 10)
			bw.Write(buf)
		}
		if err := bw.WriteByte('\n'); err != nil {
			return err
		}
	}
	return bw.Flush()
}
