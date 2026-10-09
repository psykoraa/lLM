// Webová stránka bez programu: místo místního serveru volá worker s WebAssembly (worker.js).
// Rozhraní odpovídá tomu, co od serveru očekává program.js.
(function(){
  'use strict';
  var worker = null, seq = 0, pending = {};
  var wasmError = 'Prohlížeč nepodporuje WebAssembly nebo web workery.';

  function start(){
    if(worker) return worker;
    worker = new Worker('worker.js');
    worker.onmessage = function(ev){
      var cb = pending[ev.data.id];
      delete pending[ev.data.id];
      if(cb) cb(ev.data.out);
    };
    worker.onerror = function(){
      var all = pending; pending = {};
      for(var k in all) all[k]({error: 'Výpočetní část stránky se nepodařilo spustit.'});
    };
    return worker;
  }
  function rpc(fn, arg, cb, transfer){
    var id = ++seq;
    pending[id] = cb;
    try{ start().postMessage({id: id, fn: fn, arg: arg}, transfer || []); }
    catch(e){ delete pending[id]; cb({error: wasmError}); }
  }

  window.__LLM_WEB__ = {
    call: function(method, path, body, cb){
      if(path.indexOf('/api/train') === 0){
        rpc('train', body, function(d){ cb(200, d); });
      }else if(path.indexOf('/api/matrix/view') === 0){
        var q = new URLSearchParams(path.split('?')[1] || '');
        rpc('view', {k: parseInt(q.get('k'), 10) || 20, mode: q.get('mode') || 'freq'}, function(d){ cb(200, d); });
      }else if(path.indexOf('/api/matrix') === 0){
        rpc('matrix', body, function(d){ cb(200, d); });
      }else{
        cb(200, {});
      }
    },
    pdf: function(file, cb){
      var r = new FileReader();
      r.onload = function(){ rpc('pdf', r.result, cb, [r.result]); };
      r.onerror = function(){ cb({error: 'PDF se nepodařilo přečíst.'}); };
      r.readAsArrayBuffer(file);
    },
    download: function(cb){
      rpc('tsv', null, function(d){
        if(!d || d.error){ cb((d && d.error) || 'Matici se nepodařilo stáhnout.'); return; }
        var url = URL.createObjectURL(new Blob([d.data], {type: 'text/tab-separated-values;charset=utf-8'}));
        var a = document.createElement('a');
        a.href = url;
        a.download = d.name;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        setTimeout(function(){ URL.revokeObjectURL(url); }, 10000);
        cb(null);
      });
    }
  };
  start();   // wasm se začne stahovat hned, ať je připravený při prvním výpočtu
  start().postMessage({id: 0, fn: 'ping'});
})();
