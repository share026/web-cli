// web-cli bridge — MV3 service worker.
// Relays messages between the Go native host (nm-host -> app) and the
// content script. Keeps only the logic that must live in the browser:
// chrome.* API calls. All decisions and persistence happen in Go.
"use strict";

const HOST_NAME = "com.share026.webcli";
const ACTION_HEADER = "X-Audit-Action-Id";
const ACTION_RULE_ID = 1;
const ACTION_WINDOW_MS = 3000; // requests fired within this window after a click carry the action ID
const ALL_RESOURCE_TYPES = [
  "main_frame", "sub_frame", "stylesheet", "script", "image", "font", "object",
  "xmlhttprequest", "ping", "csp_report", "media", "websocket", "webtransport", "webbundle", "other",
];

let port = null;
let retryMs = 1000;
let disarmTimer = null;

const log = (...args) => console.log("[web-cli]", ...args);
const newId = () => crypto.randomUUID().replace(/-/g, "");

function connect() {
  if (port) return;
  try {
    port = chrome.runtime.connectNative(HOST_NAME);
  } catch (e) {
    log("connectNative failed:", e && e.message);
    port = null;
    scheduleReconnect();
    return;
  }
  port.onMessage.addListener(onNativeMessage);
  port.onDisconnect.addListener(() => {
    const err = chrome.runtime.lastError;
    log("native host disconnected:", err ? err.message : "(no error)");
    port = null;
    scheduleReconnect();
  });
  send({ id: newId(), type: "ping", payload: { from: "extension", version: chrome.runtime.getManifest().version } });
}

function scheduleReconnect() {
  setTimeout(connect, retryMs);
  retryMs = Math.min(retryMs * 2, 30000);
}

function send(msg) {
  if (!port) return false;
  port.postMessage(msg);
  return true;
}

async function activeTab() {
  const [tab] = await chrome.tabs.query({ active: true, lastFocusedWindow: true });
  if (!tab || tab.id === undefined) throw new Error("no active tab");
  return tab;
}

// Sends a message to the content script of one frame (default: the top
// frame), injecting it first when the tab was opened before the extension was
// installed (no receiver yet). The frame ID is always explicit: content.js
// runs in every frame, and an untargeted message would be answered by
// whichever frame replies first.
async function toContent(tab, message, frameId = 0) {
  let res;
  try {
    res = await chrome.tabs.sendMessage(tab.id, message, { frameId });
  } catch (e) {
    log(`content script of frame ${frameId} not reachable, injecting:`, e && e.message);
    await chrome.scripting.executeScript({ target: { tabId: tab.id, frameIds: [frameId] }, files: ["content.js"] });
    res = await chrome.tabs.sendMessage(tab.id, message, { frameId });
  }
  if (!res) throw new Error("empty reply from content script");
  if (res.ok === false) throw new Error(res.error || "content script error");
  return res;
}

const MUTATING_OPS = new Set(["click", "type", "clear", "select", "check", "press", "submit", "upload"]);

function waitComplete(tabId, timeoutMs) {
  return new Promise((resolve, reject) => {
    let started = false;
    // back/forward from the back-forward cache may not report "loading":
    // on timeout accept a tab that is complete anyway.
    const timer = setTimeout(async () => {
      chrome.tabs.onUpdated.removeListener(fn);
      const tab = await chrome.tabs.get(tabId).catch(() => null);
      if (tab && tab.status === "complete") resolve(tab);
      else reject(new Error("navigation timed out"));
    }, timeoutMs);
    function fn(id, info, tab) {
      if (id !== tabId) return;
      if (info.status === "loading") started = true;
      if (info.status === "complete" && started) {
        clearTimeout(timer);
        chrome.tabs.onUpdated.removeListener(fn);
        resolve(tab);
      }
    }
    chrome.tabs.onUpdated.addListener(fn);
  });
}

// Back/forward exactly one history entry. chrome.tabs.goBack follows the
// browser's Back button, which skips entries the user never interacted with
// (Chrome's history manipulation intervention) - every page driven by
// automation qualifies, so it would jump several pages back. The page's own
// history.back() steps one entry; tabs.goBack is the fallback for pages that
// cannot be scripted.
async function historyGo(tabId, action) {
  try {
    await chrome.scripting.executeScript({
      target: { tabId }, world: "MAIN", args: [action],
      func: (a) => { setTimeout(() => (a === "back" ? history.back() : history.forward()), 0); },
    });
  } catch (e) {
    log("history via page failed, using tabs API:", e && e.message);
    if (action === "back") await chrome.tabs.goBack(tabId);
    else await chrome.tabs.goForward(tabId);
  }
}

