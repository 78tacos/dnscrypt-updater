(() => {
  const params = new URLSearchParams(location.search);
  const token = params.get("token") || "";
  const headers = { "X-Settings-Token": token, "Content-Type": "application/json" };

  let state = null;
  const edits = new Map();
  const fileEdits = new Map();
  const dismissed = new Set(JSON.parse(sessionStorage.getItem("dismissed") || "[]"));

  const $ = (id) => document.getElementById(id);
  const panels = {
    easy: $("panel-easy"),
    advanced: $("panel-advanced"),
    lists: $("panel-lists"),
  };

  document.querySelectorAll(".tab").forEach((btn) => {
    btn.addEventListener("click", () => {
      document.querySelectorAll(".tab").forEach((b) => b.classList.toggle("on", b === btn));
      const tab = btn.dataset.tab;
      Object.entries(panels).forEach(([k, el]) => el.classList.toggle("hidden", k !== tab));
    });
  });

  $("search").addEventListener("input", () => render());
  $("save").addEventListener("click", save);

  function api(path, opts) {
    const u = new URL(path, location.origin);
    u.searchParams.set("token", token);
    return fetch(u, opts);
  }

  async function load() {
    const res = await api("/api/state");
    if (!res.ok) {
      $("status").textContent = "Could not load settings: " + (await res.text());
      $("status").classList.remove("hidden");
      return;
    }
    state = await res.json();
    edits.clear();
    fileEdits.clear();
    render();
  }

  function fieldMap() {
    const m = new Map();
    (state.catalog.fields || []).forEach((f) => m.set(f.path, f));
    return m;
  }

  function currentOf(path) {
    if (edits.has(path)) return edits.get(path);
    return state.current[path] || { path, present: false, value: null, raw: "" };
  }

  function setEdit(path, patch, quiet) {
    const cur = currentOf(path);
    edits.set(path, Object.assign({}, cur, patch, { dirty: true }));
    if (!quiet) render();
  }

  function query() {
    return $("search").value.trim().toLowerCase();
  }

  function matches(f) {
    const q = query();
    if (!q) return true;
    const blob = [f.path, f.key, f.help, f.section_title, ...(f.examples || [])].join(" ").toLowerCase();
    return blob.includes(q);
  }

  function render() {
    if (!state) return;
    $("meta").innerHTML =
      "<div>Schema from DNSCrypt/dnscrypt-proxy <strong>" +
      esc(state.upstream_tag) +
      "</strong></div><div><code>" +
      esc(state.toml_path || state.install_dir || "(not installed)") +
      "</code></div>";

    const missing = $("missing");
    if (state.missing_proxy || !state.toml_exists) {
      missing.classList.remove("hidden");
      missing.textContent =
        "dnscrypt-proxy is not installed in this location yet. Use the tray item Install / Update dnscrypt-proxy first.";
    } else {
      missing.classList.add("hidden");
    }
    $("save").disabled = !state.toml_exists;

    renderPending();
    renderSuggestions();
    renderPresets();
    renderEasy();
    renderAdvanced();
    renderFiles();
  }

  function zipURL() {
    const u = new URL("/api/pending.zip", location.origin);
    u.searchParams.set("token", token);
    return u.toString();
  }

  function renderPending() {
    const el = $("pending-banner");
    const p = state.pending;
    if (!p || !p.present) {
      el.classList.add("hidden");
      el.innerHTML = "";
      return;
    }
    el.classList.remove("hidden");
    el.innerHTML =
      "<strong>Queued settings</strong> could not be written to the install directory. They are saved in <code>" +
      esc(p.dir) +
      "</code>. Right-click the tray icon and choose <strong>Apply pending settings</strong> (Administrator or root may be required)." +
      '<div class="row"><a class="chip" href="' +
      zipURL() +
      '">Download zip</a><button type="button" class="ghost" id="discard-pending">Discard queued files</button></div>';
    $("discard-pending").addEventListener("click", discardPending);
  }

  async function discardPending() {
    const res = await api("/api/pending", { method: "DELETE", headers });
    if (!res.ok) {
      showStatus("Could not discard queued files: " + (await res.text()), "warn");
      return;
    }
    await load();
  }

  function showStatus(text, kind) {
    const el = $("status");
    el.classList.remove("hidden", "warn", "ok");
    el.classList.add(kind === "ok" ? "ok" : "warn");
    el.textContent = text;
  }

  function renderSuggestions() {
    const root = $("suggestions");
    const items = (state.suggestions || []).filter((s) => !dismissed.has(s.id));
    if (!items.length) {
      root.classList.add("hidden");
      root.innerHTML = "";
      return;
    }
    root.classList.remove("hidden");
    root.innerHTML =
      "<h2>Suggestions</h2>" +
      items
        .map((s) => {
          const cls = s.level === "info" ? "suggest info" : "suggest";
          return (
            '<div class="' +
            cls +
            '"><h3>' +
            esc(s.title) +
            "</h3><p class='muted'>" +
            esc(s.detail) +
            "</p><div class='row'><button class='ghost' data-jump='" +
            esc(s.path || "") +
            "'>Show setting</button><button class='ghost' data-dismiss='" +
            esc(s.id) +
            "'>Dismiss</button></div></div>"
          );
        })
        .join("");
    root.querySelectorAll("[data-dismiss]").forEach((b) =>
      b.addEventListener("click", () => {
        dismissed.add(b.dataset.dismiss);
        sessionStorage.setItem("dismissed", JSON.stringify([...dismissed]));
        render();
      })
    );
    root.querySelectorAll("[data-jump]").forEach((b) =>
      b.addEventListener("click", () => {
        const path = b.dataset.jump;
        document.querySelectorAll(".tab")[1].click();
        const el = document.querySelector('[data-field="' + cssEscape(path) + '"]');
        if (el) el.scrollIntoView({ behavior: "smooth", block: "center" });
      })
    );
  }

  function renderPresets() {
    const root = $("presets");
    root.innerHTML = (state.presets || [])
      .map(
        (p) =>
          '<button class="chip" data-preset="' +
          esc(p.id) +
          '" title="' +
          esc(p.description) +
          '">' +
          esc(p.name) +
          "</button>"
      )
      .join("");
    root.querySelectorAll("[data-preset]").forEach((b) =>
      b.addEventListener("click", () => applyPreset(b.dataset.preset))
    );
  }

  function applyPreset(id) {
    const p = (state.presets || []).find((x) => x.id === id);
    if (!p) return;
    p.patches.forEach((patch) => {
      setEdit(patch.path, {
        present: patch.enabled,
        value: patch.value,
        raw: "",
      }, true);
    });
    render();
    $("status").classList.remove("hidden");
    $("status").textContent = "Preset “" + p.name + "” applied in the form. Save to write dnscrypt-proxy.toml.";
  }

  function grouped(fields) {
    const groups = [];
    const idx = new Map();
    fields.forEach((f) => {
      const title = f.section_title || f.section || "Other";
      if (!idx.has(title)) {
        idx.set(title, groups.length);
        groups.push({ title, fields: [] });
      }
      groups[idx.get(title)].fields.push(f);
    });
    return groups;
  }

  function renderEasy() {
    const fields = (state.catalog.fields || []).filter((f) => f.easy && matches(f));
    $("easy-fields").innerHTML = grouped(fields)
      .map((g) => '<h3 class="section-title">' + esc(g.title) + "</h3>" + g.fields.map(fieldHTML).join(""))
      .join("");
    bindFields($("easy-fields"));
  }

  function renderAdvanced() {
    const fields = (state.catalog.fields || []).filter(matches);
    $("advanced-fields").innerHTML = grouped(fields)
      .map((g) => '<h3 class="section-title">' + esc(g.title) + "</h3>" + g.fields.map(fieldHTML).join(""))
      .join("");
    bindFields($("advanced-fields"));
  }

  function fieldHTML(f) {
    const cur = currentOf(f.path);
    const enabled = !!cur.present;
    const dirty = cur.dirty ? " dirty" : "";
    const val = cur.value != null ? cur.value : f.default;
    let widget = "";
    if (f.type === "bool") {
      widget =
        '<label class="toggle"><input type="checkbox" data-bool="' +
        esc(f.path) +
        '" ' +
        (val ? "checked" : "") +
        "> on</label>";
    } else if (f.type === "string_list" || f.type === "int_list") {
      const list = Array.isArray(val) ? val.join(", ") : val == null ? "" : String(val);
      widget = '<input type="text" data-list="' + esc(f.path) + '" value="' + esc(list) + '" placeholder="comma-separated">';
    } else if (f.type === "int" || f.type === "float") {
      widget = '<input type="number" data-num="' + esc(f.path) + '" value="' + esc(val == null ? "" : String(val)) + '">';
    } else if (f.type === "inline_table_list") {
      widget =
        '<textarea data-json="' +
        esc(f.path) +
        '">' +
        esc(JSON.stringify(val || [], null, 2)) +
        "</textarea>";
    } else {
      widget = '<input type="text" data-str="' + esc(f.path) + '" value="' + esc(val == null ? "" : String(val)) + '">';
    }
    const examples = (f.examples || [])
      .slice(0, 4)
      .map((ex) => '<button type="button" class="chip" data-example="' + esc(f.path) + '" data-ex="' + esc(ex) + '">' + esc(ex) + "</button>")
      .join("");
    const related = f.related_file
      ? '<p class="muted">List file: <code>' + esc(f.related_file) + "</code></p>"
      : "";
    return (
      '<article class="field' +
      dirty +
      '" data-field="' +
      esc(f.path) +
      '"><div class="field-head"><label class="toggle"><input class="en" type="checkbox" data-en="' +
      esc(f.path) +
      '" ' +
      (enabled ? "checked" : "") +
      "> enable</label><div><div class='title'>" +
      esc(f.key) +
      "</div><div class='path'>" +
      esc(f.path) +
      "</div></div></div><p class='help'>" +
      esc(f.help || "") +
      "</p>" +
      related +
      '<div class="row">' +
      widget +
      "</div><div class='chips row'>" +
      examples +
      "</div></article>"
    );
  }

  function bindFields(root) {
    root.querySelectorAll("[data-en]").forEach((el) =>
      el.addEventListener("change", () => {
        const path = el.dataset.en;
        const cur = currentOf(path);
        const f = fieldMap().get(path);
        let value = cur.value;
        if (value == null && f && f.default != null) value = f.default;
        if (value == null && f && f.type === "bool") value = true;
        setEdit(path, { present: el.checked, value });
      })
    );
    root.querySelectorAll("[data-bool]").forEach((el) =>
      el.addEventListener("change", () => setEdit(el.dataset.bool, { present: true, value: el.checked }))
    );
    root.querySelectorAll("[data-str]").forEach((el) =>
      el.addEventListener("change", () => setEdit(el.dataset.str, { present: true, value: el.value }))
    );
    root.querySelectorAll("[data-num]").forEach((el) =>
      el.addEventListener("change", () => setEdit(el.dataset.num, { present: true, value: Number(el.value) }))
    );
    root.querySelectorAll("[data-list]").forEach((el) =>
      el.addEventListener("change", () => {
        const parts = el.value
          .split(",")
          .map((s) => s.trim())
          .filter(Boolean);
        setEdit(el.dataset.list, { present: true, value: parts });
      })
    );
    root.querySelectorAll("[data-json]").forEach((el) =>
      el.addEventListener("change", () => {
        try {
          setEdit(el.dataset.json, { present: true, value: JSON.parse(el.value || "[]") });
        } catch (err) {
          $("status").classList.remove("hidden");
          $("status").textContent = "Invalid JSON for " + el.dataset.json + ": " + err.message;
        }
      })
    );
    root.querySelectorAll("[data-example]").forEach((el) =>
      el.addEventListener("click", () => applyExample(el.dataset.example, el.dataset.ex))
    );
  }

  function applyExample(path, raw) {
    const f = fieldMap().get(path);
    if (!f) return;
    let value = raw;
    if (f.type === "bool") value = raw === "true";
    else if (f.type === "int" || f.type === "float") value = Number(raw);
    else if (f.type === "string_list" || f.type === "int_list") {
      try {
        const parsed = JSON.parse(raw.replace(/'/g, '"'));
        value = parsed;
      } catch {
        value = raw
          .replace(/^\[/, "")
          .replace(/\]$/, "")
          .split(",")
          .map((s) => s.trim().replace(/^['"]|['"]$/g, ""))
          .filter(Boolean);
      }
    } else if (typeof raw === "string") {
      value = raw.replace(/^['"]|['"]$/g, "");
    }
    setEdit(path, { present: true, value });
  }

  function renderFiles() {
    const root = $("list-files");
    const files = state.files || [];
    root.innerHTML = files
      .map((f) => {
        const content = fileEdits.has(f.name) ? fileEdits.get(f.name) : f.content;
        return (
          '<div class="file"><h2>' +
          esc(f.name) +
          '</h2><textarea data-file="' +
          esc(f.name) +
          '">' +
          esc(content) +
          "</textarea></div>"
        );
      })
      .join("");
    root.querySelectorAll("[data-file]").forEach((el) =>
      el.addEventListener("change", () => fileEdits.set(el.dataset.file, el.value))
    );
  }

  async function save() {
    if (!state || !state.toml_exists) return;
    $("save").disabled = true;
    const patches = [];
    edits.forEach((v, path) => {
      const f = fieldMap().get(path);
      let value = v.value;
      if (value === undefined) value = null;
      if (value == null && f && f.default != null) value = f.default;
      if (value == null && f && f.type === "bool") value = true;
      patches.push({ path, enabled: !!v.present, value });
    });
    const files = [];
    fileEdits.forEach((content, name) => files.push({ name, content }));
    const res = await api("/api/apply", {
      method: "POST",
      headers,
      body: JSON.stringify({ patches, files }),
    });
    const text = await res.text();
    if (!res.ok) {
      showStatus("Save failed: " + text, "warn");
      $("save").disabled = false;
      return;
    }
    let msg = "Saved.";
    let pending = false;
    try {
      const data = JSON.parse(text);
      msg = data.message || msg;
      pending = !!data.pending;
    } catch {}
    showStatus(msg, pending ? "warn" : "ok");
    await load();
  }

  function esc(s) {
    return String(s ?? "")
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  function cssEscape(s) {
    return String(s).replace(/"/g, '\\"');
  }

  load();
})();
