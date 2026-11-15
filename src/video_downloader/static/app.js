"use strict";

const POLL_INTERVAL_MS = 1000;
const SVG_NS = "http://www.w3.org/2000/svg";

const PRESET_NAMES = { best: "Meilleure qualité", audio: "Audio seul" };
const EXTRACTOR_NAMES = { Youtube: "YouTube", Twitter: "X" };
const ERROR_MESSAGES = {
  unsupported_url: "Aucune vidéo trouvée à cette adresse.",
  unavailable: "Cette vidéo est privée, supprimée ou demande une connexion.",
  playlist: "Les playlists ne sont pas prises en charge.",
  extraction_failed: "L'analyse a échoué. yt-dlp est peut-être à mettre à jour.",
};
const FALLBACK_ERROR = "Le serveur ne répond pas. Réessaie dans un instant.";

const $ = (id) => document.getElementById(id);
const decimal = new Intl.NumberFormat("fr-FR", { maximumFractionDigits: 1 });

// The video being previewed: { url, title, preset }.
let preview = null;
const rows = new Map();
// The API only learns a title once the download starts; this covers the queued state.
const titles = new Map();

class ApiError extends Error {
  constructor(code) {
    super(code);
    this.code = code;
  }
}

async function api(method, path, body) {
  const response = await fetch(path, {
    method,
    headers: body ? { "content-type": "application/json" } : {},
    body: body ? JSON.stringify(body) : undefined,
  });
  if (response.status === 204) return null;
  const data = await response.json();
  if (!response.ok) throw new ApiError(data.code);
  return data;
}

function messageFor(error) {
  return ERROR_MESSAGES[error.code] ?? FALLBACK_ERROR;
}

function formatBytes(bytes) {
  return bytes >= 1e9 ? `${decimal.format(bytes / 1e9)} Go` : `${decimal.format(bytes / 1e6)} Mo`;
}

function formatDuration(seconds) {
  const total = Math.round(seconds);
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const rest = total % 60;
  if (hours) return `${hours} h ${String(minutes).padStart(2, "0")} min`;
  if (minutes) return `${minutes} min ${String(rest).padStart(2, "0")}`;
  return `${rest} s`;
}

function extractorName(extractor) {
  return EXTRACTOR_NAMES[extractor] ?? extractor;
}

function presetName(id) {
  return PRESET_NAMES[id] ?? id;
}

function icon(name) {
  const svg = document.createElementNS(SVG_NS, "svg");
  svg.setAttribute("class", "icon");
  svg.setAttribute("aria-hidden", "true");
  const use = document.createElementNS(SVG_NS, "use");
  use.setAttribute("href", `#icon-${name}`);
  svg.append(use);
  return svg;
}

function showMessage(element, message) {
  element.hidden = !message;
  element.querySelector("span").textContent = message ?? "";
}

function setUrlError(message) {
  showMessage($("url-error"), message);
  $("url-hint").hidden = Boolean(message);
  $("url").setAttribute("aria-invalid", String(Boolean(message)));
}

function setAnalysing(busy) {
  const button = $("analyse");
  $("url").disabled = busy;
  button.disabled = busy;
  button.setAttribute("aria-busy", String(busy));
  button.querySelector(".button-label").textContent = busy ? "Analyse…" : "Analyser";
  $("skeleton").hidden = !busy;
}

async function analyse(event) {
  event.preventDefault();
  const url = $("url").value.trim();
  setUrlError(null);
  preview = null;
  $("preview").hidden = true;
  setAnalysing(true);
  try {
    showPreview(url, await api("POST", "/api/info", { url }));
  } catch (error) {
    setUrlError(messageFor(error));
  } finally {
    setAnalysing(false);
  }
}

