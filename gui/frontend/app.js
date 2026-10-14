"use strict";

// Pont vers le backend Go (injecté par Wails) et vers les événements.
const backend = () => window.go && window.go.main && window.go.main.App;
const rt = () => window.runtime;

const $ = (id) => document.getElementById(id);
const el = {
  videoDir: $("videoDir"), name: $("name"), cols: $("cols"), rows: $("rows"),
  frameWidth: $("frameWidth"), sort: $("sort"), openBrowser: $("openBrowser"),
  background: $("background"), foreground: $("foreground"),
  backgroundHex: $("backgroundHex"), foregroundHex: $("foregroundHex"),
  console: $("console"), progress: $("progress"), status: $("status"),
  create: $("create"), cancel: $("cancel"), reset: $("reset"),
  legacy: $("legacy"), quit: $("quit"), browse: $("browse"), version: $("version"),
};

let running = false;
let toolsProblem = ""; // message si ffmpeg est introuvable

// ---------- formulaire <-> préférences ----------

function fillForm(p) {
  el.videoDir.value = p.videoDir || "";
  el.name.value = p.name || "";
  el.cols.value = p.cols;
  el.rows.value = p.rows;
  el.frameWidth.value = p.frameWidth;
  el.sort.checked = !!p.sort;
  el.openBrowser.checked = !!p.openBrowser;
  el.background.value = p.background;
  el.foreground.value = p.foreground;
  showColors();
  refreshState();
}

function readForm() {
  return {
    videoDir: el.videoDir.value.trim(),
    name: el.name.value.trim(),
    cols: parseInt(el.cols.value, 10),
    rows: parseInt(el.rows.value, 10),
    frameWidth: parseInt(el.frameWidth.value, 10),
    sort: el.sort.checked,
    background: el.background.value,
    foreground: el.foreground.value,
    openBrowser: el.openBrowser.checked,
  };
}

function showColors() {
  el.backgroundHex.textContent = el.background.value;
  el.foregroundHex.textContent = el.foreground.value;
}

// Renvoie un message d'erreur, ou "" si le formulaire est valide.
function validate(p) {
  if (!p.videoDir) return "Indiquez le répertoire des vidéos.";
  const inRange = (n, min, max) => Number.isInteger(n) && n >= min && n <= max;
  if (!inRange(p.cols, 2, 5)) return "Miniatures par ligne : entre 2 et 5.";
  if (!inRange(p.rows, 2, 6)) return "Miniatures par colonne : entre 2 et 6.";
  if (!inRange(p.frameWidth, 80, 1920)) return "Largeur des captures : entre 80 et 1920 pixels.";
  return "";
}

function setStatus(text) { el.status.textContent = text; }

function refreshState() {
  const err = validate(readForm());
  el.create.disabled = running || err !== "" || toolsProblem !== "";
  el.reset.disabled = running;
  el.legacy.disabled = running;
  el.browse.disabled = running;
  el.cancel.hidden = !running;
  if (!running) {
    setStatus(toolsProblem ? "ffmpeg est introuvable : voir la console." : err || "Prêt : cliquez sur « Créer la galerie ».");
  }
}

// ---------- console ----------

function log(text, cls) {
  const line = document.createElement("div");
  if (cls) line.className = cls;
  line.textContent = text;
  el.console.appendChild(line);
  el.console.scrollTop = el.console.scrollHeight;
}

function clearConsole() { el.console.replaceChildren(); }

const INTRO = [
  "Prérequis pour créer votre galerie :",
  "",
  "1 - Les vidéos doivent être au format mp4, webm ou ogg, toutes dans le même répertoire.",
  "2 - Plus besoin de fournir une image : les 9 captures de chaque vidéo sont extraites",
  "    automatiquement (ffmpeg doit être installé).",
  "3 - La galerie est créée dans le sous-répertoire « pages » et s'ouvre sans serveur.",
];

function showIntro() {
  clearConsole();
  INTRO.forEach((l) => log(l));
}

// ---------- actions ----------

