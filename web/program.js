// Doplňky, které program přidává k webové stránce při zobrazení v okně: rychlý výpočet slovníku
// v programu, vlastní text (karta Trénink), ukončení programu a hlídání, zda je okno otevřené.
(function(){
  'use strict';
  var TOKEN = window.__LLM_TOKEN__;

  function call(method, path, body, cb){
    var x = new XMLHttpRequest();
    x.open(method, path, true);
    x.setRequestHeader('X-Token', TOKEN);
    x.onload = function(){ var d = null; try{ d = JSON.parse(x.responseText); }catch(e){} cb(x.status, d); };
    x.onerror = function(){ cb(0, null); };
    x.send(body === undefined ? null : JSON.stringify(body));
  }

  // rychlý výpočet slovníku (stránka ho zavolá místo výpočtu v prohlížeči)
  window.llmTrain = function(req, cb){
    call('POST', '/api/train', req, function(st, d){
      if(!d){ cb(null); return; }
      if(d.error){ cb({error: d.error, chars: d.chars}); return; }
      cb({
        initial: d.initial,
        tokens: d.tokens,
        exhausted: !!d.exhausted,
        words: d.words,
        merges: d.merges.map(function(m){
          return {a: m.a, b: m.b, sym: m.new, count: m.count, ca: m.countA, cb: m.countB, dup: !!m.dup};
        })
      });
    });
  };

  // stránka se slovníkem tokenů se otevírá s přístupovým klíčem
  var origOpen = window.open;
  window.open = function(url, name, features){
    if(typeof url === 'string' && url.indexOf('slovnik-tokenu.html') === 0){
      url += (url.indexOf('?') < 0 ? '?' : '&') + 't=' + TOKEN;
    }
    return origOpen.call(window, url, name, features);
  };

  // program běží, dokud je stránka otevřená
  setInterval(function(){ call('GET', '/api/ping', undefined, function(){}); }, 5000);
  window.addEventListener('pagehide', function(){
    if(navigator.sendBeacon) navigator.sendBeacon('/api/closing?t=' + TOKEN, '');
  });
  call('GET', '/api/ping', undefined, function(){});

  // tlačítko pro ukončení programu
  var wrap = document.querySelector('.wrap');
  var quitRow = document.createElement('div');
  quitRow.style.cssText = 'text-align:center;margin-top:18px;';
  var quit = document.createElement('button');
  quit.className = 'btn';
  quit.type = 'button';
  quit.textContent = 'Ukončit program';
  quit.addEventListener('click', function(){
    call('POST', '/api/quit', {}, function(){
      document.body.innerHTML = '<div style="text-align:center;padding:80px 20px;font-family:Work Sans,sans-serif;color:#5d6b76">Program byl ukončen. Toto okno můžete zavřít.</div>';
    });
  });
  quitRow.appendChild(quit);
  wrap.appendChild(quitRow);

  // karta Trénink: vlastní text jako korpus (místo textu o Albertu Einsteinovi)
  var area = document.getElementById('in-trenink');
  if(area && window.llmCorpus){
    var panel = area.closest('.panel');
    var bar = document.createElement('div');
    bar.style.cssText = 'display:flex;flex-wrap:wrap;gap:10px;align-items:center;margin-top:12px;';
    function button(text, cls){
      var b = document.createElement('button');
      b.type = 'button';
      b.className = 'btn' + (cls ? ' ' + cls : '');
      b.textContent = text;
      bar.appendChild(b);
      return b;
    }
    var fileBtn = button('Načíst ze souboru…', '');
    var useBtn = button('Použít text jako korpus', 'primary');
    var resetBtn = button('Vrátit text o Albertu Einsteinovi', '');
    var fileIn = document.createElement('input');
    fileIn.type = 'file';
    fileIn.accept = '.txt,.md,.csv,.tsv,.html,.json,text/*';
    fileIn.hidden = true;
    bar.appendChild(fileIn);
    var status = document.createElement('p');
    status.className = 'hint';
    status.style.cssText = 'margin:10px 0 0;';
    status.textContent = 'Korpus: text o Albertu Einsteinovi. Hledání a slovníky na kartě Tokenizace pracují s korpusem.';
    panel.appendChild(bar);
    panel.appendChild(status);

    function use(text, name){
      if(!text.trim()){ status.textContent = 'Vložte text nebo ho načtěte ze souboru.'; return; }
      window.llmCorpus.set(text);
      status.textContent = 'Korpus: ' + name + ' (' + text.length.toLocaleString('cs-CZ') + ' znaků). Přepněte na kartu Tokenizace.';
      var tab = document.getElementById('top-tab-tokenizace');
      if(tab) tab.click();
    }
    useBtn.addEventListener('click', function(){ use(area.value, 'vlastní text'); });
    resetBtn.addEventListener('click', function(){
      window.llmCorpus.reset();
      status.textContent = 'Korpus: text o Albertu Einsteinovi.';
    });
    fileBtn.addEventListener('click', function(){ fileIn.click(); });
    fileIn.addEventListener('change', function(){
      var f = fileIn.files[0];
      if(!f) return;
      var r = new FileReader();
      r.onload = function(){ area.value = r.result; use(r.result, f.name); };
      r.onerror = function(){ status.textContent = 'Soubor se nepodařilo přečíst.'; };
      r.readAsText(f, 'UTF-8');
    });

    // soubor přetažený na program
    call('GET', '/api/initial', undefined, function(st, d){
      if(d && d.text){ area.value = d.text; use(d.text, d.name); }
    });
  }
})();