function presetButton(preset, selected) {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "preset";
  button.dataset.preset = preset.id;
  button.setAttribute("aria-pressed", String(selected));

  const name = document.createElement("span");
  name.className = "preset-name";
  const detail = document.createElement("span");
  detail.className = "preset-detail";
  if (preset.id === "audio") {
    name.textContent = PRESET_NAMES.audio;
    detail.textContent = "M4A";
  } else if (preset.id === "best") {
    name.textContent = PRESET_NAMES.best;
    detail.textContent = preset.resolution ? `MP4 · ${preset.resolution}p` : "MP4";
  } else {
    name.textContent = `${preset.resolution}p`;
    detail.textContent = "MP4";
  }
  button.append(name, detail);
  return button;
}

function showPreview(url, info) {
  if (info.presets.length === 0) {
    setUrlError("Aucun format téléchargeable pour cette vidéo.");
    return;
  }
  const preferred = info.presets.find((preset) => preset.id === "1080p") ?? info.presets[0];
  preview = { url, title: info.title ?? url, preset: preferred.id };

  // The placeholder stays visible until the thumbnail has really loaded.
  const thumb = $("preview-thumb");
  thumb.hidden = true;
  thumb.onload = () => {
    thumb.hidden = false;
  };
  if (info.thumbnail) thumb.src = info.thumbnail;
  else thumb.removeAttribute("src");

  const source = $("preview-source");
  source.textContent = extractorName(info.extractor) ?? "";
  source.hidden = !info.extractor;
  $("preview-title").textContent = preview.title;
  $("preview-meta").textContent = [
    info.uploader,
    info.duration == null ? null : formatDuration(info.duration),
  ]
    .filter(Boolean)
    .join(" · ");
  $("presets").replaceChildren(
    ...info.presets.map((preset) => presetButton(preset, preset.id === preferred.id)),
  );
  $("preview").hidden = false;
}

function selectPreset(event) {
  const chosen = event.target.closest(".preset");
  if (!chosen) return;
  preview.preset = chosen.dataset.preset;
  for (const button of $("presets").children) {
    button.setAttribute("aria-pressed", String(button === chosen));
  }
}

async function startDownload() {
  const { url, title, preset } = preview;
  $("download").disabled = true;
  try {
    const job = await api("POST", "/api/downloads", { url, preset });
    titles.set(job.id, title);
    preview = null;
    $("preview").hidden = true;
    $("url").value = "";
    await refreshJobs();
  } catch (error) {
    setUrlError(messageFor(error));
  } finally {
    $("download").disabled = false;
  }
}

function statusText(job) {
  switch (job.status) {
    case "queued":
      return "En attente";
    case "downloading":
      if (job.percent == null) return "Téléchargement…";
      return [
        `${Math.round(job.percent)} %`,
        job.speed ? `${formatBytes(job.speed)}/s` : null,
        job.eta == null ? null : `reste ${formatDuration(job.eta)}`,
      ]
        .filter(Boolean)
        .join(" · ");
    case "merging":
      return job.preset === "audio"
        ? "Conversion de l'audio…"
        : "Fusion de l'audio et de la vidéo…";
    case "done":
      return `Terminé · ${formatBytes(job.size)}`;
    default:
      return `Échec : ${job.error}`;
  }
}

function actionButton(action, label, iconName, extraClass) {
  const button = document.createElement("button");
  button.type = "button";
  button.className = `button ${extraClass}`;
  button.dataset.action = action;
  button.setAttribute("aria-label", label);
  if (iconName) button.append(icon(iconName));
  if (extraClass !== "button-icon") {
    const text = document.createElement("span");
    text.textContent = label;
    button.append(text);
  }
  return button;
}

function actionsFor(job) {
  const remove = actionButton("delete", "Supprimer", "trash", "button-icon");
  if (job.status === "done") {
    const save = document.createElement("a");
    save.className = "button button-accent";
    save.href = `/api/downloads/${job.id}/file`;
    save.download = "";
    save.textContent = "Enregistrer";
    return [save, remove];
  }
  if (job.status === "failed") {
    return [actionButton("retry", "Réessayer", null, ""), remove];
  }
  return [actionButton("delete", "Annuler", "close", "button-cancel")];
}

