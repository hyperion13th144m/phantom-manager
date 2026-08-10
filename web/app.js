"use strict";

// The log pane is the direct descendant of the old WinForms log box: every
// command the manager runs and every line it produces ends up here, so a user
// can paste the whole thing when something goes wrong.

const logEl = document.getElementById("log");
const connEl = document.getElementById("conn");
const versionEl = document.getElementById("version");
const followEl = document.getElementById("follow");

// Events are replayed from the server's ring buffer on every (re)connect, so
// track the highest sequence seen to avoid printing duplicates after a reload.
let lastSeq = 0;

function appendEvent(ev) {
  if (ev.seq <= lastSeq) return;
  lastSeq = ev.seq;

  const line = document.createElement("div");
  line.className = `line k-${ev.kind}`;

  const ts = document.createElement("span");
  ts.className = "ts";
  ts.textContent = ev.time;

  const msg = document.createElement("span");
  msg.className = "msg";
  msg.textContent = ev.kind === "cmd" ? `> ${ev.text}` : ev.text;

  line.append(ts, msg);
  logEl.append(line);

  if (followEl.checked) logEl.scrollTop = logEl.scrollHeight;
}

function setConnected(ok) {
  connEl.textContent = ok ? "接続中" : "未接続";
  connEl.className = `badge ${ok ? "badge-on" : "badge-off"}`;
}

function connect() {
  const es = new EventSource("/api/events");
  es.onopen = () => setConnected(true);
  es.onmessage = (e) => {
    try {
      appendEvent(JSON.parse(e.data));
    } catch {
      // A malformed frame should not kill the stream.
    }
  };
  // EventSource reconnects on its own; just reflect the state.
  es.onerror = () => setConnected(false);
}

// --- 環境チェック -----------------------------------------------------------

const checksEl = document.getElementById("checks");
const checkedAtEl = document.getElementById("checked-at");
const recheckEl = document.getElementById("recheck");

const STATE_MARK = { ok: "○", ng: "✕", warn: "!", unset: "—" };

function renderChecks(status) {
  checksEl.replaceChildren();
  for (const c of status.checks) {
    const li = document.createElement("li");
    li.className = `check s-${c.state}`;

    const mark = document.createElement("span");
    mark.className = "mark";
    mark.textContent = STATE_MARK[c.state] ?? "?";

    const label = document.createElement("span");
    label.className = "label";
    label.textContent = c.label;

    const detail = document.createElement("span");
    detail.className = "detail";
    detail.textContent = c.detail;

    li.append(mark, label, detail);

    if (c.hint) {
      const hint = document.createElement("div");
      hint.className = "hint";
      hint.textContent = c.hint;
      li.append(hint);
    }
    checksEl.append(li);
  }
  checkedAtEl.textContent = new Date(status.checkedAt).toLocaleTimeString("ja-JP");
}

async function loadStatus() {
  recheckEl.disabled = true;
  try {
    const r = await fetch("/api/status");
    renderChecks(await r.json());
  } catch (e) {
    checkedAtEl.textContent = `チェックに失敗しました: ${e}`;
  } finally {
    recheckEl.disabled = false;
  }
}

recheckEl.addEventListener("click", loadStatus);

async function loadHealth() {
  try {
    const r = await fetch("/api/health");
    const h = await r.json();
    versionEl.textContent = h.version;
  } catch {
    versionEl.textContent = "";
  }
}

document.getElementById("clear").addEventListener("click", () => {
  logEl.replaceChildren();
});

setConnected(false);
loadHealth();
loadStatus();
connect();