// Runs in the page (world MAIN). Returns a JSON-safe result.
async function pageEval(code) {
  const out = (v) => {
    if (v === undefined) return { ok: true, type: "undefined" };
    if (v instanceof Element) return { ok: true, type: "element", value: v.outerHTML.slice(0, 2000) };
    try { return { ok: true, type: typeof v, value: JSON.parse(JSON.stringify(v)) }; }
    catch { return { ok: true, type: typeof v, value: String(v) }; }
  };
  let v;
  try {
    v = (0, eval)(code); // indirect eval: global scope, like the DevTools console
  } catch (e) {
    if (e instanceof EvalError || /Content Security Policy|unsafe-eval/i.test(String(e))) {
      return { ok: false, csp: true, error: String(e) };
    }
    return { ok: false, error: String((e && e.stack) || e) };
  }
  try { return out(await v); } catch (e) { return { ok: false, error: String((e && e.stack) || e) }; }
}

async function withDebugger(tabId, fn) {
  const target = { tabId };
  await chrome.debugger.attach(target, "1.3");
  try { return await fn(target); } finally { await chrome.debugger.detach(target).catch(() => {}); }
}

async function debuggerEval(tabId, code) {
  return withDebugger(tabId, async (target) => {
    const r = await chrome.debugger.sendCommand(target, "Runtime.evaluate", {
      expression: code, awaitPromise: true, returnByValue: true, userGesture: true, replMode: true,
    });
    if (r.exceptionDetails) {
      return { ok: false, error: (r.exceptionDetails.exception && r.exceptionDetails.exception.description) || r.exceptionDetails.text };
    }
    return { ok: true, type: r.result.type, value: r.result.value ?? r.result.description };
  });
}

// Tag every request the tab issues during the next ACTION_WINDOW_MS with the
// action ID, so the Go proxy can link traffic to the DOM operation.
async function armActionHeader(tabId, actionId) {
  clearTimeout(disarmTimer);
  await chrome.declarativeNetRequest.updateSessionRules({
    removeRuleIds: [ACTION_RULE_ID],
    addRules: [{
      id: ACTION_RULE_ID,
      priority: 1,
      action: {
        type: "modifyHeaders",
        requestHeaders: [{ header: ACTION_HEADER, operation: "set", value: actionId }],
      },
      condition: { tabIds: [tabId], resourceTypes: ALL_RESOURCE_TYPES },
    }],
  });
  disarmTimer = setTimeout(disarmActionHeader, ACTION_WINDOW_MS);
}

async function disarmActionHeader() {
  clearTimeout(disarmTimer);
  await chrome.declarativeNetRequest.updateSessionRules({ removeRuleIds: [ACTION_RULE_ID] });
}

// --- frames -------------------------------------------------------------------
// Hints are numbered per frame by content.js. collectAll merges the top frame
// with the visible child frames it reports (recursively, offsetting rects into
// top-level viewport coordinates) and renumbers them; frameHints maps the
// global number back to (frameId, local hint) for the following DOM op.

let frameHints = { tabId: -1, map: new Map() };
const MAX_FRAME_DEPTH = 6;

// token -> { tabId, frameId }, filled by frame_token messages from child
// frames (content.js posts a token into each iframe it reports).
const tokenFrames = new Map();

async function frameIdFor(tabId, token, waitMs = 700) {
  const until = Date.now() + waitMs;
  for (;;) {
    const m = tokenFrames.get(token);
    if (m && m.tabId === tabId) return m.frameId;
    if (Date.now() > until) return null;
    await new Promise((r) => setTimeout(r, 20));
  }
}

// Resolves the tokens of reported child frames in parallel; frames whose
// content script never answered (not scriptable) are dropped.
async function withFrameIds(tabId, frames) {
  const ids = await Promise.all((frames || []).map((f) => frameIdFor(tabId, f.token)));
  return (frames || []).map((f, i) => ({ ...f, frame_id: ids[i] })).filter((f) => f.frame_id !== null);
}

function intersect(a, b) {
  if (!a) return b;
  if (!b) return a;
  const x = Math.max(a.x, b.x), y = Math.max(a.y, b.y);
  const w = Math.min(a.x + a.w, b.x + b.w) - x, h = Math.min(a.y + a.h, b.y + b.h) - y;
  return w > 0 && h > 0 ? { x, y, w, h } : null;
}

