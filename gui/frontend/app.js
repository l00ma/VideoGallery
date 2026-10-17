"use strict";

// Pont vers le backend Go (injecté par Wails) et vers les événements.
const backend = () => window.go && window.go.main && window.go.main.App;
const rt = () => window.runtime;

const $ = (id) => document.getElementById(id);
const el = {
  videoDir: $("videoDir"), name: $("name"), cols: $("cols"), rows: $("rows"),
  frameWidth: $("frameWidth"), order: $("order"), openBrowser: $("openBrowser"),
  background: $("background"), foreground: $("foreground"),
  backgroundHex: $("backgroundHex"), foregroundHex: $("foregroundHex"),
  console: $("console"), progress: $("progress"), status: $("status"),
  create: $("create"), cancel: $("cancel"), reset: $("reset"),
  swapColors: $("swapColors"), quit: $("quit"), browse: $("browse"), version: $("version"),
  checkUpdates: $("checkUpdates"), checkNow: $("checkNow"),
  updateBar: $("updateBar"), updateText: $("updateText"), updateProgress: $("updateProgress"),
  updateNow: $("updateNow"), updateNotes: $("updateNotes"), updateLater: $("updateLater"),
  ffmpegBar: $("ffmpegBar"), ffmpegText: $("ffmpegText"), ffmpegProgress: $("ffmpegProgress"),
  ffmpegGet: $("ffmpegGet"),
};

let pendingUpdate = null; // release proposée (objet renvoyé par CheckUpdate)

let running = false;
let toolsProblem = ""; // message si ffmpeg est introuvable

// ---------- formulaire <-> préférences ----------

