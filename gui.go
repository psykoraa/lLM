//go:build !js

package main

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
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

type guiServer struct {
	token       string
	initialName string
	initialText string
	lastSeen    atomic.Int64 // čas posledního kontaktu stránky (unix ms)
	closingAt   atomic.Int64 // čas, kdy se stránka zavřela (unix ms), 0 = není
	quit        chan struct{}

	*engine
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
	srv := &guiServer{quit: make(chan struct{}), engine: newEngine()}
	if fs.NArg() == 1 {
		// soubor přetažený na program nebo zadaný na příkazové řádce
		if text, err := readInputText(fs.Arg(0)); err == nil {
			srv.initialText = text
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
	mux.HandleFunc("/api/pdf", srv.auth(srv.handlePDF))
	mux.HandleFunc("/api/matrix", srv.auth(srv.handleMatrix))
	mux.HandleFunc("/api/matrix/view", srv.auth(srv.handleMatrixView))
	mux.HandleFunc("/api/matrix/download", srv.authQuery(srv.handleMatrixDownload))
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

// authQuery je jako auth, ale přístupový klíč může být i v adrese (pro stažení souboru odkazem).
func (s *guiServer) authQuery(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Token") != s.token && r.URL.Query().Get("t") != s.token {
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

// handlePDF přijme PDF (surové bajty) a vrátí z něj vytažený text.
func (s *guiServer) handlePDF(w http.ResponseWriter, r *http.Request) {
	s.lastSeen.Store(nowMs())
	if r.Method != http.MethodPost {
		http.Error(w, "očekáván POST", http.StatusMethodNotAllowed)
		return
	}
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<30))
	if err != nil {
		writeJSON(w, map[string]string{"error": "Soubor se nepodařilo přijmout: " + err.Error()})
		return
	}
	text, err := ExtractPDFText(b)
	if err != nil {
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, map[string]string{"text": text})
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
	writeJSON(w, s.Train(req))
}

func (s *guiServer) handleMatrix(w http.ResponseWriter, r *http.Request) {
	s.lastSeen.Store(nowMs())
	if r.Method != http.MethodPost {
		http.Error(w, "očekáván POST", http.StatusMethodNotAllowed)
		return
	}
	var req matrixRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, matrixResponse{Error: "Požadavek se nepodařilo přečíst: " + err.Error()})
		return
	}
	writeJSON(w, s.Matrix(req))
}

func (s *guiServer) handleMatrixView(w http.ResponseWriter, r *http.Request) {
	s.lastSeen.Store(nowMs())
	k, _ := strconv.Atoi(r.URL.Query().Get("k"))
	writeJSON(w, s.MatrixView(k, r.URL.Query().Get("mode") != "index"))
}

// handleMatrixDownload pošle celou matici jako soubor .tsv.
func (s *guiServer) handleMatrixDownload(w http.ResponseWriter, r *http.Request) {
	s.lastSeen.Store(nowMs())
	m, alg := s.MatrixFile()
	if m == nil {
		http.Error(w, "Matice ještě není vytvořená.", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/tab-separated-values; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+matrixFileName(alg, m)+`"`)
	m.WriteTSV(w)
}
