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

// Sends a message to the content script, injecting it first when the tab was
// opened before the extension was installed (no receiver yet).
async function toContent(tab, message) {
  let res;
  try {
    res = await chrome.tabs.sendMessage(tab.id, message);
  } catch (e) {
    log("content script not reachable, injecting:", e && e.message);
    await chrome.scripting.executeScript({ target: { tabId: tab.id }, files: ["content.js"] });
    res = await chrome.tabs.sendMessage(tab.id, message);
  }
  if (!res) throw new Error("empty reply from content script");
  if (res.ok === false) throw new Error(res.error || "content script error");
  return res;
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

const handlers = {
  async ping() {
    return ["pong", { from: "extension" }];
  },

  async collect() {
    const tab = await activeTab();
    return ["elements", await toContent(tab, { type: "collect" })];
  },

  async click({ hint, tag }) {
    const tab = await activeTab();
    const actionId = crypto.randomUUID(); // generated right before the DOM action
    // Only tag requests when the app says its proxy is running: the proxy
    // strips the header, so it never reaches real servers.
    if (tag) await armActionHeader(tab.id, actionId);
    let res;
    try {
      res = await toContent(tab, { type: "click", hint });
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

chrome.runtime.onStartup.addListener(connect);
chrome.runtime.onInstalled.addListener(connect);
connect();
