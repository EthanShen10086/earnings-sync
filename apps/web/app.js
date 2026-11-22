import { createClient } from "../../packages/api-client/index.js";

const I18N = {
  zh: {
    title: "earnings-sync",
    subtitle:
      "只读事实表：排序 / 筛选 / CSV；鉴权默认 bypass。Sheets 旧链路与 PG/API 展示并存（见 docs/coexistence-acceptance.md）。",
    ticker: "Ticker",
    api: "API Base",
    filter: "筛选科目",
    token: "Bearer（可选）",
    load: "加载",
    exportCsv: "导出 CSV",
    refreshAuth: "刷新身份",
    refreshPlatform: "刷新平台",
    themeDark: "深色",
    themeLight: "浅色",
    langZh: "中文",
    langEn: "EN",
    colPeriod: "期间",
    colItem: "科目",
    colValue: "数值",
    colUnit: "单位",
    colActions: "操作",
    copy: "复制",
    idle: "尚未加载",
    loading: "加载中…",
    empty: "无数据。先用 STORAGE_MODE=dual|pg-only ingest 一份 fixture。",
    loaded: (n, cache) => `共 ${n} 条` + (cache ? " · cache_hit" : ""),
    who: (p) => `当前身份：${p.name || p.sub} · ${p.source} · roles=${(p.roles || []).join(",")}`,
    whoFail: "未能读取 /v1/auth/me（检查 AUTH_MODE）",
    copied: "已复制行",
    exported: (n) => `已导出 ${n} 行 CSV`,
    platformFail: "未能读取平台状态",
    storageLine: (s, api) =>
      `API ${api || "?"} · storage want=${s.want_mode || "?"} → effective=${s.mode || "?"} · sheets=${s.sheets_kind || "?"} · pg=${s.pg_kind || "?"}`,
  },
  en: {
    title: "earnings-sync",
    subtitle:
      "Read-only facts: sort / filter / CSV; AUTH_MODE=bypass. Sheets path coexists with PG/API UI (see docs/coexistence-acceptance.md).",
    ticker: "Ticker",
    api: "API Base",
    filter: "Filter line item",
    token: "Bearer (optional)",
    load: "Load",
    exportCsv: "Export CSV",
    refreshAuth: "Refresh identity",
    refreshPlatform: "Refresh platform",
    themeDark: "Dark",
    themeLight: "Light",
    langZh: "中文",
    langEn: "EN",
    colPeriod: "Period",
    colItem: "Line item",
    colValue: "Value",
    colUnit: "Unit",
    colActions: "Actions",
    copy: "Copy",
    idle: "Not loaded yet",
    loading: "Loading…",
    empty: "No rows. Ingest a fixture with STORAGE_MODE=dual|pg-only first.",
    loaded: (n, cache) => `${n} rows` + (cache ? " · cache_hit" : ""),
    who: (p) => `Signed in as ${p.name || p.sub} · ${p.source} · roles=${(p.roles || []).join(",")}`,
    whoFail: "Could not load /v1/auth/me (check AUTH_MODE)",
    copied: "Row copied",
    exported: (n) => `Exported ${n} CSV rows`,
    platformFail: "Could not load platform status",
    storageLine: (s, api) =>
      `API ${api || "?"} · storage want=${s.want_mode || "?"} → effective=${s.mode || "?"} · sheets=${s.sheets_kind || "?"} · pg=${s.pg_kind || "?"}`,
  },
};

const state = {
  lang: localStorage.getItem("es.lang") || "zh",
  theme: localStorage.getItem("es.theme") || "dark",
  facts: [],
  sortKey: "period",
  sortDir: "asc",
  apiVersion: "",
  lastCacheHit: false,
};

const el = {
  title: document.getElementById("title"),
  subtitle: document.getElementById("subtitle"),
  ticker: document.getElementById("ticker"),
  api: document.getElementById("api"),
  filter: document.getElementById("filter"),
  token: document.getElementById("token"),
  load: document.getElementById("load"),
  exportCsv: document.getElementById("exportCsv"),
  refreshAuth: document.getElementById("refreshAuth"),
  refreshPlatform: document.getElementById("refreshPlatform"),
  status: document.getElementById("status"),
  whoami: document.getElementById("whoami"),
  platform: document.getElementById("platform"),
  rows: document.getElementById("rows"),
  empty: document.getElementById("empty"),
  thPeriod: document.getElementById("th-period"),
  thItem: document.getElementById("th-item"),
  thValue: document.getElementById("th-value"),
  thUnit: document.getElementById("th-unit"),
  thActions: document.getElementById("th-actions"),
  lblTicker: document.getElementById("lbl-ticker"),
  lblApi: document.getElementById("lbl-api"),
  lblFilter: document.getElementById("lbl-filter"),
  lblToken: document.getElementById("lbl-token"),
  themeDark: document.getElementById("theme-dark"),
  themeLight: document.getElementById("theme-light"),
  langZh: document.getElementById("lang-zh"),
  langEn: document.getElementById("lang-en"),
};

