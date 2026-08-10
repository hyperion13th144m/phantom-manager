"use strict";

// The log pane is the direct descendant of the old WinForms log box: every
// command the manager runs and every line it produces ends up here, so a user
// can paste the whole thing when something goes wrong.

const logEl = document.getElementById("log");
const connEl = document.getElementById("conn");
const versionEl = document.getElementById("version");
const followEl = document.getElementById("follow");
const jobEl = document.getElementById("job");
const cancelEl = document.getElementById("cancel");

// Events are replayed from the server's ring buffer on every (re)connect, so
// track the highest sequence seen to avoid printing duplicates after a reload.
let lastSeq = 0;
let busy = false;

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

  // A finished job changes the repository and the environment, so refresh what
  // the panels show rather than making the user press 再チェック.
  if (ev.kind === "start") setBusy(true, ev.text);
  if (ev.kind === "end") {
    setBusy(false, "");
    refreshAll();
  }
}

// setBusy is the web counterpart of the old manager's SetBusy: while an
// operation runs, nothing else may be started. The server refuses concurrent
// work regardless; this only keeps the UI honest about it.
function setBusy(value, name) {
  busy = value;
  jobEl.textContent = value ? `実行中: ${name}` : "";
  cancelEl.hidden = !value;
  for (const el of document.querySelectorAll(
    "[data-op], #save-env, #use-lan, #recheck, #make-script, #browse-open",
  )) {
    el.disabled = value;
  }
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

// --- 共通 -------------------------------------------------------------------

async function api(path, options) {
  const r = await fetch(path, options);
  const body = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error(body.error ?? `${r.status} ${r.statusText}`);
  return body;
}

async function post(path, payload) {
  return api(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: payload === undefined ? "{}" : JSON.stringify(payload),
  });
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
  try {
    renderChecks(await api("/api/status"));
  } catch (e) {
    checkedAtEl.textContent = `チェックに失敗しました: ${e.message}`;
  }
}

recheckEl.addEventListener("click", loadStatus);

// --- バージョン -------------------------------------------------------------

const repoSummaryEl = document.getElementById("repo-summary");
const tagEl = document.getElementById("tag");

function describeRepo(repo) {
  if (!repo.exists) return `未取得（${repo.dir}）`;
  if (!repo.ready) return `${repo.dir} は phantom-release ではありません`;
  const where = repo.detached
    ? `バージョン ${repo.tag || repo.describe || repo.head}（固定）`
    : `ブランチ ${repo.branch} @ ${repo.head}`;
  return repo.dirty ? `${where} — ローカルに変更あり` : where;
}

async function loadRepo() {
  try {
    const repo = await api("/api/repo");
    repoSummaryEl.textContent = describeRepo(repo);

    const selected = tagEl.value;
    tagEl.replaceChildren();
    for (const tag of repo.tags ?? []) {
      const opt = document.createElement("option");
      opt.value = tag;
      opt.textContent = tag;
      tagEl.append(opt);
    }
    if (!repo.tags?.length) {
      const opt = document.createElement("option");
      opt.value = "";
      opt.textContent = "（バージョン一覧を取得してください）";
      tagEl.append(opt);
    }
    tagEl.value = repo.tag || selected || tagEl.options[0]?.value || "";
  } catch (e) {
    repoSummaryEl.textContent = `取得に失敗しました: ${e.message}`;
  }
}

const REPO_OPS = {
  clone: () => post("/api/repo/clone"),
  pull: () => post("/api/repo/pull"),
  fetch: () => post("/api/repo/fetch"),
  checkout: () => {
    if (!tagEl.value) throw new Error("バージョンを選択してください");
    return post("/api/repo/checkout", { tag: tagEl.value });
  },
  // Checking out a tag detaches HEAD, and pull refuses to run there. This is
  // the way back.
  unpin: () => post("/api/repo/unpin"),
};

for (const button of document.querySelectorAll("[data-op]")) {
  button.addEventListener("click", async () => {
    try {
      await REPO_OPS[button.dataset.op]();
    } catch (e) {
      repoSummaryEl.textContent = e.message;
    }
  });
}

cancelEl.addEventListener("click", () => post("/api/jobs/cancel").catch(() => {}));

// --- データディレクトリ -----------------------------------------------------

const envStatusEl = document.getElementById("env-status");
const fields = ["srcDir", "dataDir", "httpPort", "publicUrl"].map((id) =>
  document.getElementById(id),
);

async function loadEnv() {
  try {
    const { settings, exists, path } = await api("/api/env");
    document.getElementById("srcDir").value = settings.srcDir;
    document.getElementById("dataDir").value = settings.dataDir;
    document.getElementById("httpPort").value = settings.httpPort;
    document.getElementById("publicUrl").value = settings.publicUrl;
    envStatusEl.textContent = exists ? path : `未作成（保存すると ${path} に書き出します）`;
  } catch (e) {
    envStatusEl.textContent = `読み込みに失敗しました: ${e.message}`;
  }
}

document.getElementById("save-env").addEventListener("click", async () => {
  envStatusEl.textContent = "保存中…";
  try {
    const { path } = await post("/api/env", {
      srcDir: document.getElementById("srcDir").value.trim(),
      dataDir: document.getElementById("dataDir").value.trim(),
      httpPort: Number(document.getElementById("httpPort").value),
      publicUrl: document.getElementById("publicUrl").value.trim(),
    });
    envStatusEl.textContent = `保存しました: ${path}`;
    loadStatus();
  } catch (e) {
    envStatusEl.textContent = `保存に失敗しました: ${e.message}`;
  }
});

