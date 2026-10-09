//go:build js && wasm

package main

import (
	"bytes"
	"encoding/json"
	"syscall/js"
)

// Webová stránka: stejné výpočty jako v programu, přeložené do WebAssembly a spuštěné
// ve webovém workeru přímo v prohlížeči (web/worker.js). Funkce přijímají a vracejí JSON
// (stejný jako místní server v gui.go), takže stránka se chová stejně.

func main() {
	e := newEngine()
	g := js.Global()

	g.Set("llmTrain", js.FuncOf(func(this js.Value, a []js.Value) any {
		var req apiRequest
		if err := json.Unmarshal([]byte(a[0].String()), &req); err != nil {
			return toJSON(apiResponse{Error: "Požadavek se nepodařilo přečíst: " + err.Error()})
		}
		return toJSON(e.Train(req))
	}))
	g.Set("llmMatrix", js.FuncOf(func(this js.Value, a []js.Value) any {
		var req matrixRequest
		if err := json.Unmarshal([]byte(a[0].String()), &req); err != nil {
			return toJSON(matrixResponse{Error: "Požadavek se nepodařilo přečíst: " + err.Error()})
		}
		return toJSON(e.Matrix(req))
	}))
	g.Set("llmMatrixView", js.FuncOf(func(this js.Value, a []js.Value) any {
		return toJSON(e.MatrixView(a[0].Int(), a[1].String() != "index"))
	}))
	// llmMatrixTSV() -> {name, data: Uint8Array} nebo {error}
	g.Set("llmMatrixTSV", js.FuncOf(func(this js.Value, a []js.Value) any {
		m, alg := e.MatrixFile()
		if m == nil {
			return map[string]any{"error": "Matice ještě není vytvořená."}
		}
		var buf bytes.Buffer
		m.WriteTSV(&buf)
		u8 := js.Global().Get("Uint8Array").New(buf.Len())
		js.CopyBytesToJS(u8, buf.Bytes())
		return map[string]any{"name": matrixFileName(alg, m), "data": u8}
	}))
	// llmPDF(Uint8Array) -> JSON {text} nebo {error}
	g.Set("llmPDF", js.FuncOf(func(this js.Value, a []js.Value) any {
		b := make([]byte, a[0].Get("length").Int())
		js.CopyBytesToGo(b, a[0])
		text, err := ExtractPDFText(b)
		if err != nil {
			return toJSON(map[string]string{"error": err.Error()})
		}
		return toJSON(map[string]string{"text": text})
	}))
	g.Set("llmReady", true)
	select {}
}

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