function t() {
  return I18N[state.lang] || I18N.zh;
}

function client() {
  return createClient({
    baseURL: el.api.value,
    getToken: () => el.token.value.trim(),
  });
}

function applyChrome() {
  const dict = t();
  document.documentElement.lang = state.lang === "zh" ? "zh-CN" : "en";
  document.documentElement.setAttribute("data-theme", state.theme);
  el.title.textContent = dict.title;
  el.subtitle.textContent = dict.subtitle;
  el.lblTicker.textContent = dict.ticker;
  el.lblApi.textContent = dict.api;
  el.lblFilter.textContent = dict.filter;
  el.lblToken.textContent = dict.token;
  el.load.textContent = dict.load;
  el.exportCsv.textContent = dict.exportCsv;
  el.refreshAuth.textContent = dict.refreshAuth;
  el.refreshPlatform.textContent = dict.refreshPlatform;
  el.themeDark.textContent = dict.themeDark;
  el.themeLight.textContent = dict.themeLight;
  el.langZh.textContent = dict.langZh;
  el.langEn.textContent = dict.langEn;
  el.thPeriod.textContent = dict.colPeriod;
  el.thItem.textContent = dict.colItem;
  el.thValue.textContent = dict.colValue;
  el.thUnit.textContent = dict.colUnit;
  el.thActions.textContent = dict.colActions;
  el.themeDark.setAttribute("aria-pressed", String(state.theme === "dark"));
  el.themeLight.setAttribute("aria-pressed", String(state.theme === "light"));
  el.langZh.setAttribute("aria-pressed", String(state.lang === "zh"));
  el.langEn.setAttribute("aria-pressed", String(state.lang === "en"));
  if (!state.facts.length && el.status.textContent === "") {
    setStatus(dict.idle, "");
  }
  renderTable();
}

function setStatus(msg, kind) {
  el.status.className = "status" + (kind ? " " + kind : "");
  el.status.textContent = msg;
}

async function loadAuth() {
  try {
    const data = await client().authMe();
    el.whoami.textContent = t().who(data.principal || {});
  } catch (e) {
    el.whoami.textContent = t().whoFail + " · " + (e.message || e);
  }
}

async function loadPlatform() {
  try {
    const [meta, status] = await Promise.all([client().meta(), client().platformStatus()]);
    state.apiVersion = meta.api_version || status.api_version || "";
    const storage = status.storage || meta.storage || {};
    const notes = Array.isArray(storage.notes) && storage.notes.length ? " · " + storage.notes.join("; ") : "";
    el.platform.textContent = t().storageLine(storage, state.apiVersion) + notes;
  } catch (e) {
    el.platform.textContent = t().platformFail + " · " + (e.message || e);
  }
}

function periodKey(f) {
  return (f.fiscal_year || 0) * 10 + (f.fiscal_quarter || 0);
}

function filteredSorted() {
  const q = el.filter.value.trim().toLowerCase();
  let rows = state.facts.slice();
  if (q) {
    rows = rows.filter((f) => String(f.line_item || "").toLowerCase().includes(q));
  }
  const dir = state.sortDir === "asc" ? 1 : -1;
  rows.sort((a, b) => {
    let cmp = 0;
    switch (state.sortKey) {
      case "item":
        cmp = String(a.line_item || "").localeCompare(String(b.line_item || ""));
        break;
      case "value": {
        const av = a.value != null ? a.value : Number.NEGATIVE_INFINITY;
        const bv = b.value != null ? b.value : Number.NEGATIVE_INFINITY;
        cmp = av === bv ? 0 : av < bv ? -1 : 1;
        break;
      }
      case "unit":
        cmp = String(a.unit || "").localeCompare(String(b.unit || ""));
        break;
      default:
        cmp = periodKey(a) - periodKey(b);
    }
    return cmp * dir;
  });
  return rows;
}

function syncSortHeaders() {
  const map = {
    period: el.thPeriod,
    item: el.thItem,
    value: el.thValue,
    unit: el.thUnit,
  };
  Object.entries(map).forEach(([key, node]) => {
    if (key === state.sortKey) node.setAttribute("aria-sort", state.sortDir === "asc" ? "ascending" : "descending");
    else node.removeAttribute("aria-sort");
  });
}