async function create() {
  const p = readForm();
  const err = validate(p);
  if (err) { setStatus(err); return; }

  running = true;
  clearConsole();
  el.progress.hidden = false;
  el.progress.removeAttribute("value"); // barre indéterminée jusqu'au 1er retour
  refreshState();
  setStatus("Création de la galerie en cours…");
  try {
    await backend().Generate(p); // rend la main aussitôt ; la suite arrive par événements
  } catch (e) {
    onDone({ error: String(e) });
  }
}

function onProgress({ done, total }) {
  el.progress.max = total;
  el.progress.value = done;
  setStatus(`Vidéo ${done} / ${total} traitée`);
}

function onDone(s) {
  running = false;
  el.progress.hidden = true;
  refreshState();

  if (s.cancelled) {
    log("\nGénération annulée.", "err");
    setStatus("Annulé.");
    return;
  }
  if (s.error) {
    log("\nErreur : " + s.error, "err");
    setStatus("Échec de la création de la galerie.");
    return;
  }

  log("");
  log("FIN DE TRAITEMENT", "ok");
  log(`${s.videos} vidéo(s) dans la galerie, ${s.pages} page(s) d'index`);
  log(`Images : ${s.generated} générée(s), ${s.cached} déjà à jour`);
  if (s.failed && s.failed.length) {
    log(`\n${s.failed.length} vidéo(s) ignorée(s) car illisible(s) :`, "err");
    s.failed.forEach((f) => log(`  ${f.video} : ${f.error}`, "err"));
  }
  setStatus("Galerie créée avec succès.");

  if (readForm().openBrowser) {
    backend().OpenInBrowser(s.indexPath).catch((e) => log("Ouverture impossible : " + e, "err"));
    log("\nLa galerie s'ouvre dans le navigateur.");
  }
  log("\nPour la rouvrir plus tard : " + s.indexPath);
}

async function browse() {
  try {
    const dir = await backend().ChooseDirectory(el.videoDir.value.trim());
    if (dir) {
      el.videoDir.value = dir;
      refreshState();
    }
  } catch (e) {
    log("Sélection impossible : " + e, "err");
  }
}

async function resetForm() {
  fillForm(await backend().DefaultPrefs());
  showIntro();
}

async function importLegacy() {
  try {
    const p = await backend().ImportLegacyPrefs();
    if (p) {
      fillForm(p);
      setStatus("Valeurs de l'ancienne version importées.");
    }
  } catch (e) {
    log("Import impossible : " + e, "err");
  }
}

// ---------- initialisation ----------

// Les liaisons Wails sont injectées de façon asynchrone : on les attend un peu.
async function waitForBackend(timeoutMs) {
  const start = Date.now();
  while ((!backend() || !rt()) && Date.now() - start < timeoutMs) {
    await new Promise((r) => setTimeout(r, 50));
  }
  return !!backend() && !!rt();
}

async function init() {
  if (!(await waitForBackend(3000))) {
    setStatus("Backend indisponible : cette page doit être lancée via l'application.");
    el.create.disabled = true;
    return;
  }

  rt().EventsOn("log", (line) => log(line));
  rt().EventsOn("progress", onProgress);
  rt().EventsOn("done", onDone);

  const [prefs, version, problem] = await Promise.all([
    backend().LoadPrefs(), backend().Version(), backend().ToolsStatus(),
  ]);
  toolsProblem = problem;
  el.version.textContent = "The Video Gallery v" + version;
  fillForm(prefs);
  showIntro();
  if (toolsProblem) {
    log("");
    log("⚠ " + toolsProblem, "err");
  }

  for (const id of ["videoDir", "name", "cols", "rows", "frameWidth", "sort", "openBrowser", "background", "foreground"]) {
    el[id].addEventListener("input", () => { showColors(); refreshState(); });
  }
  el.browse.addEventListener("click", browse);
  el.create.addEventListener("click", create);
  el.cancel.addEventListener("click", () => backend().Cancel());
  el.reset.addEventListener("click", resetForm);
  el.legacy.addEventListener("click", importLegacy);
  el.quit.addEventListener("click", () => backend().Quit());
}

init();