function buildRow(job) {
  const row = $("job-template").content.firstElementChild.cloneNode(true);
  row.dataset.id = job.id;
  row.dataset.status = job.status;
  row.dataset.url = job.url;
  row.dataset.preset = job.preset;
  if (job.status === "done") row.querySelector(".job-status").prepend(icon("check"));
  if (job.status === "failed") row.querySelector(".job-status").prepend(icon("warning"));
  row.querySelector(".bar").hidden = !["downloading", "merging"].includes(job.status);
  row.querySelector(".job-actions").append(...actionsFor(job));
  return row;
}

function updateRow(row, job) {
  const percent = job.status === "merging" ? 100 : (job.percent ?? 0);
  row.querySelector(".job-title").textContent = job.title ?? titles.get(job.id) ?? job.url;
  row.querySelector(".job-meta").textContent = [
    extractorName(job.extractor),
    presetName(job.preset),
  ]
    .filter(Boolean)
    .join(" · ");
  row.querySelector(".job-status-text").textContent = statusText(job);
  row.querySelector(".bar").setAttribute("aria-valuenow", String(Math.round(percent)));
  row.querySelector(".bar-fill").style.width = `${percent}%`;
}

function renderJobs(jobs) {
  const seen = new Set();
  for (const job of jobs) {
    seen.add(job.id);
    let row = rows.get(job.id);
    // A row is only rebuilt when its status changes: replacing it on every poll
    // would swallow a click that lands between two refreshes.
    if (!row || row.dataset.status !== job.status) {
      const fresh = buildRow(job);
      if (row) row.replaceWith(fresh);
      else $("jobs").prepend(fresh);
      rows.set(job.id, fresh);
      row = fresh;
    }
    updateRow(row, job);
  }
  for (const [id, row] of rows) {
    if (seen.has(id)) continue;
    row.remove();
    rows.delete(id);
    titles.delete(id);
  }
  $("empty").hidden = jobs.length > 0;
}

async function refreshJobs() {
  try {
    renderJobs(await api("GET", "/api/downloads"));
    showMessage($("notice"), null);
  } catch {
    showMessage($("notice"), "Serveur injoignable, nouvel essai en cours…");
  }
}

async function onJobAction(event) {
  const button = event.target.closest("[data-action]");
  if (!button) return;
  const row = button.closest(".job");
  button.disabled = true;
  try {
    if (button.dataset.action === "retry") {
      const { url, preset } = row.dataset;
      const job = await api("POST", "/api/downloads", { url, preset });
      titles.set(job.id, row.querySelector(".job-title").textContent);
    }
    await api("DELETE", `/api/downloads/${row.dataset.id}`);
  } catch (error) {
    // Already gone on the server is what we wanted.
    if (error.code !== "not_found") {
      showMessage($("notice"), "L'action a échoué. Réessaie dans un instant.");
      button.disabled = false;
      return;
    }
  }
  await refreshJobs();
}

async function loadFooter() {
  try {
    const { ytdlp_version: version, max_age_days: days } = await api("GET", "/api/config");
    const unit = days >= 2 ? "jours" : "jour";
    $("footer").textContent =
      `yt-dlp ${version} · fichiers supprimés après ${decimal.format(days)} ${unit}`;
  } catch {
    // The footer is informative only: without it the page works the same.
  }
}

async function poll() {
  if (!document.hidden) await refreshJobs();
  setTimeout(poll, POLL_INTERVAL_MS);
}

$("url-form").addEventListener("submit", analyse);
$("presets").addEventListener("click", selectPreset);
$("download").addEventListener("click", startDownload);
$("jobs").addEventListener("click", onJobAction);
document.addEventListener("visibilitychange", () => {
  if (!document.hidden) refreshJobs();
});

loadFooter();
poll();
