"use strict";

// The whole UI is drawn from one /api/state snapshot. Fetching the panels
// separately let them disagree — a service table from before an operation next
// to a repository panel from after it — and each answer carried its own idea of
// what was allowed. One request, one consistent picture.
//
// Which buttons are live is decided by the server and applied here. The old
// manager's SetBusy did the same job in the form, but a web UI has second tabs
// and reloads, so the rules are enforced server-side too; this is presentation.

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

const $ = (id) => document.getElementById(id);

// --- ログ -------------------------------------------------------------------

// Every command the manager runs and every line it produces lands here, so a
// user can paste the whole thing when something goes wrong. Carried over from
// the old WinForms log box.

const logEl = $("log");
const followEl = $("follow");

// Events are replayed from the server's ring buffer on every reconnect, so
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

  // A job boundary changes what is allowed and what the panels should show.
  if (ev.kind === "start" || ev.kind === "end") refresh();
}

function setConnected(ok) {
  const el = $("conn");
  el.textContent = ok ? "接続中" : "未接続";
  el.className = `badge ${ok ? "badge-on" : "badge-off"}`;
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
  es.onerror = () => setConnected(false); // EventSource reconnects on its own
}

// --- 環境チェック -----------------------------------------------------------

const STATE_MARK = { ok: "○", ng: "✕", warn: "!", unset: "—" };

function renderChecks(checks) {
  const list = $("checks");
  list.replaceChildren();
  for (const c of checks.checks) {
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
    list.append(li);
  }
  $("checked-at").textContent = new Date(checks.checkedAt).toLocaleTimeString("ja-JP");
}

// --- バージョン -------------------------------------------------------------

function describeRepo(repo) {
  if (!repo.exists) return `未取得（${repo.dir}）`;
  if (!repo.ready) return `${repo.dir} は phantom-release ではありません`;
  const where = repo.detached
    ? `バージョン ${repo.tag || repo.describe || repo.head} を固定中`
    : `ブランチ ${repo.branch} @ ${repo.head}${repo.tag ? `（${repo.tag}）` : ""}`;
  return repo.dirty ? `${where} — ローカルに変更あり` : where;
}

function renderRepo(repo) {
  $("repo-summary").textContent = describeRepo(repo);

  const tagEl = $("tag");
  const keep = tagEl.value;
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
  tagEl.value = repo.tag || keep || tagEl.options[0]?.value || "";
}

// --- データディレクトリ -----------------------------------------------------

// The form is only refilled when it is not being edited, so a background
// refresh cannot overwrite half-typed input.
let envDirty = false;

function renderEnv(state) {
  if (!envDirty) {
    $("srcDir").value = state.env.srcDir;
    $("dataDir").value = state.env.dataDir;
    $("httpPort").value = state.env.httpPort;
    $("publicUrl").value = state.env.publicUrl;
  }
  $("env-status").textContent = state.envSaved
    ? state.envPath
    : `未作成（保存すると ${state.envPath} に書き出します）`;
}

for (const id of ["srcDir", "dataDir", "httpPort", "publicUrl"]) {
  $(id).addEventListener("input", () => {
    envDirty = true;
  });
}

// --- サービス ---------------------------------------------------------------

function renderServices(state) {
  $("compose-error").textContent = state.composeError ?? "";

  const services = state.services ?? [];
  const url = $("phantom-url");
  const running = services.some((s) => s.running);
  url.textContent = running ? state.publicUrl : "";
  url.href = state.publicUrl || "#";
  url.hidden = !running;

  const body = $("services");
  body.replaceChildren();
  if (!services.length) {
    const tr = document.createElement("tr");
    const td = document.createElement("td");
    td.colSpan = 4;
    td.className = "muted";
    td.textContent = state.composeError ? "" : "コンテナはありません";
    tr.append(td);
    body.append(tr);
    return;
  }
  for (const svc of services) {
    const tr = document.createElement("tr");
    for (const [value, cls] of [
      [svc.name, "name"],
      [svc.health ? `${svc.state} (${svc.health})` : svc.state, svc.running ? "s-ok" : "s-ng"],
      [svc.status, "muted"],
      [svc.ports, "mono"],
    ]) {
      const td = document.createElement("td");
      td.className = cls;
      td.textContent = value ?? "";
      tr.append(td);
    }
    body.append(tr);
  }
}

