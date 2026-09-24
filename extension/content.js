// web-cli bridge — content script.
// Collects visible, clickable elements using the Vimium C visibility approach
// (client rects clipped to the viewport + computed style + elementFromPoint
// occlusion test), tags them with hint numbers, and performs clicks with the
// same synthetic event sequence Vimium C uses.
(() => {
  "use strict";
  if (globalThis.__webcliContentLoaded) return; // injected twice (manifest + scripting fallback)
  globalThis.__webcliContentLoaded = true;

  const SELECTOR = "a, button, input, select, textarea, [role=button], [onclick]";
  const HINT_ATTR = "data-webcli-hint";
  const MAX_TEXT = 80;
  const FOCUS_INPUT_TYPES = new Set([
    "", "text", "search", "email", "url", "password", "number", "tel", "date",
    "datetime-local", "month", "time", "week", "color",
  ]);

  let hints = new Map();

  // --- visibility (Vimium C style) ---------------------------------------

  function viewport() {
    return {
      w: window.innerWidth || document.documentElement.clientWidth,
      h: window.innerHeight || document.documentElement.clientHeight,
    };
  }

  // 1. getBoundingClientRect / getClientRects: non-empty and intersecting the viewport.
  //    Like Vimium C, use the first client rect that survives clipping so that
  //    links wrapping over several lines get a point on actual text.
  function visibleRect(el) {
    const b = el.getBoundingClientRect();
    const { w, h } = viewport();
    if (b.width <= 0 || b.height <= 0) return null;
    if (b.bottom <= 0 || b.right <= 0 || b.top >= h || b.left >= w) return null;
    for (const r of el.getClientRects()) {
      const left = Math.max(r.left, 0);
      const top = Math.max(r.top, 0);
      const right = Math.min(r.right, w);
      const bottom = Math.min(r.bottom, h);
      if (right - left >= 1 && bottom - top >= 1) return { left, top, right, bottom };
    }
    return null;
  }

  // 2. getComputedStyle: visibility/display/opacity. visibility is inherited;
  //    opacity is not, so ancestors are checked for opacity 0 as well.
  function styleVisible(el) {
    const cs = getComputedStyle(el);
    if (cs.visibility === "hidden" || cs.visibility === "collapse") return false;
    if (cs.display === "none") return false;
    for (let n = el; n && n.nodeType === Node.ELEMENT_NODE; n = n.parentElement) {
      if (getComputedStyle(n).opacity === "0") return false;
    }
    return true;
  }

  function deepElementFromPoint(x, y) {
    let hit = document.elementFromPoint(x, y);
    while (hit && hit.shadowRoot) {
      const inner = hit.shadowRoot.elementFromPoint(x, y);
      if (!inner || inner === hit) break;
      hit = inner;
    }
    return hit;
  }

  // 3. elementFromPoint: the element (or one of its descendants / its label)
  //    must be the topmost hit at the centre of the visible rect, or at one of
  //    its inset corners — otherwise it is covered by a modal/overlay.
  function notCovered(el, r) {
    const cx = (r.left + r.right) / 2;
    const cy = (r.top + r.bottom) / 2;
    const points = [
      [cx, cy],
      [r.left + 1, r.top + 1], [r.right - 1, r.top + 1],
      [r.left + 1, r.bottom - 1], [r.right - 1, r.bottom - 1],
    ];
    for (const [x, y] of points) {
      const hit = deepElementFromPoint(x, y);
      if (!hit) continue;
      if (hit === el || el.contains(hit)) return true;
      if (hit instanceof HTMLLabelElement && hit.control === el) return true;
    }
    return false;
  }

  function isHintable(el) {
    if (el.disabled) return null;
    if (el instanceof HTMLInputElement && el.type === "hidden") return null;
    if (!styleVisible(el)) return null;
    const r = visibleRect(el);
    if (!r) return null;
    if (!notCovered(el, r)) return null;
    return r;
  }

  // --- description --------------------------------------------------------

  function clip(s) {
    s = (s || "").replace(/\s+/g, " ").trim();
    return s.length > MAX_TEXT ? s.slice(0, MAX_TEXT - 1) + "…" : s;
  }

  // Text of the <label>s (or aria-labelledby) naming a form control, so that
  // "type Email ..." finds <label for=email>Email</label><input id=email>.
  function labelText(el) {
    const ids = el.getAttribute("aria-labelledby");
    if (ids) {
      const t = ids.split(/\s+/).map((id) => document.getElementById(id)).filter(Boolean)
        .map((n) => n.innerText).join(" ");
      if (t.trim()) return t;
    }
    if (el.labels && el.labels.length) {
      return Array.from(el.labels).map((l) => {
        // label text without the text of the control itself (wrapping labels)
        const c = l.cloneNode(true);
        for (const x of c.querySelectorAll("input,select,textarea")) x.remove();
        return c.innerText || c.textContent;
      }).join(" ");
    }
    return "";
  }

  // A CSS selector that identifies el uniquely now and, preferably, after a
  // reload (ids / name / test ids first, nth-of-type path as a fallback).
  function selectorFor(el) {
    const esc = CSS.escape;
    const unique = (sel) => { try { return document.querySelectorAll(sel).length === 1; } catch { return false; } };
    const tag = el.tagName.toLowerCase();
    if (el.id && !/^\d|\d{5,}/.test(el.id) && unique("#" + esc(el.id))) return "#" + esc(el.id);
    for (const attr of ["name", "data-testid", "data-test", "data-qa", "aria-label", "placeholder"]) {
      const v = el.getAttribute(attr);
      if (!v) continue;
      const sel = `${tag}[${attr}="${v.replace(/\\/g, "\\\\").replace(/"/g, '\\"')}"]`;
      if (unique(sel)) return sel;
    }
    const parts = [];
    for (let n = el; n && n.nodeType === Node.ELEMENT_NODE && n !== document.documentElement; n = n.parentElement) {
      if (n !== el && n.id && !/^\d|\d{5,}/.test(n.id) && unique("#" + esc(n.id))) {
        parts.unshift("#" + esc(n.id));
        break;
      }
      const t = n.tagName.toLowerCase();
      if (t === "body") { parts.unshift("body"); break; }
      let i = 1;
      for (let sib = n.previousElementSibling; sib; sib = sib.previousElementSibling) if (sib.tagName === n.tagName) i++;
      parts.unshift(`${t}:nth-of-type(${i})`);
    }
    return parts.join(" > ");
  }

  function describe(el) {
    const tag = el.tagName.toLowerCase();
    let text = "";
    if (el instanceof HTMLInputElement) {
      text = ["button", "submit", "reset"].includes(el.type)
        ? el.value
        : el.getAttribute("aria-label") || labelText(el) || el.placeholder || el.name ||
          (["checkbox", "radio"].includes(el.type) ? "" : el.value);
    } else if (el instanceof HTMLSelectElement) {
      const label = el.getAttribute("aria-label") || labelText(el);
      const cur = (el.selectedOptions[0] && el.selectedOptions[0].text) || "";
      text = label ? `${label}: ${cur}` : cur || el.name;
    } else if (el instanceof HTMLTextAreaElement) {
      text = el.getAttribute("aria-label") || labelText(el) || el.placeholder || el.name;
    } else {
      text = el.innerText || el.getAttribute("aria-label") || el.title;
      if (!text) {
        const img = el.querySelector("img[alt]");
        text = img ? img.alt : "";
      }
    }
    return {
      tag,
      type: el instanceof HTMLInputElement ? el.type : undefined,
      role: el.getAttribute("role") || undefined,
      text: clip(text),
      href: el instanceof HTMLAnchorElement ? el.href : undefined,
      name: el.getAttribute("name") || undefined,
      id: el.id || undefined,
      checked: el instanceof HTMLInputElement && ["checkbox", "radio"].includes(el.type) ? el.checked : undefined,
    };
  }

  // --- commands -----------------------------------------------------------

  // scope "viewport" (default): the Vimium C filter (what you can click now).
  // scope "page": every rendered, enabled control, also outside the viewport
  // (used to resolve text targets such as "type Password ..." on long pages).
  function pageRect(el) {
    if (el.disabled) return null;
    if (el instanceof HTMLInputElement && el.type === "hidden") return null;
    if (!styleVisible(el)) return null;
    const b = el.getBoundingClientRect();
    if (b.width <= 0 || b.height <= 0) return null;
    return { left: b.left, top: b.top, right: b.right, bottom: b.bottom };
  }

  function collect(scope) {
    for (const old of document.querySelectorAll(`[${HINT_ATTR}]`)) old.removeAttribute(HINT_ATTR);
    hints = new Map();
    const elements = [];
    let n = 0;
    for (const el of document.querySelectorAll(SELECTOR)) {
      const r = scope === "page" ? pageRect(el) : isHintable(el);
      if (!r) continue;
      n++;
      el.setAttribute(HINT_ATTR, String(n));
      hints.set(n, el);
      elements.push({
        hint: n,
        ...describe(el),
        rect: { x: r.left, y: r.top, w: r.right - r.left, h: r.bottom - r.top },
      });
    }
    return { ok: true, url: location.href, title: document.title, elements };
  }

  function isFocusTarget(el) {
    if (el instanceof HTMLTextAreaElement || el instanceof HTMLSelectElement) return true;
    if (el instanceof HTMLInputElement) return FOCUS_INPUT_TYPES.has(el.type);
    return el.isContentEditable;
  }

  function mouse(el, type, x, y) {
    const init = {
      bubbles: true, cancelable: true, composed: true, view: window,
      clientX: x, clientY: y, button: 0, buttons: type === "mousedown" ? 1 : 0, detail: 1,
    };
    const Ctor = type.startsWith("pointer") ? PointerEvent : MouseEvent;
    if (Ctor === PointerEvent) Object.assign(init, { pointerId: 1, pointerType: "mouse", isPrimary: true });
    return el.dispatchEvent(new Ctor(type, init));
  }

  // Vimium C click sequence: over -> down -> focus -> up -> click.
  function simulateClick(el) {
    const r = el.getBoundingClientRect();
    const x = r.left + r.width / 2;
    const y = r.top + r.height / 2;
    mouse(el, "pointerover", x, y);
    mouse(el, "mouseover", x, y);
    mouse(el, "pointerdown", x, y);
    mouse(el, "mousedown", x, y);
    if (typeof el.focus === "function") el.focus({ preventScroll: true });
    mouse(el, "pointerup", x, y);
    mouse(el, "mouseup", x, y);
    mouse(el, "click", x, y);
  }

  function click(hint) {
    const el = hints.get(hint) || document.querySelector(`[${HINT_ATTR}="${Number(hint)}"]`);
    if (!el || !el.isConnected) return { ok: false, error: `hint ${hint} is stale; collect again` };
    const d = describe(el);
    const kind = isFocusTarget(el) ? "focus" : "click";
    // Reply first, act on the next task: a click may navigate and tear down
    // this document (and the message channel) before sendResponse runs.
    setTimeout(() => {
      el.scrollIntoView({ block: "nearest", inline: "nearest" });
      if (kind === "focus") el.focus();
      else simulateClick(el);
    }, 0);
    return { ok: true, kind, tag: d.tag, text: d.text, selector: selectorFor(el), url: location.href };
  }

  // --- form input, keys and page operations ---------------------------------

  function resolve(target) {
    if (!target) return null;
    let el = null;
    if (target.hint !== undefined) {
      el = hints.get(Number(target.hint)) || document.querySelector(`[${HINT_ATTR}="${Number(target.hint)}"]`);
      if (!el || !el.isConnected) throw new Error(`hint ${target.hint} is stale; run 'list' again`);
    } else if (target.css) {
      el = document.querySelector(target.css);
      if (!el) throw new Error(`not found: css=${target.css}`);
    }
    return el;
  }

  function need(target) {
    const el = resolve(target);
    if (!el) throw new Error("this operation needs a target element");
    return el;
  }

  function result(el, extra) {
    const d = describe(el);
    return { ok: true, tag: d.tag, type: d.type, text: d.text, selector: selectorFor(el), url: location.href, ...extra };
  }

  const protoOf = (el) =>
    el instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype
      : el instanceof HTMLSelectElement ? HTMLSelectElement.prototype
        : HTMLInputElement.prototype;

  // Set .value through the prototype setter and fire the events frameworks
  // listen to (React/Vue/Angular track the value and react to input/change).
  function setValue(el, value, data) {
    Object.getOwnPropertyDescriptor(protoOf(el), "value").set.call(el, value);
    el.dispatchEvent(new InputEvent("input", { bubbles: true, composed: true, inputType: "insertText", data }));
    el.dispatchEvent(new Event("change", { bubbles: true }));
  }

  const KEYS = {
    Enter: [13, "Enter"], Tab: [9, "Tab"], Escape: [27, "Escape"], Backspace: [8, "Backspace"],
    Delete: [46, "Delete"], Space: [32, "Space"], ArrowUp: [38, "ArrowUp"], ArrowDown: [40, "ArrowDown"],
    ArrowLeft: [37, "ArrowLeft"], ArrowRight: [39, "ArrowRight"], Home: [36, "Home"], End: [35, "End"],
    PageUp: [33, "PageUp"], PageDown: [34, "PageDown"],
  };

  function keyEvent(el, type, key) {
    const [code, name] = KEYS[key] || [key.toUpperCase().charCodeAt(0), "Key" + key.toUpperCase()];
    const k = key === "Space" ? " " : key;
    return el.dispatchEvent(new KeyboardEvent(type, {
      key: k, code: name, keyCode: code, which: code, bubbles: true, cancelable: true, composed: true,
    }));
  }

  function typeInto(el, text, append) {
    el.scrollIntoView({ block: "center", inline: "nearest" });
    el.focus();
    if (el.isContentEditable) {
      if (!append) document.execCommand("selectAll", false);
      document.execCommand("insertText", false, text);
      return;
    }
    if (!(el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement)) {
      throw new Error(`<${el.tagName.toLowerCase()}> is not a text field`);
    }
    for (const ch of text.slice(0, 1)) keyEvent(el, "keydown", ch);
    setValue(el, append ? el.value + text : text, text);
    for (const ch of text.slice(-1)) keyEvent(el, "keyup", ch);
  }

  function selectOption(el, want) {
    if (!(el instanceof HTMLSelectElement)) throw new Error(`<${el.tagName.toLowerCase()}> is not a <select>`);
    const w = String(want).trim().toLowerCase();
    const opts = Array.from(el.options);
    const opt = opts.find((o) => o.value === want) || opts.find((o) => o.text.trim().toLowerCase() === w) ||
      opts.find((o) => o.text.trim().toLowerCase().includes(w));
    if (!opt) throw new Error(`no option ${JSON.stringify(want)}; options: ${opts.map((o) => o.text.trim()).join(", ")}`);
    el.focus();
    setValue(el, opt.value);
    return opt.text.trim();
  }

  function focusables() {
    return Array.from(document.querySelectorAll(
      "a[href], button, input, select, textarea, [tabindex], [contenteditable=true]",
    )).filter((e) => !e.disabled && e.tabIndex >= 0 && pageRect(e) && !(e instanceof HTMLInputElement && e.type === "hidden"));
  }

  function defaultButton(form) {
    return Array.from(form.elements).find((e) =>
      (e instanceof HTMLButtonElement && (e.type || "submit") === "submit") ||
      (e instanceof HTMLInputElement && ["submit", "image"].includes(e.type)));
  }

  function submitForm(form) {
    const btn = defaultButton(form);
    if (typeof form.requestSubmit === "function") form.requestSubmit(btn || undefined);
    else if (btn) btn.click();
    else form.submit();
  }

  // press: key events on the target (or the focused element) plus the default
  // action a real key would have, since synthetic events have none.
  function press(el, key) {
    el = el || document.activeElement || document.body;
    const down = keyEvent(el, "keydown", key);
    if (key.length === 1 || key === "Enter") keyEvent(el, "keypress", key);
    let did = "";
    if (down) {
      const text = el instanceof HTMLInputElement && FOCUS_INPUT_TYPES.has(el.type);
      if (key === "Enter") {
        if (el instanceof HTMLTextAreaElement) { setValue(el, el.value + "\n", "\n"); did = "newline"; }
        else if (text && el.form) { submitForm(el.form); did = "submit"; }
        else if (el.matches("a[href], button, input[type=submit], input[type=button], [role=button]")) { simulateClick(el); did = "click"; }
      } else if (key === "Space" && el.matches("button, input[type=checkbox], input[type=radio], [role=button]")) {
        simulateClick(el); did = "click";
      } else if (key === "Tab") {
        const list = focusables();
        const next = list[(list.indexOf(el) + 1) % list.length];
        if (next) { next.focus(); did = "focus " + selectorFor(next); }
      } else if (key === "Backspace" && (text || el instanceof HTMLTextAreaElement)) {
        setValue(el, el.value.slice(0, -1)); did = "delete";
      } else if (key.length === 1 && (text || el instanceof HTMLTextAreaElement)) {
        setValue(el, el.value + key, key); did = "insert";
      } else if (["PageDown", "PageUp", "Home", "End"].includes(key) && !text) {
        scroll({ PageDown: "down", PageUp: "up", Home: "top", End: "bottom" }[key]); did = "scroll";
      }
    }
    keyEvent(el, "keyup", key);
    return { el, did };
  }

  function scroll(to) {
    const h = window.innerHeight * 0.8;
    if (to === "top") window.scrollTo(0, 0);
    else if (to === "bottom") window.scrollTo(0, document.documentElement.scrollHeight);
    else if (to === "up") window.scrollBy(0, -h);
    else window.scrollBy(0, h);
    return { ok: true, scroll_y: Math.round(window.scrollY), url: location.href };
  }

  function storageDump(area) {
    const out = {};
    for (let i = 0; i < area.length; i++) { const k = area.key(i); out[k] = area.getItem(k); }
    return out;
  }

  // Ops that change the page reply first and act on the next task, because
  // they may navigate and tear down this document (and the reply channel).
  function later(fn) { setTimeout(fn, 0); }

  function dom(msg) {
    const t = msg.target;
    switch (msg.op) {
      case "click": {
        const el = need(t);
        const kind = isFocusTarget(el) ? "focus" : "click";
        const r = result(el, { kind });
        later(() => {
          el.scrollIntoView({ block: "center", inline: "nearest" });
          if (kind === "focus") el.focus(); else simulateClick(el);
        });
        return r;
      }
      case "focus": {
        const el = need(t);
        el.scrollIntoView({ block: "center", inline: "nearest" });
        el.focus();
        return result(el, { kind: "focus" });
      }
      case "type": {
        const el = need(t);
        typeInto(el, String(msg.text ?? ""), !!msg.append);
        return result(el, { kind: "type", length: String(msg.text ?? "").length });
      }
      case "clear": {
        const el = need(t);
        typeInto(el, "", false);
        return result(el, { kind: "clear" });
      }
      case "select": {
        const el = need(t);
        const chosen = selectOption(el, msg.value);
        return result(el, { kind: "select", value: chosen });
      }
      case "check": {
        const el = need(t);
        if (!(el instanceof HTMLInputElement) || !["checkbox", "radio"].includes(el.type)) {
          throw new Error(`<${el.tagName.toLowerCase()}> is not a checkbox/radio`);
        }
        const want = msg.on !== false;
        if (el.checked !== want) simulateClick(el);
        return result(el, { kind: want ? "check" : "uncheck", checked: el.checked });
      }
      case "press": {
        const el = resolve(t);
        const key = String(msg.key || "Enter");
        const target = el || document.activeElement || document.body;
        const r = result(target, { kind: "press", key });
        later(() => press(target, key));
        return r;
      }
      case "submit": {
        const el = resolve(t) || document.activeElement;
        const form = el && (el instanceof HTMLFormElement ? el : el.form || el.closest("form"));
        if (!form) throw new Error("no form found for the target / focused element");
        const r = result(form, { kind: "submit" });
        later(() => submitForm(form));
        return r;
      }
      case "scroll": {
        const el = resolve(t);
        if (el) { el.scrollIntoView({ block: "center" }); return result(el, { kind: "scroll" }); }
        return scroll(msg.to);
      }
      case "text": {
        const el = resolve(t) || document.body;
        return { ok: true, url: location.href, title: document.title, text: el.innerText };
      }
      case "html": {
        const el = resolve(t);
        const dt = document.doctype ? `<!DOCTYPE ${document.doctype.name}>\n` : "";
        return { ok: true, url: location.href, title: document.title, html: el ? el.outerHTML : dt + document.documentElement.outerHTML };
      }
      case "exists": {
        let found = false;
        if (msg.text !== undefined) found = (document.body && document.body.innerText || "").includes(msg.text);
        else if (t && t.css) found = !!document.querySelector(t.css);
        return { ok: true, found, url: location.href, title: document.title };
      }
      case "info":
        return { ok: true, url: location.href, title: document.title, ready_state: document.readyState };
      case "storage_get":
        return { ok: true, origin: location.origin, local: storageDump(localStorage), session: storageDump(sessionStorage) };
      case "storage_set": {
        let n = 0;
        for (const [area, obj] of [[localStorage, msg.local], [sessionStorage, msg.session]]) {
          for (const [k, v] of Object.entries(obj || {})) { area.setItem(k, String(v)); n++; }
        }
        return { ok: true, origin: location.origin, set: n };
      }
      default:
        throw new Error(`unknown dom op ${msg.op}`);
    }
  }

  // --- recording of real user input (like the DevTools Recorder) ---------
  // Only trusted events (real mouse/keyboard) are recorded, so operations
  // driven by web-cli itself are not recorded twice.

  let rec = null; // { secrets: bool } while recording
  const lastTyped = new WeakMap();

  function emit(op, el, extra) {
    if (!rec) return;
    const d = describe(el);
    const ev = { op, selector: selectorFor(el), tag: d.tag, type: d.type, text: d.text, url: location.href, ...extra };
    try { chrome.runtime.sendMessage({ type: "record_event", event: ev }); } catch { /* extension reloaded */ }
  }

  function recordValue(el) {
    if (lastTyped.get(el) === el.value) return;
    lastTyped.set(el, el.value);
    const secret = el instanceof HTMLInputElement && el.type === "password";
    emit("type", el, secret && !rec.secrets ? { secret: true } : { value: el.value, secret });
  }

  const isTextField = (el) => el instanceof HTMLTextAreaElement ||
    (el instanceof HTMLInputElement && FOCUS_INPUT_TYPES.has(el.type)) || el.isContentEditable;

  function onRecClick(e) {
    if (!rec || !e.isTrusted) return;
    const el = e.target instanceof Element && e.target.closest(SELECTOR);
    if (!el || el instanceof HTMLSelectElement || isTextField(el)) return;
    if (el instanceof HTMLInputElement && ["checkbox", "radio"].includes(el.type)) {
      emit(el.checked ? "check" : "uncheck", el);
      return;
    }
    // flush a text field whose change event has not fired yet
    const a = document.activeElement;
    if (a && a !== el && isTextField(a) && !a.isContentEditable) recordValue(a);
    emit("click", el);
  }

  function onRecChange(e) {
    if (!rec || !e.isTrusted) return;
    const el = e.target;
    if (el instanceof HTMLSelectElement) emit("select", el, { value: el.selectedOptions[0] ? el.selectedOptions[0].text.trim() : el.value });
    else if (isTextField(el) && !el.isContentEditable) recordValue(el);
  }

  function onRecKey(e) {
    if (!rec || !e.isTrusted || e.key !== "Enter") return;
    const el = e.target;
    if (el instanceof HTMLInputElement && FOCUS_INPUT_TYPES.has(el.type)) {
      recordValue(el);
      emit("press", el, { key: "Enter" });
    }
  }

  function setRecording(state) {
    rec = state && state.on ? { secrets: !!state.secrets } : null;
  }

  window.addEventListener("click", onRecClick, true);
  window.addEventListener("change", onRecChange, true);
  window.addEventListener("keydown", onRecKey, true);
  try {
    chrome.runtime.sendMessage?.({ type: "record_state" })?.then?.(setRecording, () => {});
  } catch { /* not available (tests) */ }

  chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
    try {
      switch (msg && msg.type) {
        case "collect":
          sendResponse(collect(msg.scope));
          break;
        case "click":
          sendResponse(click(msg.hint));
          break;
        case "dom":
          sendResponse(dom(msg));
          break;
        case "record_state":
          setRecording(msg.state);
          sendResponse({ ok: true });
          break;
        default:
          return false;
      }
    } catch (e) {
      sendResponse({ ok: false, error: String((e && e.message) || e) });
    }
    return false;
  });
})();