// PHANTOM_PUBLIC_URL defaults to localhost, which is right when phantom is used
// from this PC only. Reaching it from another machine on the LAN needs the
// Windows host's address, which only Windows can tell us.
document.getElementById("use-lan").addEventListener("click", async () => {
  try {
    const { adapters } = await api("/api/lan-addresses");
    if (!adapters?.length) {
      envStatusEl.textContent = "LAN アドレスを取得できませんでした";
      return;
    }
    const port = document.getElementById("httpPort").value || 8080;
    document.getElementById("publicUrl").value = `http://${adapters[0].ip}:${port}`;
    envStatusEl.textContent = `${adapters[0].alias} の ${adapters[0].ip} を使います（保存すると反映されます）`;
  } catch (e) {
    envStatusEl.textContent = `LAN アドレスの取得に失敗しました: ${e.message}`;
  }
});

// --- 取込スクリプト ---------------------------------------------------------

// The picker walks the Windows filesystem, not /mnt. Mapped network drives —
// which is where the source data lives here — do not appear under /mnt at all,
// and robocopy runs in the Windows session anyway, so Windows paths are the
// only ones that mean anything in the generated script.

const sourceEl = document.getElementById("source");
const pickerEl = document.getElementById("picker");
const pickerPathEl = document.getElementById("picker-path");
const pickerListEl = document.getElementById("picker-list");
const mirrorStatusEl = document.getElementById("mirror-status");
const mirrorResultEl = document.getElementById("mirror-result");

let pickerPath = "";
let pickerParent = "";

async function browse(path) {
  pickerListEl.replaceChildren();
  pickerPathEl.textContent = path || "（ドライブ）";
  mirrorStatusEl.textContent = "読み込み中…";
  try {
    const data = await api(`/api/browse?path=${encodeURIComponent(path ?? "")}`);
    pickerPath = data.path ?? "";
    pickerParent = data.parent ?? "";

    const rows = data.drives
      ? data.drives.map((d) => ({
          // A mapped drive is shown with the share behind it, so P: is not an
          // unexplained letter.
          label: d.network ? `${d.name}: — ${d.unc}` : `${d.name}:`,
          path: d.root,
        }))
      : data.entries.map((e) => ({ label: e.name, path: e.path }));

    if (!rows.length) {
      const li = document.createElement("li");
      li.className = "empty";
      li.textContent = "サブフォルダはありません";
      pickerListEl.append(li);
    }
    for (const row of rows) {
      const li = document.createElement("li");
      const button = document.createElement("button");
      button.type = "button";
      button.textContent = row.label;
      button.addEventListener("click", () => browse(row.path));
      li.append(button);
      pickerListEl.append(li);
    }
    document.getElementById("picker-up").disabled = !path;
    document.getElementById("picker-choose").disabled = !pickerPath;
    mirrorStatusEl.textContent = "";
  } catch (e) {
    mirrorStatusEl.textContent = `フォルダを読み込めませんでした: ${e.message}`;
  }
}

document.getElementById("browse-open").addEventListener("click", () => {
  pickerEl.hidden = false;
  browse(sourceEl.value.trim() || "");
});
document.getElementById("picker-close").addEventListener("click", () => {
  pickerEl.hidden = true;
});
document.getElementById("picker-up").addEventListener("click", () => browse(pickerParent));
document.getElementById("picker-choose").addEventListener("click", () => {
  sourceEl.value = pickerPath;
  pickerEl.hidden = true;
});

function renderMirrorResult(res) {
  mirrorResultEl.hidden = false;
  mirrorResultEl.replaceChildren();

  const rows = [
    ["スクリプト", res.unc],
    ["取込元", res.source],
    ["取込先", res.dest],
    ["ログ", res.log],
    ["対象", res.patterns.join(" ")],
  ];
  for (const [label, value] of rows) {
    const dt = document.createElement("dt");
    dt.textContent = label;
    const dd = document.createElement("dd");
    dd.className = "mono";
    dd.textContent = value;
    mirrorResultEl.append(dt, dd);
  }

  const open = document.createElement("button");
  open.type = "button";
  open.textContent = "保存先を開く";
  open.addEventListener("click", async () => {
    const r = await post("/api/open", { target: res.unc.replace(/\\[^\\]+$/, "") });
    // Interop can be switched off entirely, so the path stays on screen either
    // way for the user to open by hand.
    if (!r.opened) mirrorStatusEl.textContent = `エクスプローラを開けませんでした: ${r.error}`;
  });
  mirrorResultEl.append(open);
}

document.getElementById("make-script").addEventListener("click", async () => {
  mirrorStatusEl.textContent = "作成中…";
  mirrorResultEl.hidden = true;
  try {
    const res = await post("/api/mirror-script", { source: sourceEl.value.trim() });
    mirrorStatusEl.textContent = "作成しました。エクスプローラから実行してください。";
    renderMirrorResult(res);
  } catch (e) {
    mirrorStatusEl.textContent = `作成に失敗しました: ${e.message}`;
  }
});

// --- 起動 -------------------------------------------------------------------

async function loadHealth() {
  try {
    const h = await api("/api/health");
    versionEl.textContent = h.version;
  } catch {
    versionEl.textContent = "";
  }
}

function refreshAll() {
  loadStatus();
  loadRepo();
  loadEnv();
}

document.getElementById("clear").addEventListener("click", () => {
  logEl.replaceChildren();
});

setConnected(false);
loadHealth();
refreshAll();
connect();
