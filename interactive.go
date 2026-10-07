package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// runInteractive provede uživatele celým postupem otázkami (pro dvojklik na program ve Windows).
// Okno se na konci nezavře, dokud uživatel nestiskne Enter. Vrací kód ukončení.
func runInteractive(path string) int {
	in := bufio.NewReader(os.Stdin)
	code := 0
	if err := interactive(in, path); err != nil {
		fmt.Fprintln(os.Stderr, "\nChyba:", err)
		code = 1
	}
	fmt.Print("\nStiskněte Enter pro ukončení…")
	in.ReadString('\n')
	return code
}

// errInputClosed: vstup skončil (okno zavřeno nebo konec přesměrovaného vstupu).
var errInputClosed = fmt.Errorf("vstup byl ukončen")

func ask(in *bufio.Reader, prompt, def string) (string, error) {
	if def != "" {
		fmt.Printf("%s [%s]: ", prompt, def)
	} else {
		fmt.Printf("%s: ", prompt)
	}
	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		return "", errInputClosed
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return def, nil
	}
	return line, nil
}

func interactive(in *bufio.Reader, path string) error {
	fmt.Println("Slovník tokenů: BPE, WordPiece, SentencePiece")
	fmt.Println("=============================================")
	fmt.Println()

	for {
		if path == "" {
			fmt.Println("Textový soubor (UTF-8) sem můžete přetáhnout myší, nebo napsat jeho cestu.")
			var err error
			if path, err = ask(in, "Soubor", ""); err != nil {
				return err
			}
		}
		path = strings.Trim(strings.TrimSpace(path), "\"'")
		if st, err := os.Stat(path); err == nil && !st.IsDir() {
			break
		}
		if path != "" {
			fmt.Printf("Soubor %q jsem nenašel, zkuste to znovu.\n\n", path)
		}
		path = ""
	}

	fmt.Println()
	fmt.Println("Algoritmus:")
	fmt.Println("  1 = BPE (nejčastější dvojice)")
	fmt.Println("  2 = WordPiece (největší poměr #(AB) / (#(A) · #(B)))")
	fmt.Println("  3 = SentencePiece (jako WordPiece, mezery jako _, tokeny přes více slov)")
	var alg Algorithm
	for {
		a, err := ask(in, "Vyberte 1, 2 nebo 3", "1")
		if err != nil {
			return err
		}
		switch strings.ToLower(a) {
		case "1", "bpe":
			alg = BPE
		case "2", "wordpiece", "wp":
			alg = WordPiece
		case "3", "sentencepiece", "sp":
			alg = SentencePiece
		default:
			fmt.Println("Zadejte 1, 2 nebo 3.")
			continue
		}
		break
	}

	var size int
	for {
		v, err := ask(in, "\nPožadovaná velikost slovníku (počet tokenů)", "1000")
		if err != nil {
			return err
		}
		n, err := strconv.Atoi(v)
		if err == nil && n > 0 {
			size = n
			break
		}
		fmt.Println("Zadejte kladné celé číslo.")
	}

	ignoreCase := true
	c, err := ask(in, "Nerozlišovat velká a malá písmena? (a/n)", "a")
	if err != nil {
		return err
	}
	switch strings.ToLower(c) {
	case "n", "ne":
		ignoreCase = false
	}

	dir := filepath.Dir(path)
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	vocabPath := filepath.Join(dir, fmt.Sprintf("%s-%s-slovnik.tsv", base, alg))
	mergesPath := filepath.Join(dir, fmt.Sprintf("%s-%s-postup.tsv", base, alg))

	fmt.Println("\nPočítám…")
	if err := doTrain(path, alg, size, ignoreCase, vocabPath, mergesPath); err != nil {
		return err
	}
	fmt.Println("\nHotovo. Slovník (číslo tokenu, tabulátor, token) je v prvním souboru,")
	fmt.Println("postup sloučení v druhém. Oba jsou vedle vašeho textu a otevřete je v Poznámkovém bloku.")
	return nil
}