// --- 取込スクリプト ---------------------------------------------------------

// The picker walks the Windows filesystem, not /mnt: mapped network drives do
// not appear under /mnt at all, and robocopy runs in the Windows session, so
// Windows paths are the only ones the generated script can use.

let pickerPath = "";
let pickerParent = "";

async function browse(path) {
  const list = $("picker-list");
  list.replaceChildren();
  $("picker-path").textContent = path || "（ドライブ）";
  $("mirror-status").textContent = "読み込み中…";
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
      list.append(li);
    }
    for (const row of rows) {
      const li = document.createElement("li");
      const button = document.createElement("button");
      button.type = "button";
      button.textContent = row.label;
      button.addEventListener("click", () => browse(row.path));
      li.append(button);
      list.append(li);
    }
    $("picker-up").disabled = !path;
    $("picker-choose").disabled = !pickerPath;
    $("mirror-status").textContent = "";
  } catch (e) {
    $("mirror-status").textContent = `フォルダを読み込めませんでした: ${e.message}`;
  }
}

function renderMirrorResult(res) {
  const el = $("mirror-result");
  el.hidden = false;
  el.replaceChildren();

  for (const [label, value] of [
    ["スクリプト", res.unc],
    ["取込元", res.source],
    ["取込先", res.dest],
    ["ログ", res.log],
    ["対象", res.patterns.join(" ")],
  ]) {
    const dt = document.createElement("dt");
    dt.textContent = label;
    const dd = document.createElement("dd");
    dd.className = "mono";
    dd.textContent = value;
    el.append(dt, dd);
  }

  const open = document.createElement("button");
  open.type = "button";
  open.textContent = "保存先を開く";
  open.addEventListener("click", async () => {
    const r = await post("/api/open", { target: res.unc.replace(/\\[^\\]+$/, "") });
    // Interop can be switched off entirely, so the path stays on screen either
    // way for the user to open by hand.
    if (!r.opened) $("mirror-status").textContent = `エクスプローラを開けませんでした: ${r.error}`;
  });
  el.append(open);
}

// --- 操作 -------------------------------------------------------------------

// Each entry is keyed by the action name the server uses, which is also the
// button's data-action and the endpoint's suffix.
const ACTIONS = {
  "repo/clone": () => post("/api/repo/clone"),
  "repo/pull": () => post("/api/repo/pull"),
  "repo/fetch": () => post("/api/repo/fetch"),
  "repo/unpin": () => post("/api/repo/unpin"),
  "repo/checkout": () => {
    if (!$("tag").value) throw new Error("バージョンを選択してください");
    return post("/api/repo/checkout", { tag: $("tag").value });
  },
  "compose/build": () => post("/api/compose/build"),
  "compose/pull": () => post("/api/compose/pull"),
  "compose/up": () => post("/api/compose/up"),
  "compose/down": () => post("/api/compose/down"),
  // The only irreversible button on the page, so it asks first. The index is
  // rebuilt from PHANTOM_DATA_DIR, which this does not touch.
  "compose/es-volume-rm": () => {
    const ok = confirm(
      "Elasticsearch のデータ (ボリューム) を削除します。\n" +
        "検索インデックスは失われ、次回の起動後に作り直しになります。\n\n" +
        "実行しますか？",
    );
    if (!ok) return;
    return post("/api/compose/es-volume-rm");
  },
  "mirror/browse": () => {
    $("picker").hidden = false;
    return browse($("source").value.trim() || "");
  },
  "mirror/create": async () => {
    $("mirror-status").textContent = "作成中…";
    $("mirror-result").hidden = true;
    const res = await post("/api/mirror-script", { source: $("source").value.trim() });
    $("mirror-status").textContent = "作成しました。エクスプローラから実行してください。";
    renderMirrorResult(res);
  },
  "env/save": async () => {
    $("env-status").textContent = "保存中…";
    const { path } = await post("/api/env", {
      srcDir: $("srcDir").value.trim(),
      dataDir: $("dataDir").value.trim(),
      httpPort: Number($("httpPort").value),
      publicUrl: $("publicUrl").value.trim(),
    });
    envDirty = false;
    $("env-status").textContent = `保存しました: ${path}`;
    refresh();
  },
};

