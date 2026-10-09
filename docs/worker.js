// Worker webové stránky: načte llm.wasm (Go přeložené do WebAssembly) a provádí výpočty,
// aby se stránka při počítání nezasekla.
importScripts('wasm_exec.js');

var ready = null;
function init(){
  if(ready) return ready;
  var go = new Go();
  ready = fetch('llm.wasm').then(function(r){
    if(!r.ok) throw new Error('llm.wasm se nepodařilo stáhnout (' + r.status + ')');
    if(WebAssembly.instantiateStreaming && (r.headers.get('Content-Type') || '').indexOf('application/wasm') >= 0){
      return WebAssembly.instantiateStreaming(r, go.importObject);
    }
    return r.arrayBuffer().then(function(b){ return WebAssembly.instantiate(b, go.importObject); });
  }).then(function(res){
    go.run(res.instance);   // běží stále (Go čeká na volání)
  });
  return ready;
}

onmessage = function(ev){
  var m = ev.data;
  init().then(function(){
    var out, transfer = [];
    try{
      switch(m.fn){
        case 'train': out = JSON.parse(self.llmTrain(JSON.stringify(m.arg))); break;
        case 'matrix': out = JSON.parse(self.llmMatrix(JSON.stringify(m.arg))); break;
        case 'view': out = JSON.parse(self.llmMatrixView(m.arg.k, m.arg.mode)); break;
        case 'pdf': out = JSON.parse(self.llmPDF(new Uint8Array(m.arg))); break;
        case 'tsv':
          out = self.llmMatrixTSV();
          if(out.data) transfer.push(out.data.buffer);
          break;
        case 'ping': out = {ok: true}; break;
        default: out = {error: 'Neznámá funkce.'};
      }
    }catch(e){
      out = {error: 'Chyba výpočtu: ' + (e && e.message || e)};
    }
    postMessage({id: m.id, out: out}, transfer);
  }, function(e){
    postMessage({id: m.id, out: {error: 'Nepodařilo se načíst výpočetní část stránky: ' + (e && e.message || e)}});
  });
};