function fillForm(p) {
  el.videoDir.value = p.videoDir || "";
  el.name.value = p.name || "";
  el.cols.value = p.cols;
  el.rows.value = p.rows;
  el.frameWidth.value = p.frameWidth;
  el.order.value = p.order || "name";
  el.openBrowser.checked = !!p.openBrowser;
  el.checkUpdates.checked = !!p.checkUpdates;
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
    order: el.order.value,
    background: el.background.value,
    foreground: el.foreground.value,
    openBrowser: el.openBrowser.checked,
    checkUpdates: el.checkUpdates.checked,
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
  el.browse.disabled = running;
  el.cancel.hidden = !running;
  el.updateNow.disabled = running;
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
  "1 - Les vidéos doivent être au format mp4, m4v, mov, webm ou ogg, toutes dans le même répertoire.",
  "2 - Une planche est créée pour chaque vidéo à partir de 9 images extraites",
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

function swapColors() {
  [el.background.value, el.foreground.value] = [el.foreground.value, el.background.value];
  showColors();
  refreshState();
}

// ---------- mises à jour ----------

// manual : vrai quand l'utilisateur a cliqué lui-même (on lui répond toujours) ;
// la vérification automatique reste silencieuse en cas d'échec (hors ligne...).
async function checkUpdate(manual) {
  try {
    const rel = await backend().CheckUpdate();
    if (!rel) {
      if (manual) setStatus("Vous avez déjà la dernière version.");
      return;
    }
    pendingUpdate = rel;
    el.updateText.textContent = `La version ${rel.version} est disponible.`;
    el.updateBar.hidden = false;
  } catch (e) {
    if (manual) setStatus("Recherche de mise à jour impossible : " + e);
  }
}

async function applyUpdate() {
  el.updateNow.disabled = true;
  el.updateLater.disabled = true;
  el.updateProgress.hidden = false;
  el.updateProgress.removeAttribute("value");
  el.updateText.textContent = "Téléchargement et installation… le programme va redémarrer.";
  try {
    await backend().ApplyUpdate(); // en cas de succès, la fenêtre se ferme et se rouvre
  } catch (e) {
    el.updateProgress.hidden = true;
    el.updateNow.disabled = running;
    el.updateLater.disabled = false;
    el.updateText.textContent = "Mise à jour impossible : " + e;
  }
}

function onUpdateProgress({ done, total }) {
  if (total > 0) {
    el.updateProgress.max = total;
    el.updateProgress.value = done;
  }
}

// ---------- ffmpeg (Windows) ----------

// Proposé seulement quand ffmpeg est introuvable et sous Windows (FFmpegOffer
// renvoie null ailleurs). Hors ligne, le bandeau s'affiche quand même :
// FFmpegInstall retentera et dira ce qui ne va pas.
async function offerFFmpeg() {
  let build;
  try {
    build = await backend().FFmpegOffer();
    if (build === null) return;
  } catch (_) { /* hors ligne : bouton sans version ni taille */ }
  el.ffmpegText.textContent = "ffmpeg est introuvable : il est nécessaire pour créer une galerie.";
  if (build) el.ffmpegGet.textContent = `Télécharger ffmpeg ${build.version} (${Math.round(build.size / 1e6)} Mo)`;
  el.ffmpegBar.hidden = false;
}

async function installFFmpeg() {
  el.ffmpegGet.disabled = true;
  el.ffmpegProgress.hidden = false;
  el.ffmpegProgress.removeAttribute("value");
  el.ffmpegText.textContent = "Téléchargement de ffmpeg…";
  try {
    const problem = await backend().FFmpegInstall();
    el.ffmpegProgress.hidden = true;
    if (problem) {
      el.ffmpegGet.disabled = false;
      el.ffmpegText.textContent = "ffmpeg a été installé mais reste inutilisable : " + problem;
      return;
    }
    toolsProblem = "";
    el.ffmpegGet.hidden = true;
    el.ffmpegText.textContent = "ffmpeg est installé : vous pouvez créer votre galerie.";
    log("ffmpeg installé dans le dossier « ffmpeg » du programme.", "ok");
    refreshState();
    setTimeout(() => { el.ffmpegBar.hidden = true; }, 5000);
  } catch (e) {
    el.ffmpegProgress.hidden = true;
    el.ffmpegGet.disabled = false;
    el.ffmpegText.textContent = "Téléchargement de ffmpeg impossible : " + e;
  }
}

function onFFmpegProgress({ done, total }) {
  if (total > 0) {
    el.ffmpegProgress.max = total;
    el.ffmpegProgress.value = done;
    el.ffmpegText.textContent = `Téléchargement de ffmpeg : ${Math.round(done / 1e6)} / ${Math.round(total / 1e6)} Mo`;
  }
}

// La fenêtre suit la hauteur des bandeaux (mise à jour, ffmpeg) : elle
// grandit quand l'un apparaît (ou passe sur deux lignes) et rétrécit d'autant
// quand il disparaît, pour que le reste du formulaire garde sa place. Les
// tailles Wails sont en pixels logiques, comme celles du DOM (Windows : mise à
// l'échelle DPI incluse).
let barSpace = 0;

async function fitBars() {
  let space = 0;
  for (const bar of [el.updateBar, el.ffmpegBar]) {
    if (!bar.hidden) space += bar.offsetHeight + 12; // 12 = gap de <main>
  }
  const delta = space - barSpace;
  barSpace = space;
  if (!delta) return;
  try {
    if (await rt().WindowIsMaximised() || await rt().WindowIsFullscreen()) return;
    const { w, h } = await rt().WindowGetSize();
    const max = window.screen.availHeight || h + delta;
    rt().WindowSetSize(w, Math.min(h + delta, Math.max(h, max)));
  } catch (_) { /* au pire, <main> défile (style.css) */ }
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
  rt().EventsOn("updateProgress", onUpdateProgress);
  rt().EventsOn("ffmpegProgress", onFFmpegProgress);

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

  for (const id of ["videoDir", "name", "cols", "rows", "frameWidth", "order", "openBrowser", "checkUpdates", "background", "foreground"]) {
    el[id].addEventListener("input", () => { showColors(); refreshState(); });
  }
  el.browse.addEventListener("click", browse);
  el.create.addEventListener("click", create);
  el.cancel.addEventListener("click", () => backend().Cancel());
  el.reset.addEventListener("click", resetForm);
  el.swapColors.addEventListener("click", swapColors);
  el.quit.addEventListener("click", () => backend().Quit());
  el.checkNow.addEventListener("click", () => checkUpdate(true));
  el.updateNow.addEventListener("click", applyUpdate);
  el.updateLater.addEventListener("click", () => { el.updateBar.hidden = true; });
  el.updateNotes.addEventListener("click", () => {
    if (pendingUpdate && pendingUpdate.url) rt().BrowserOpenURL(pendingUpdate.url);
  });

  const barObserver = new ResizeObserver(fitBars);
  barObserver.observe(el.updateBar);
  barObserver.observe(el.ffmpegBar);
  el.ffmpegGet.addEventListener("click", installFFmpeg);
  if (toolsProblem) offerFFmpeg();
  if (prefs.checkUpdates) checkUpdate(false);
}

init();