// Where an action reports its own failure, so an error lands next to the
// control that caused it rather than only in the log.
const ERROR_TARGET = {
  "repo/": "repo-summary",
  "env/": "env-status",
  "mirror/": "mirror-status",
  "compose/": "compose-error",
};

function reportError(action, message) {
  for (const [prefix, id] of Object.entries(ERROR_TARGET)) {
    if (action.startsWith(prefix)) {
      $(id).textContent = message;
      return;
    }
  }
}

for (const button of document.querySelectorAll("[data-action]")) {
  button.addEventListener("click", async () => {
    const action = button.dataset.action;
    try {
      await ACTIONS[action]();
    } catch (e) {
      reportError(action, e.message);
      // The server may have refused on a condition the page had not seen yet.
      refresh();
    }
  });
}

$("picker-close").addEventListener("click", () => {
  $("picker").hidden = true;
});
$("picker-up").addEventListener("click", () => browse(pickerParent));
$("picker-choose").addEventListener("click", () => {
  $("source").value = pickerPath;
  $("picker").hidden = true;
});
$("cancel").addEventListener("click", () => post("/api/jobs/cancel").catch(() => {}));
$("clear").addEventListener("click", () => logEl.replaceChildren());
$("refresh").addEventListener("click", refresh);

// PHANTOM_PUBLIC_URL defaults to localhost, which is right when phantom is used
// from this PC only. Reaching it from another machine needs the Windows host's
// address, which only Windows can tell us.
$("use-lan").addEventListener("click", async () => {
  try {
    const { adapters } = await api("/api/lan-addresses");
    if (!adapters?.length) {
      $("env-status").textContent = "LAN アドレスを取得できませんでした";
      return;
    }
    const port = $("httpPort").value || 8080;
    $("publicUrl").value = `http://${adapters[0].ip}:${port}`;
    envDirty = true;
    $("env-status").textContent = `${adapters[0].alias} の ${adapters[0].ip} を使います（保存すると反映されます）`;
  } catch (e) {
    $("env-status").textContent = `LAN アドレスの取得に失敗しました: ${e.message}`;
  }
});

// --- 状態の反映 -------------------------------------------------------------

function applyCapabilities(can, busy) {
  for (const button of document.querySelectorAll("[data-action]")) {
    const cap = can[button.dataset.action];
    if (!cap) continue;
    button.disabled = !cap.allowed;
    // A greyed-out button that explains itself on hover beats a dead one.
    button.title = cap.allowed ? "" : cap.reason;
  }
  $("refresh").disabled = busy;
  $("use-lan").disabled = busy;
  for (const id of ["srcDir", "dataDir", "httpPort", "publicUrl"]) {
    $(id).disabled = !can["env/save"]?.allowed;
  }
}

let refreshing = false;

// Panels are drawn independently and the controls are updated last, whatever
// happened above. One panel that fails to draw used to abort the rest of the
// refresh, which left every button frozen at whatever it happened to be —
// a broken service table made the whole page look disabled.
function draw(name, render) {
  try {
    render();
  } catch (e) {
    console.error(`${name} の描画に失敗しました`, e);
    return `${name}: ${e.message}`;
  }
  return "";
}

async function refresh() {
  if (refreshing) return;
  refreshing = true;
  try {
    const state = await api("/api/state");
    const failures = [
      draw("環境チェック", () => renderChecks(state.checks)),
      draw("バージョン", () => renderRepo(state.repo)),
      draw("データディレクトリ", () => renderEnv(state)),
      draw("サービス", () => renderServices(state)),
    ].filter(Boolean);

    const busy = state.job?.running ?? false;
    $("job").textContent = busy ? `実行中: ${state.job.name}` : "";
    $("cancel").hidden = !busy;
    applyCapabilities(state.can ?? {}, busy);

    if (failures.length) $("checked-at").textContent = `表示エラー: ${failures.join(" / ")}`;
  } catch (e) {
    $("checked-at").textContent = `状態を取得できませんでした: ${e.message}`;
  } finally {
    refreshing = false;
  }
}

async function loadVersion() {
  try {
    $("version").textContent = (await api("/api/health")).version;
  } catch {
    $("version").textContent = "";
  }
}

setConnected(false);
loadVersion();
refresh();
connect();
