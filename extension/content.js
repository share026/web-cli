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

  function describe(el) {
    const tag = el.tagName.toLowerCase();
    let text = "";
    if (el instanceof HTMLInputElement) {
      text = ["button", "submit", "reset"].includes(el.type)
        ? el.value
        : el.getAttribute("aria-label") || el.placeholder || el.name || el.value;
    } else if (el instanceof HTMLSelectElement) {
      text = (el.selectedOptions[0] && el.selectedOptions[0].text) || el.name;
    } else if (el instanceof HTMLTextAreaElement) {
      text = el.getAttribute("aria-label") || el.placeholder || el.name;
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
    };
  }

  // --- commands -----------------------------------------------------------

  function collect() {
    for (const old of document.querySelectorAll(`[${HINT_ATTR}]`)) old.removeAttribute(HINT_ATTR);
    hints = new Map();
    const elements = [];
    let n = 0;
    for (const el of document.querySelectorAll(SELECTOR)) {
      const r = isHintable(el);
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
    return { ok: true, kind, tag: d.tag, text: d.text, url: location.href };
  }

  chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
    try {
      switch (msg && msg.type) {
        case "collect":
          sendResponse(collect());
          break;
        case "click":
          sendResponse(click(msg.hint));
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
