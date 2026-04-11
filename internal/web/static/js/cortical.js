// CorticalStack frontend — vanilla JS, no framework.

(function () {
  // ---------- Ingest tabs ----------
  const tabs = document.querySelectorAll(".tab");
  const panels = document.querySelectorAll(".tab-panel");
  tabs.forEach((tab) => {
    tab.addEventListener("click", () => {
      const target = tab.dataset.tab;
      tabs.forEach((t) => t.classList.toggle("tab-active", t === tab));
      panels.forEach((p) => p.classList.toggle("tab-panel-active", p.dataset.panel === target));
    });
  });

  // ---------- Ingest forms ----------
  const textForm = document.getElementById("form-text");
  const urlForm = document.getElementById("form-url");
  const fileForm = document.getElementById("form-file");
  const audioForm = document.getElementById("form-audio");

  if (textForm) {
    textForm.addEventListener("submit", async (e) => {
      e.preventDefault();
      const fd = new FormData(textForm);
      const body = { text: fd.get("text"), title: fd.get("title") || "" };
      await submitIngest("/api/ingest/text", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
    });
  }
  if (urlForm) {
    urlForm.addEventListener("submit", async (e) => {
      e.preventDefault();
      const fd = new FormData(urlForm);
      await submitIngest("/api/ingest/url", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ url: fd.get("url") }),
      });
    });
  }
  [fileForm, audioForm].forEach((form) => {
    if (!form) return;
    form.addEventListener("submit", async (e) => {
      e.preventDefault();
      const fd = new FormData(form);
      await submitIngest("/api/ingest/file", { method: "POST", body: fd });
    });
  });

  let currentJobID = null;

  async function submitIngest(url, init) {
    const panel = document.getElementById("job-panel");
    const logEl = document.getElementById("job-log");
    const statusEl = document.getElementById("job-status");
    const idEl = document.getElementById("job-id");
    const noteEl = document.getElementById("job-note");
    const noteLink = document.getElementById("job-note-link");
    const previewPanel = document.getElementById("preview-panel");
    if (logEl) logEl.innerHTML = "";
    if (noteEl) noteEl.hidden = true;
    if (previewPanel) previewPanel.hidden = true;
    if (panel) panel.hidden = false;

    let res;
    try {
      res = await fetch(url, init);
    } catch (err) {
      appendLog("network error: " + err);
      return;
    }
    if (!res.ok) {
      const txt = await res.text();
      appendLog("error: " + txt);
      return;
    }
    const { job_id } = await res.json();
    currentJobID = job_id;
    if (idEl) idEl.textContent = job_id;
    if (statusEl) statusEl.textContent = "running";

    const es = new EventSource(`/api/jobs/${job_id}/stream`);
    es.addEventListener("job_snapshot", (e) => {
      const job = JSON.parse(e.data);
      if (statusEl) statusEl.textContent = job.status;
      (job.messages || []).forEach(appendLog);
      if (job.note_path) showNote(job.note_path);
      if (job.preview && job.status === "awaiting_confirmation") {
        showPreview(job.preview);
      }
      if (job.status === "completed" || job.status === "failed") es.close();
    });
    es.addEventListener("job_status", (e) => {
      const evt = JSON.parse(e.data);
      if (statusEl) statusEl.textContent = evt.status;
      if (evt.message) appendLog(evt.message);
    });
    es.addEventListener("job_progress", (e) => {
      const evt = JSON.parse(e.data);
      if (evt.message) appendLog(evt.message);
    });
    es.addEventListener("job_preview", (e) => {
      const data = JSON.parse(e.data);
      if (data.preview) showPreview(data.preview);
    });
    es.addEventListener("job_complete", (e) => {
      const evt = JSON.parse(e.data);
      if (statusEl) statusEl.textContent = "completed";
      if (evt.message) appendLog(evt.message);
      fetch(`/api/jobs/${job_id}`).then((r) => r.json()).then((job) => {
        if (job.note_path) showNote(job.note_path);
      });
      if (previewPanel) previewPanel.hidden = true;
      es.close();
    });
    es.addEventListener("job_failed", (e) => {
      const evt = JSON.parse(e.data);
      if (statusEl) statusEl.textContent = "failed";
      appendLog("error: " + (evt.message || "unknown"));
      es.close();
    });

    function appendLog(msg) {
      if (!logEl) return;
      const li = document.createElement("li");
      li.textContent = msg;
      logEl.appendChild(li);
      logEl.scrollTop = logEl.scrollHeight;
    }
    function showNote(path) {
      if (!noteEl || !noteLink) return;
      noteEl.hidden = false;
      noteLink.textContent = path;
      noteLink.href = `/library?note=${encodeURIComponent(path)}`;
    }
  }

  // ---------- Preview modal ----------
  function showPreview(preview) {
    const panel = document.getElementById("preview-panel");
    if (!panel) return;
    panel.hidden = false;

    document.getElementById("preview-title").value = preview.suggested_title || "";
    document.getElementById("preview-intention").value = preview.intention || "information";
    document.getElementById("preview-summary").value = preview.summary || "";
    document.getElementById("preview-reasoning").textContent =
      (preview.reasoning ? `Reasoning: ${preview.reasoning}` : "") +
      (preview.confidence != null ? ` · Confidence: ${(preview.confidence * 100).toFixed(0)}%` : "");

    const projectsField = document.getElementById("preview-projects");
    if (projectsField) {
      projectsField.value = (preview.suggested_project_ids || []).join("\n");
    }
  }

  const confirmBtn = document.getElementById("btn-confirm");
  if (confirmBtn) {
    confirmBtn.addEventListener("click", async () => {
      if (!currentJobID) return;
      const payload = {
        title: document.getElementById("preview-title").value,
        intention: document.getElementById("preview-intention").value,
        project_ids: document.getElementById("preview-projects").value
          .split("\n")
          .map((s) => s.trim())
          .filter(Boolean),
        why: document.getElementById("preview-why").value,
      };
      const res = await fetch(`/api/jobs/${currentJobID}/confirm`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
      if (!res.ok) {
        alert("confirm failed: " + (await res.text()));
      }
    });
  }

  // ---------- Library tree + preview ----------
  const treeEl = document.getElementById("vault-tree");
  const previewEl = document.getElementById("vault-preview");
  if (treeEl) {
    fetch("/api/vault/tree").then((r) => r.json()).then((root) => {
      treeEl.innerHTML = "";
      treeEl.appendChild(renderNode(root));
    }).catch((err) => {
      treeEl.textContent = "error loading tree: " + err;
    });
  }

  function renderNode(node) {
    if (node.is_dir) {
      const ul = document.createElement("ul");
      const li = document.createElement("li");
      li.className = "dir";
      li.textContent = "▸ " + node.name;
      ul.appendChild(li);
      (node.children || []).forEach((child) => ul.appendChild(renderNode(child)));
      return ul;
    }
    const li = document.createElement("li");
    li.className = "file";
    li.textContent = "· " + node.name;
    li.addEventListener("click", () => {
      fetch("/api/vault/file?path=" + encodeURIComponent(node.path))
        .then((r) => r.text())
        .then((text) => {
          if (previewEl) previewEl.textContent = text;
        });
    });
    return li;
  }

  // Auto-open a note when library is loaded with ?note=
  if (treeEl && previewEl) {
    const params = new URLSearchParams(window.location.search);
    const note = params.get("note");
    if (note) {
      fetch("/api/vault/file?path=" + encodeURIComponent(note))
        .then((r) => r.text())
        .then((text) => (previewEl.textContent = text));
    }
  }

  // ---------- Projects create ----------
  const projectForm = document.getElementById("form-project");
  if (projectForm) {
    projectForm.addEventListener("submit", async (e) => {
      e.preventDefault();
      const fd = new FormData(projectForm);
      const res = await fetch("/api/projects", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name: fd.get("name"),
          description: fd.get("description") || "",
        }),
      });
      if (res.ok) {
        window.location.reload();
      } else {
        alert("create failed: " + (await res.text()));
      }
    });
  }

  // ---------- Actions status + reconcile ----------
  document.querySelectorAll(".status-select").forEach((sel) => {
    sel.addEventListener("change", async () => {
      const id = sel.dataset.actionId;
      const res = await fetch(`/api/actions/${id}/status`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ status: sel.value }),
      });
      if (!res.ok) {
        alert("status update failed: " + (await res.text()));
      }
    });
  });
  const reconcileBtn = document.getElementById("btn-reconcile");
  if (reconcileBtn) {
    reconcileBtn.addEventListener("click", async () => {
      const res = await fetch("/api/actions/reconcile", { method: "POST" });
      if (res.ok) {
        const result = await res.json();
        alert(`Reconciled: scanned ${result.scanned}, matched ${result.lines_matched}, updated ${result.updated}`);
        window.location.reload();
      } else {
        alert("reconcile failed: " + (await res.text()));
      }
    });
  }

  // ---------- ShapeUp: create idea + advance ----------
  const ideaForm = document.getElementById("form-shapeup-idea");
  if (ideaForm) {
    ideaForm.addEventListener("submit", async (e) => {
      e.preventDefault();
      const fd = new FormData(ideaForm);
      const body = {
        title: fd.get("title"),
        content: fd.get("content"),
        project_ids: String(fd.get("projects") || "")
          .split("\n")
          .map((s) => s.trim())
          .filter(Boolean),
      };
      const res = await fetch("/api/shapeup/idea", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      if (res.ok) {
        window.location.reload();
      } else {
        alert("create idea failed: " + (await res.text()));
      }
    });
  }
  document.querySelectorAll(".advance-btn").forEach((btn) => {
    btn.addEventListener("click", async () => {
      const controls = btn.closest(".advance-controls");
      if (!controls) return;
      const threadId = controls.dataset.threadId;
      const stage = controls.querySelector(".advance-stage").value;
      const hints = controls.querySelector(".advance-hints").value;
      btn.disabled = true;
      btn.textContent = "Advancing…";
      const res = await fetch(`/api/shapeup/threads/${threadId}/advance`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ target_stage: stage, hints }),
      });
      btn.disabled = false;
      btn.textContent = "Advance →";
      if (res.ok) {
        window.location.reload();
      } else {
        alert("advance failed: " + (await res.text()));
      }
    });
  });

  // ---------- UseCases: generate from doc or text ----------
  const useCaseDocForm = document.getElementById("form-usecase-doc");
  if (useCaseDocForm) {
    useCaseDocForm.addEventListener("submit", async (e) => {
      e.preventDefault();
      const fd = new FormData(useCaseDocForm);
      await submitGenerate("/api/usecases/from-doc", {
        source_path: fd.get("source_path"),
        hint: fd.get("hint") || "",
      });
    });
  }
  const useCaseTextForm = document.getElementById("form-usecase-text");
  if (useCaseTextForm) {
    useCaseTextForm.addEventListener("submit", async (e) => {
      e.preventDefault();
      const fd = new FormData(useCaseTextForm);
      await submitGenerate("/api/usecases/from-text", {
        description: fd.get("description"),
        actors_hint: fd.get("actors_hint") || "",
      });
    });
  }

  // ---------- Prototypes: synthesize ----------
  const prototypeForm = document.getElementById("form-prototype");
  if (prototypeForm) {
    prototypeForm.addEventListener("submit", async (e) => {
      e.preventDefault();
      const fd = new FormData(prototypeForm);
      await submitGenerate("/api/prototypes", {
        title: fd.get("title"),
        source_paths: String(fd.get("source_paths") || "")
          .split("\n")
          .map((s) => s.trim())
          .filter(Boolean),
        format: fd.get("format"),
        hints: fd.get("hints") || "",
      });
    });
  }

  // ---------- PRD: synthesize ----------
  const prdForm = document.getElementById("form-prd");
  if (prdForm) {
    prdForm.addEventListener("submit", async (e) => {
      e.preventDefault();
      const fd = new FormData(prdForm);
      await submitGenerate("/api/prds", {
        pitch_path: fd.get("pitch_path"),
        extra_context_paths: String(fd.get("extra_context_paths") || "")
          .split("\n")
          .map((s) => s.trim())
          .filter(Boolean),
        extra_context_tags: String(fd.get("extra_context_tags") || "")
          .split(",")
          .map((s) => s.trim())
          .filter(Boolean),
        project_ids: String(fd.get("project_ids") || "")
          .split(",")
          .map((s) => s.trim())
          .filter(Boolean),
      });
    });
  }

  async function submitGenerate(url, body) {
    const res = await fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    if (res.ok) {
      window.location.reload();
    } else {
      alert(`request to ${url} failed: ` + (await res.text()));
    }
  }

  // ---------- Persona editor (SOUL / USER / MEMORY) ----------
  const personaForm = document.getElementById("form-persona");
  if (personaForm) {
    personaForm.addEventListener("submit", async (e) => {
      e.preventDefault();
      const name = personaForm.dataset.name;
      const content = document.getElementById("persona-content").value;
      const statusEl = document.getElementById("persona-save-status");
      if (statusEl) statusEl.textContent = "saving…";
      const res = await fetch(`/api/persona/${name}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ content }),
      });
      if (res.ok) {
        if (statusEl) statusEl.textContent = "saved";
        setTimeout(() => { if (statusEl) statusEl.textContent = ""; }, 2000);
      } else {
        const err = await res.text();
        if (statusEl) statusEl.textContent = "error: " + err;
      }
    });
  }
})();