async function collectAll(tab, scope) {
  const top = await toContent(tab, { type: "collect", scope }, 0);
  const map = new Map();
  const elements = [];
  let n = 0;
  const add = (els, frameId, off, clip, frameUrl) => {
    for (const e of els || []) {
      const rect = { x: e.rect.x + off.x, y: e.rect.y + off.y, w: e.rect.w, h: e.rect.h };
      if (clip && !intersect(clip, rect)) continue;
      n++;
      map.set(n, { frameId, hint: e.hint });
      elements.push({ ...e, hint: n, rect, frame: frameId ? frameUrl : undefined });
    }
  };
  add(top.elements, 0, { x: 0, y: 0 }, null);
  const viewportScope = scope !== "page";
  const queue = (await withFrameIds(tab.id, top.frames)).map((f) => ({ ...f, depth: 1 }));
  while (queue.length) {
    const f = queue.shift();
    if (f.depth > MAX_FRAME_DEPTH || (viewportScope && !f.clip)) continue;
    let res;
    try {
      res = await toContent(tab, { type: "collect", scope }, f.frame_id);
    } catch (e) {
      log(`frame ${f.frame_id} not collectable:`, e && e.message);
      continue;
    }
    add(res.elements, f.frame_id, f.box, viewportScope ? f.clip : null, res.url);
    for (const c of await withFrameIds(tab.id, res.frames)) {
      const box = { x: f.box.x + c.box.x, y: f.box.y + c.box.y };
      const clip = viewportScope ? intersect(f.clip, { x: f.box.x + c.clip.x, y: f.box.y + c.clip.y, w: c.clip.w, h: c.clip.h }) : null;
      queue.push({ frame_id: c.frame_id, box, clip, depth: f.depth + 1 });
    }
  }
  frameHints = { tabId: tab.id, map };
  return { ok: true, url: top.url, title: top.title, elements };
}

// All frame IDs of the tab in document order (top first), via content.js so
// only frames that are actually rendered in the page are included.
async function allFrames(tab) {
  const ids = [0];
  for (let i = 0; i < ids.length && ids.length < 50; i++) {
    try {
      const r = await toContent(tab, { type: "dom", op: "frames" }, ids[i]);
      for (const f of await withFrameIds(tab.id, r.frames)) if (!ids.includes(f.frame_id)) ids.push(f.frame_id);
    } catch { /* frame gone or not scriptable */ }
  }
  return ids;
}

// Frame that currently has the keyboard focus (follows focused iframes down).
async function focusedFrame(tab) {
  let id = 0;
  for (let depth = 0; depth < MAX_FRAME_DEPTH; depth++) {
    const r = await toContent(tab, { type: "dom", op: "focus_frame" }, id).catch(() => null);
    const next = r && r.token ? await frameIdFor(tab.id, r.token) : null;
    if (next === null) break;
    id = next;
  }
  return id;
}

// Runs a dom op in the right frame: hints via frameHints, css= targets in the
// first frame where the selector matches, target-less press/submit in the
// focused frame, everything else in the top frame.
async function domInFrames(tab, p) {
  const msg = { ...p, type: "dom" };
  const t = p.target;
  if (t && t.hint !== undefined) {
    const m = frameHints.tabId === tab.id && frameHints.map.get(Number(t.hint));
    if (m) return { res: await toContent(tab, { ...msg, target: { hint: m.hint } }, m.frameId), frameId: m.frameId };
    return { res: await toContent(tab, msg, 0), frameId: 0 };
  }
  if ((t && t.css) || (p.op === "exists" && p.text !== undefined)) {
    let first;
    for (const id of await allFrames(tab)) {
      try {
        const res = await toContent(tab, msg, id);
        if (p.op !== "exists" || res.found) return { res, frameId: id };
        first = first || { res, frameId: id };
      } catch (e) {
        if (!/not found: css=/.test(String(e && e.message))) throw e;
        first = first || { err: e };
      }
    }
    if (first.err) throw first.err;
    return first;
  }
  if (!t && (p.op === "press" || p.op === "submit")) {
    const id = await focusedFrame(tab);
    return { res: await toContent(tab, msg, id), frameId: id };
  }
  return { res: await toContent(tab, msg, 0), frameId: 0 };
}

// --- file uploads ------------------------------------------------------------
// The host may send at most 1 MB per native message, so files arrive as
// base64 chunks (each a multiple of 3 raw bytes, so the base64 parts simply
// concatenate) and are assembled here until the 'upload' dom op uses them.

const uploads = new Map(); // upload_id -> { files: [{name, mime, last_modified, parts: []}], at }
const UPLOAD_TTL_MS = 10 * 60 * 1000;

