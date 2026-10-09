package main

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Společné jádro výpočtů pro okno v prohlížeči (gui.go, místní server) i pro webovou stránku
// (wasm_js.go, výpočet přímo v prohlížeči): sestavení slovníku a matice společného výskytu.

type apiRequest struct {
	Text       string `json:"text"`
	Algo       string `json:"algo"`
	Size       int    `json:"size"`
	IgnoreCase bool   `json:"ignoreCase"`
}

type apiMerge struct {
	A      string `json:"a"`
	B      string `json:"b"`
	New    string `json:"new"`
	Count  int    `json:"count"`
	CountA int    `json:"countA"`
	CountB int    `json:"countB"`
	Dup    bool   `json:"dup,omitempty"`
}

type apiResponse struct {
	Error     string     `json:"error,omitempty"` // "nowords", "toosmall" nebo text chyby
	Chars     int        `json:"chars,omitempty"`
	Initial   []string   `json:"initial,omitempty"`
	Tokens    []string   `json:"tokens,omitempty"`
	Merges    []apiMerge `json:"merges,omitempty"`
	Exhausted bool       `json:"exhausted,omitempty"`
	Words     int        `json:"words,omitempty"`
	Total     int        `json:"total,omitempty"`
	Ms        int64      `json:"ms"`
}

type matrixRequest struct {
	Algo  string `json:"algo"`
	Delta int    `json:"delta"`
}

type matrixResponse struct {
	Error      string `json:"error,omitempty"`
	Size       int    `json:"size,omitempty"`       // počet tokenů ve slovníku = rozměr matice
	Delta      int    `json:"delta,omitempty"`      // Δ
	TextTokens int    `json:"textTokens,omitempty"` // počet tokenů v textu
	Nonzero    int    `json:"nonzero,omitempty"`    // počet nenulových buněk
	Total      uint64 `json:"total,omitempty"`      // součet všech buněk
	Ms         int64  `json:"ms"`
}

type matrixViewToken struct {
	N     int    `json:"n"` // číslo tokenu ve slovníku (od 1)
	Token string `json:"token"`
}

type matrixViewResponse struct {
	Error  string            `json:"error,omitempty"`
	Tokens []matrixViewToken `json:"tokens,omitempty"`
	Cells  [][]uint32        `json:"cells,omitempty"`
	Size   int               `json:"size,omitempty"`
}

// trainedVocab je poslední úspěšně vytvořený slovník s textem, ze kterého vznikl
// (potřebuje ho matice společného výskytu).
type trainedVocab struct {
	corpus *Corpus
	res    *Result
}

// engine drží poslední hotové slovníky a matici.
type engine struct {
	mu      sync.Mutex
	trained map[Algorithm]*trainedVocab // poslední hotový slovník každého algoritmu
	matrix  *Matrix                     // naposledy vytvořená matice společného výskytu
	matrixA Algorithm
}

func newEngine() *engine { return &engine{trained: map[Algorithm]*trainedVocab{}} }

func (e *engine) Train(req apiRequest) apiResponse {
	alg, ok := ParseAlgorithm(req.Algo)
	if !ok {
		return apiResponse{Error: "Neznámý algoritmus."}
	}
	if req.Size < 1 {
		return apiResponse{Error: "Velikost slovníku musí být kladné číslo."}
	}
	t0 := time.Now()
	corpus, err := BuildCorpusOrdered(strings.NewReader(req.Text), alg, req.IgnoreCase)
	if err != nil {
		return apiResponse{Error: err.Error()}
	}
	if len(corpus.Words) == 0 {
		return apiResponse{Error: "nowords"}
	}
	res, err := Train(corpus, alg, req.Size)
	if err != nil {
		if te, ok := err.(ErrTooSmall); ok {
			return apiResponse{Error: "toosmall", Chars: te.Chars}
		}
		return apiResponse{Error: err.Error()}
	}
	e.mu.Lock()
	e.trained[alg] = &trainedVocab{corpus: corpus, res: res}
	e.mu.Unlock()
	out := apiResponse{
		Initial:   res.Initial,
		Tokens:    res.Tokens,
		Exhausted: res.Exhausted,
		Words:     len(corpus.Words),
		Total:     corpus.Total,
		Ms:        time.Since(t0).Milliseconds(),
	}
	out.Merges = make([]apiMerge, len(res.Merges))
	for i, m := range res.Merges {
		out.Merges[i] = apiMerge{A: m.A, B: m.B, New: m.New, Count: m.Count, CountA: m.CountA, CountB: m.CountB, Dup: m.Dup}
	}
	return out
}

// Matrix vytvoří matici společného výskytu ze slovníku, který se naposledy vytvořil
// zadaným algoritmem, a uloží ji pro zobrazení a stažení.
func (e *engine) Matrix(req matrixRequest) matrixResponse {
	alg, ok := ParseAlgorithm(req.Algo)
	if !ok {
		return matrixResponse{Error: "Neznámý algoritmus."}
	}
	e.mu.Lock()
	tv := e.trained[alg]
	e.mu.Unlock()
	if tv == nil {
		return matrixResponse{Error: "Slovník ještě není vytvořený. Vytvořte ho na kartě Tokenizace."}
	}
	t0 := time.Now()
	seq, err := TokenSequence(tv.corpus, tv.res)
	if err != nil {
		return matrixResponse{Error: err.Error()}
	}
	m, err := BuildMatrix(seq, tv.res.Tokens, req.Delta)
	if err != nil {
		return matrixResponse{Error: err.Error()}
	}
	e.mu.Lock()
	e.matrix, e.matrixA = m, alg
	e.mu.Unlock()
	return matrixResponse{
		Size: len(m.Tokens), Delta: m.Delta, TextTokens: m.TextTokens,
		Nonzero: m.Nonzero, Total: m.Total, Ms: time.Since(t0).Milliseconds(),
	}
}

// MatrixView vrátí čtvercový výřez matice: k tokenů (nejvýše 100), buď prvních podle čísla
// (byFreq == false), nebo nejčastějších.
func (e *engine) MatrixView(k int, byFreq bool) matrixViewResponse {
	e.mu.Lock()
	m := e.matrix
	e.mu.Unlock()
	if m == nil {
		return matrixViewResponse{Error: "Matice ještě není vytvořená."}
	}
	if k < 1 {
		k = 20
	}
	if k > 100 {
		k = 100
	}
	idx := m.Pick(k, byFreq)
	v := len(m.Tokens)
	out := matrixViewResponse{Size: v}
	for _, i := range idx {
		out.Tokens = append(out.Tokens, matrixViewToken{N: i + 1, Token: m.Tokens[i]})
	}
	for _, i := range idx {
		row := make([]uint32, len(idx))
		for j, c := range idx {
			row[j] = m.Cells[i*v+c]
		}
		out.Cells = append(out.Cells, row)
	}
	return out
}

// MatrixFile vrátí naposledy vytvořenou matici a algoritmus, ze kterého vznikla (nebo nil).
func (e *engine) MatrixFile() (*Matrix, Algorithm) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.matrix, e.matrixA
}

func matrixFileName(alg Algorithm, m *Matrix) string {
	return fmt.Sprintf("matice-%s-delta%d.tsv", alg, m.Delta)
}