function renderTable() {
  const dict = t();
  const rows = filteredSorted();
  el.rows.innerHTML = "";
  syncSortHeaders();
  if (!rows.length) {
    el.empty.hidden = false;
    el.empty.textContent = state.facts.length ? dict.empty : dict.idle;
    return;
  }
  el.empty.hidden = true;
  for (const f of rows) {
    const tr = document.createElement("tr");
    const period = (f.fiscal_year || "") + "Q" + (f.fiscal_quarter || "");
    const val = f.value != null ? f.value : f.text || "";
    tr.innerHTML =
      "<td>" +
      period +
      "</td>" +
      "<td>" +
      escapeHtml(f.line_item || "") +
      "</td>" +
      '<td class="num">' +
      escapeHtml(String(val)) +
      "</td>" +
      "<td>" +
      escapeHtml(f.unit || "") +
      "</td>" +
      '<td><div class="row-actions"><button type="button" class="secondary" data-copy="1">' +
      dict.copy +
      "</button></div></td>";
    tr.querySelector("[data-copy]").addEventListener("click", async () => {
      const line = [period, f.line_item || "", val, f.unit || ""].join("\t");
      try {
        await navigator.clipboard.writeText(line);
        setStatus(dict.copied, "ok");
      } catch (_) {
        setStatus(line, "");
      }
    });
    el.rows.appendChild(tr);
  }
}

function escapeHtml(s) {
  return String(s)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

async function loadFacts() {
  const dict = t();
  const ticker = el.ticker.value.trim().toUpperCase();
  setStatus(dict.loading, "");
  el.rows.innerHTML = "";
  try {
    const data = await client().facts(ticker);
    state.facts = data.facts || [];
    state.lastCacheHit = !!(data.meta && data.meta.cache_hit);
    if (data.meta && data.meta.api_version) state.apiVersion = data.meta.api_version;
    if (!state.facts.length) {
      setStatus(dict.empty, "");
    } else {
      setStatus(dict.loaded(state.facts.length, state.lastCacheHit), "ok");
    }
    renderTable();
  } catch (e) {
    state.facts = [];
    renderTable();
    setStatus(String(e.message || e), "err");
  }
}

function exportCsv() {
  const rows = filteredSorted();
  const lines = ["period,line_item,value,unit"];
  for (const f of rows) {
    const period = (f.fiscal_year || "") + "Q" + (f.fiscal_quarter || "");
    const val = f.value != null ? f.value : f.text || "";
    lines.push([period, csv(f.line_item), csv(val), csv(f.unit)].join(","));
  }
  const blob = new Blob([lines.join("\n")], { type: "text/csv;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = (el.ticker.value.trim().toUpperCase() || "facts") + ".csv";
  a.click();
  URL.revokeObjectURL(url);
  setStatus(t().exported(rows.length), "ok");
}

function csv(v) {
  const s = String(v == null ? "" : v);
  if (/[",\n]/.test(s)) return '"' + s.replace(/"/g, '""') + '"';
  return s;
}

function setSort(key) {
  if (state.sortKey === key) state.sortDir = state.sortDir === "asc" ? "desc" : "asc";
  else {
    state.sortKey = key;
    state.sortDir = "asc";
  }
  renderTable();
}

el.load.addEventListener("click", () => {
  loadFacts();
  loadAuth();
  loadPlatform();
});
el.exportCsv.addEventListener("click", exportCsv);
el.refreshAuth.addEventListener("click", loadAuth);
el.refreshPlatform.addEventListener("click", loadPlatform);
el.filter.addEventListener("input", renderTable);
el.thPeriod.addEventListener("click", () => setSort("period"));
el.thItem.addEventListener("click", () => setSort("item"));
el.thValue.addEventListener("click", () => setSort("value"));
el.thUnit.addEventListener("click", () => setSort("unit"));
el.themeDark.addEventListener("click", () => {
  state.theme = "dark";
  localStorage.setItem("es.theme", state.theme);
  applyChrome();
});
el.themeLight.addEventListener("click", () => {
  state.theme = "light";
  localStorage.setItem("es.theme", state.theme);
  applyChrome();
});
el.langZh.addEventListener("click", () => {
  state.lang = "zh";
  localStorage.setItem("es.lang", state.lang);
  applyChrome();
});
el.langEn.addEventListener("click", () => {
  state.lang = "en";
  localStorage.setItem("es.lang", state.lang);
  applyChrome();
});

applyChrome();
setStatus(t().idle, "");
loadAuth();
loadPlatform();