function uploadFiles(id) {
  const u = uploads.get(id);
  if (!u) throw new Error(`upload ${id} is unknown or expired; run the upload command again`);
  return u.files.map((f) => ({ name: f.name, mime: f.mime, last_modified: f.last_modified, data: f.parts.join("") }));
}

const handlers = {
  async ping() {
    return ["pong", { from: "extension" }];
  },

  async collect({ scope } = {}) {
    const tab = await activeTab();
    return ["elements", await collectAll(tab, scope)];
  },

  async upload_chunk({ upload_id: id, file, name, mime, last_modified: lastModified, index, data }) {
    const now = Date.now();
    for (const [k, v] of uploads) if (now - v.at > UPLOAD_TTL_MS) uploads.delete(k);
    let u = uploads.get(id);
    if (!u) uploads.set(id, (u = { files: [], at: now }));
    u.at = now;
    const f = (u.files[file] = u.files[file] || { name, mime, last_modified: lastModified, parts: [] });
    f.parts[index] = data || "";
    return ["upload_chunk_result", { upload_id: id, file, index, received: data ? data.length : 0 }];
  },

  async timing() {
    const tab = await activeTab();
    const res = await toContent(tab, { type: "dom", op: "timing" }, 0);
    delete res.ok;
    return ["timing_result", res];
  },

  // DOM operations in the active tab (click/type/select/check/press/submit/
  // scroll/text/html/exists/info/storage). Page-changing ops get an action
  // ID, armed as a request header exactly like clicks.
  async dom(p) {
    const tab = await activeTab();
    const mutating = MUTATING_OPS.has(p.op);
    const actionId = mutating ? crypto.randomUUID() : undefined;
    if (mutating && p.tag) await armActionHeader(tab.id, actionId);
    let res, frameId;
    try {
      if (p.op === "upload") p = { ...p, files: uploadFiles(p.upload_id) };
      ({ res, frameId } = await domInFrames(tab, p));
      if (p.op === "upload") uploads.delete(p.upload_id); // kept until success: the app retries while the target is missing
    } catch (e) {
      if (mutating && p.tag) await disarmActionHeader();
      throw e;
    }
    delete res.ok;
    return ["dom_result", { ...res, op: p.op, action_id: actionId, frame_id: frameId || undefined }];
  },

  // open / back / forward / reload in the active tab (or a new tab), waiting
  // until the tab has finished loading.
  async navigate({ action, url, new_tab: newTab, tag, timeout_ms: timeoutMs }) {
    // history navigations are usually served from the bfcache: shorter wait
    if (!timeoutMs && (action === "back" || action === "forward")) timeoutMs = 5000;
    let tab = await activeTab().catch(() => null);
    const actionId = crypto.randomUUID();
    if (newTab) {
      tab = await chrome.tabs.create({ url: "about:blank", active: true });
    }
    if (!tab) throw new Error("no tab");
    if (tag) await armActionHeader(tab.id, actionId);
    const done = waitComplete(tab.id, timeoutMs || 15000);
    if (action === "open") await chrome.tabs.update(tab.id, { url });
    else if (action === "back" || action === "forward") await historyGo(tab.id, action);
    else if (action === "reload") await chrome.tabs.reload(tab.id);
    else throw new Error(`unknown navigation ${action}`);
    const t = await done;
    // The page itself is authoritative (tab.url can lag behind for pages
    // restored from the back-forward cache).
    let { url: u, title } = t;
    try {
      const info = await toContent(t, { type: "dom", op: "info" });
      u = info.url;
      title = info.title;
    } catch { /* not scriptable (chrome://, PDF, ...) */ }
    return ["nav_result", { action, action_id: actionId, tab_id: t.id, url: u, title }];
  },

  async tabs({ op, id }) {
    if (op === "select") {
      const t = await chrome.tabs.update(id, { active: true });
      await chrome.windows.update(t.windowId, { focused: true }).catch(() => {});
    } else if (op === "close") {
      await chrome.tabs.remove(id ?? (await activeTab()).id);
    }
    const all = await chrome.tabs.query({});
    const cur = await activeTab().catch(() => null);
    return ["tabs_result", {
      tabs: all.map((t) => ({ id: t.id, window_id: t.windowId, active: !!cur && t.id === cur.id, url: t.url, title: t.title })),
    }];
  },

  // Evaluate JavaScript in the page's main world. Pages whose CSP forbids
  // eval are handled through the DevTools protocol (chrome.debugger).
  async eval({ code }) {
    const tab = await activeTab();
    let r;
    try {
      [{ result: r }] = await chrome.scripting.executeScript({
        target: { tabId: tab.id }, world: "MAIN", func: pageEval, args: [code],
      });
    } catch (e) {
      r = { ok: false, csp: true, error: String((e && e.message) || e) };
    }
    if (r && r.csp) {
      r = await debuggerEval(tab.id, code);
      r.via = "debugger";
    } else if (r) {
      r.via = "main-world";
    }
    if (!r.ok) throw new Error(r.error);
    return ["eval_result", r];
  },

  async screenshot() {
    const tab = await activeTab();
    let data;
    try {
      const url = await chrome.tabs.captureVisibleTab(tab.windowId, { format: "png" });
      data = url.slice(url.indexOf(",") + 1);
    } catch (e) {
      log("captureVisibleTab failed, using the debugger:", e && e.message);
      data = await withDebugger(tab.id, async (target) =>
        (await chrome.debugger.sendCommand(target, "Page.captureScreenshot", { format: "png" })).data);
    }
    return ["screenshot_result", { url: tab.url, title: tab.title, png_base64: data }];
  },

  async record({ on, secrets }) {
    const state = { on: !!on, secrets: !!secrets };
    await chrome.storage.session.set({ record: state });
    for (const t of await chrome.tabs.query({})) {
      chrome.tabs.sendMessage(t.id, { type: "record_state", state }).catch(() => {});
    }
    const tab = await activeTab().catch(() => null);
    return ["record_result", { ...state, url: tab && tab.url, title: tab && tab.title }];
  },

  async click({ hint, tag }) {
    const tab = await activeTab();
    const actionId = crypto.randomUUID(); // generated right before the DOM action
    // Only tag requests when the app says its proxy is running: the proxy
    // strips the header, so it never reaches real servers.
    if (tag) await armActionHeader(tab.id, actionId);
    let res;
    try {
      const m = frameHints.tabId === tab.id && frameHints.map.get(Number(hint));
      res = m ? await toContent(tab, { type: "click", hint: m.hint }, m.frameId) : await toContent(tab, { type: "click", hint });
    } catch (e) {
      if (tag) await disarmActionHeader();
      throw e;
    }
    return ["click_result", { action_id: actionId, kind: res.kind, hint, tag: res.tag, text: res.text, url: res.url }];
  },

  async cookies_get({ url }) {
    const target = url || (await activeTab()).url;
    const cookies = await chrome.cookies.getAll({ url: target });
    return ["cookies", { url: target, cookies }];
  },

  async cookies_set({ cookies }) {
    let set = 0;
    const errors = [];
    for (const details of cookies || []) {
      try {
        const c = await chrome.cookies.set(details);
        if (c) set++;
        else errors.push(`${details.name}: ${(chrome.runtime.lastError && chrome.runtime.lastError.message) || "rejected"}`);
      } catch (e) {
        errors.push(`${details.name}: ${e && e.message}`);
      }
    }
    return ["cookies_set_result", { set, errors }];
  },
};

