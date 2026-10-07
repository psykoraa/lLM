package main

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

// Webová stránka (albert-einstein.html, slovnik-tokenu.html) je zabudovaná v programu.
// Program ji zobrazí v prohlížeči a jen výpočet slovníku provede sám (rychle).
//
//go:embed web/albert-einstein.html web/slovnik-tokenu.html web/program.js web/spolecne/styl.css
var webFS embed.FS

// Přednostní port: při stejném portu zůstane uložený slovník v prohlížeči i po novém spuštění.
const preferredPort = 47831

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

type guiServer struct {
	token       string
	initialName string
	initialText string
	lastSeen    atomic.Int64 // čas posledního kontaktu stránky (unix ms)
	closingAt   atomic.Int64 // čas, kdy se stránka zavřela (unix ms), 0 = není
	quit        chan struct{}
}

func nowMs() int64 { return time.Now().UnixMilli() }

// runGUI spustí místní server na adrese 127.0.0.1, otevře prohlížeč a čeká, dokud se okno nezavře.
func runGUI(args []string) int {
	fs := flag.NewFlagSet("gui", flag.ContinueOnError)
	noBrowser := fs.Bool("no-browser", false, "neotvírat prohlížeč (jen vypsat adresu)")
	port := fs.Int("port", preferredPort, "port (0 = volný port vybere systém)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	srv := &guiServer{quit: make(chan struct{})}
	if fs.NArg() == 1 {
		// soubor přetažený na program nebo zadaný na příkazové řádce
		if b, err := os.ReadFile(fs.Arg(0)); err == nil {
			srv.initialText = string(b)
			srv.initialName = fs.Arg(0)
		} else {
			fmt.Fprintln(os.Stderr, "Soubor se nepodařilo načíst:", err)
		}
	}
	tok := make([]byte, 16)
	if _, err := rand.Read(tok); err != nil {
		fmt.Fprintln(os.Stderr, "Chyba:", err)
		return 1
	}
	srv.token = hex.EncodeToString(tok)

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil && *port != 0 {
		ln, err = net.Listen("tcp", "127.0.0.1:0") // port je obsazený (např. běží druhá kopie)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Nepodařilo se spustit místní server:", err)
		return 1
	}
	url := fmt.Sprintf("http://%s/?t=%s", ln.Addr().String(), srv.token)

	mux := http.NewServeMux()
	mux.HandleFunc("/", srv.handlePage)
	mux.HandleFunc("/spolecne/styl.css", srv.handleCSS)
	mux.HandleFunc("/api/ping", srv.auth(srv.handlePing))
	mux.HandleFunc("/api/initial", srv.auth(srv.handleInitial))
	mux.HandleFunc("/api/train", srv.auth(srv.handleTrain))
	mux.HandleFunc("/api/quit", srv.auth(srv.handleQuit))
	mux.HandleFunc("/api/closing", srv.handleClosing)

	srv.lastSeen.Store(nowMs())
	httpSrv := &http.Server{Handler: mux}
	go httpSrv.Serve(ln)

	fmt.Println("Program běží na adrese", url)
	fmt.Println("Pokud se okno prohlížeče neotevřelo samo, zkopírujte si adresu do prohlížeče.")
	fmt.Println("Program skončí po zavření stránky v prohlížeči nebo tlačítkem „Ukončit program“.")
	if !*noBrowser {
		if err := openBrowser(url); err != nil {
			fmt.Fprintln(os.Stderr, "Prohlížeč se nepodařilo otevřít:", err)
		}
		hideConsoleIfOwned()
	}

	// Ukončení: tlačítko, zavření stránky (po krátké odmlce, aby přežilo obnovení stránky),
	// nebo dlouhá nečinnost (když prohlížeč spadl).
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-srv.quit:
			time.Sleep(300 * time.Millisecond) // ať stihne odejít odpověď
			return 0
		case <-tick.C:
			now := nowMs()
			if c := srv.closingAt.Load(); c != 0 && now-c > 4000 && srv.lastSeen.Load() < c {
				return 0
			}
			if now-srv.lastSeen.Load() > 5*60*1000 {
				return 0
			}
		}
	}
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

func (s *guiServer) auth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Token") != s.token {
			http.Error(w, "neplatný přístup", http.StatusForbidden)
			return
		}
		h(w, r)
	}
}

// handlePage vrací webovou stránku (hlavní nebo stránku se slovníkem tokenů); bez přístupového
// klíče v adrese ji nevydá.
func (s *guiServer) handlePage(w http.ResponseWriter, r *http.Request) {
	var name string
	switch r.URL.Path {
	case "/":
		name = "web/albert-einstein.html"
	case "/slovnik-tokenu.html":
		name = "web/slovnik-tokenu.html"
	default:
		http.NotFound(w, r)
		return
	}
	if r.URL.Query().Get("t") != s.token {
		http.Error(w, "Neplatná adresa. Spusťte program znovu.", http.StatusForbidden)
		return
	}
	b, _ := webFS.ReadFile(name)
	page := string(b)
	if name == "web/albert-einstein.html" {
		js, _ := webFS.ReadFile("web/program.js")
		inject := `<script>window.__LLM_TOKEN__="` + s.token + `";</script>` + "\n<script>\n" + string(js) + "\n</script>\n</body>"
		page = strings.Replace(page, "</body>", inject, 1)
	} else {
		page = strings.Replace(page, `href="albert-einstein.html"`, `href="/?t=`+s.token+`"`, 1)
	}
	s.lastSeen.Store(nowMs())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte(page))
}

func (s *guiServer) handleCSS(w http.ResponseWriter, r *http.Request) {
	b, _ := webFS.ReadFile("web/spolecne/styl.css")
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Write(b)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}

func (s *guiServer) handlePing(w http.ResponseWriter, r *http.Request) {
	s.lastSeen.Store(nowMs())
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *guiServer) handleInitial(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"name": s.initialName, "text": s.initialText})
	s.initialText = "" // předat jen jednou
}

func (s *guiServer) handleClosing(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("t") == s.token {
		s.closingAt.Store(nowMs())
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *guiServer) handleQuit(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]bool{"ok": true})
	select {
	case <-s.quit:
	default:
		close(s.quit)
	}
}

func (s *guiServer) handleTrain(w http.ResponseWriter, r *http.Request) {
	s.lastSeen.Store(nowMs())
	if r.Method != http.MethodPost {
		http.Error(w, "očekáván POST", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2<<30)
	var req apiRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, apiResponse{Error: "Požadavek se nepodařilo přečíst: " + err.Error()})
		return
	}
	alg, ok := ParseAlgorithm(req.Algo)
	if !ok {
		writeJSON(w, apiResponse{Error: "Neznámý algoritmus."})
		return
	}
	if req.Size < 1 {
		writeJSON(w, apiResponse{Error: "Velikost slovníku musí být kladné číslo."})
		return
	}
	t0 := time.Now()
	corpus, err := BuildCorpus(strings.NewReader(req.Text), alg, req.IgnoreCase)
	if err != nil {
		writeJSON(w, apiResponse{Error: err.Error()})
		return
	}
	if len(corpus.Words) == 0 {
		writeJSON(w, apiResponse{Error: "nowords"})
		return
	}
	res, err := Train(corpus, alg, req.Size)
	if err != nil {
		if e, ok := err.(ErrTooSmall); ok {
			writeJSON(w, apiResponse{Error: "toosmall", Chars: e.Chars})
			return
		}
		writeJSON(w, apiResponse{Error: err.Error()})
		return
	}
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
	writeJSON(w, out)
}