async function onNativeMessage(msg) {
  if (msg.type === "pong") {
    retryMs = 1000;
    log("pong received from app", JSON.stringify(msg.payload || {}));
    send({ type: "log", payload: { event: "pong_received", reply_to: msg.id } });
    return;
  }
  if (msg.type === "error") {
    log("native host error:", msg.error);
    return;
  }
  const handler = handlers[msg.type];
  if (!handler) {
    if (msg.id) send({ id: msg.id, type: "error", error: `unsupported message type: ${msg.type}` });
    return;
  }
  try {
    const [type, payload] = await handler(msg.payload || {});
    send({ id: msg.id, type, payload });
  } catch (e) {
    send({ id: msg.id, type: "error", error: String((e && e.message) || e) });
  }
}

// Messages from content scripts: recording state and recorded user input.
chrome.runtime.onMessage?.addListener((msg, sender, sendResponse) => {
  if (msg && msg.type === "record_state") {
    chrome.storage.session.get("record").then((v) => sendResponse(v.record || { on: false }), () => sendResponse({ on: false }));
    return true;
  }
  if (msg && msg.type === "frame_token" && sender.tab && typeof sender.frameId === "number") {
    tokenFrames.set(msg.token, { tabId: sender.tab.id, frameId: sender.frameId });
    if (tokenFrames.size > 1000) tokenFrames.delete(tokenFrames.keys().next().value);
    return false;
  }
  if (msg && msg.type === "record_event") {
    send({ type: "record_event", payload: { ...msg.event, tab_id: sender.tab && sender.tab.id } });
  }
  return false;
});

chrome.runtime.onStartup.addListener(connect);
chrome.runtime.onInstalled.addListener(connect);
connect();
