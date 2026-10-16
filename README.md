<div align="center">

<img src="icon.png" alt="" width="96">

# The Video Gallery

**Génère une galerie HTML statique pour un répertoire de vidéos.**
Aucun serveur, aucun JavaScript dans les pages : la galerie s'ouvre d'un double-clic (`file://`).

</div>

---

## Ce que ça fait

Tu pointes le programme sur un dossier de vidéos (`mp4`, `m4v`, `mov`, `webm`, `ogg`), il écrit dans un sous-dossier `pages/` :

- un **index paginé** de miniatures, avec la grille de ton choix ; chaque miniature porte la durée de la vidéo, avec sa taille, sa résolution et sa date en dessous ;
- une **page par vidéo** : lecteur HTML5, boutons précédent/suivant, retour à la galerie ; un clic sur une capture de la planche lance la vidéo à cet instant, dans le lecteur de la page (petit script facultatif : sans JavaScript, la vidéo s'ouvre à cet instant dans un nouvel onglet) ;
- une **planche de 9 captures** (3×3, prises à 10 %, 20 % … 90 % de la durée) par vidéo, avec en-tête (nom, taille, résolution, durée) et l'instant de chaque capture.

Les captures sont extraites automatiquement avec ffmpeg : plus besoin de fournir une image à côté de chaque vidéo.

Le tout tient dans de simples fichiers HTML/CSS/JPEG : tu peux copier le dossier sur une clé USB, un disque réseau ou un hébergement statique. Les vidéos restent où elles sont, les pages y renvoient par chemin relatif (`../ma-video.mp4`).

### Détails qui comptent

- **Cache** : une image n'est régénérée que si elle est absente ou plus ancienne que la vidéo, et les métadonnées lues par ffprobe (durée, résolution, date de tournage) sont gardées dans `img/meta.json`. Relancer sur le même dossier est quasi instantané (`-force` pour tout refaire).
- **Tolérance aux pannes** : une vidéo illisible est ignorée et signalée, la galerie est générée avec les autres.
- **Prudence sur le dossier de sortie** : la génération supprime les pages et images obsolètes, donc elle **refuse** d'écrire dans un dossier existant qui contient autre chose qu'une galerie.
- **Formats** : un `.mov` (téléphone, appareil photo) est lu par le navigateur comme un MP4. Une vidéo en HEVC/H.265 (format par défaut des iPhone) est acceptée, mais sa lecture dépend du navigateur et du système : la génération le signale, et la page de la vidéo propose de l'ouvrir avec le lecteur du système. Les `.mkv` et `.avi` ne sont pas pris en charge.
- **Rotation** : la résolution affichée tient compte des vidéos tournées à 90°/270°.
- **Parallélisme** : plusieurs vidéos sont traitées en même temps (une par cœur par défaut).

## Ce qu'il te faut

| | Linux (64 bits) | Windows (64 bits) |
|---|---|---|
| **ffmpeg et ffprobe** (obligatoires, non fournis) | `sudo apt install ffmpeg` | à télécharger sur [ffmpeg.org](https://ffmpeg.org/download.html), puis `ffmpeg.exe` et `ffprobe.exe` à côté du programme |
| **Pour l'interface graphique** | `sudo apt install libgtk-3-0 libwebkit2gtk-4.1-0` | [WebView2 Runtime](https://developer.microsoft.com/microsoft-edge/webview2/) (déjà là sur Windows 11 et Windows 10 à jour) |
| **Pour la ligne de commande seule** | rien d'autre | rien d'autre |

ffmpeg et ffprobe sont cherchés d'abord à côté de l'exécutable, puis dans le `PATH`.

> Ubuntu 24.04 : seul `webkit2gtk` 4.1 est disponible, c'est celui qu'utilise le programme.

## Utilisation

Les archives `VideoGallery-<version>-linux-amd64.tar.gz` et `…-windows-amd64.zip` des [releases](../../releases) contiennent l'interface graphique (`VideoGallery`) et la ligne de commande (`videogallery`).

### Interface graphique

Lance `VideoGallery`, puis suis les étapes : répertoire des vidéos, nom de la galerie, nombre de miniatures par ligne (2 à 5) et par colonne (2 à 6). L'ordre des vidéos (nom, date, durée ou taille, dans les deux sens), les couleurs (avec un bouton pour inverser fond et texte), la largeur des captures et l'ouverture automatique dans le navigateur sont dans « Options ». Les réglages sont mémorisés après chaque génération réussie (`~/.config/VideoGallery/prefs.json` sous Linux). La génération peut être annulée à tout moment.

Sous Linux, `./install.sh` ajoute « The Video Gallery » au menu des applications (sans `sudo`) ; `./install.sh --uninstall` le retire.

### Ligne de commande

```bash
videogallery [options] <répertoire des vidéos>

videogallery -cols 4 -rows 5 -name "Vacances" ~/Vidéos/vacances
videogallery -sort date-desc -open ~/Vidéos/vacances
videogallery -h        # aide complète, avec exemples et codes de retour
```

| Option | Défaut | Rôle |
|---|---|---|
| `-name` | nom du dossier | nom affiché de la galerie |
| `-out` | `<dossier>/pages` | répertoire de sortie |
| `-cols`, `-rows` | `4`, `5` | miniatures par ligne / par colonne |
| `-bg`, `-fg` | `#baaf91`, `#333333` | couleurs de fond et de texte (`#rrggbb`) |
| `-frame-width` | `320` | largeur en pixels d'une capture de la planche |
| `-jobs` | nombre de CPU | vidéos traitées en parallèle |
| `-sort` | `name` | ordre des vidéos : `name`, `name-desc`, `date-desc` (récentes d'abord), `date`, `duration-desc` (longues d'abord), `duration`, `size-desc` (lourdes d'abord), `size`, `none` (ordre du système de fichiers). La date est celle du tournage si la vidéo l'indique, sinon celle du fichier |
| `-no-sort` | | équivaut à `-sort none` (conservé pour compatibilité) |
| `-open` | | ouvrir la galerie dans le navigateur à la fin |
| `-force` | | régénérer les images même à jour |
| `-update` | | installer la dernière version publiée |
| `-no-update-check` | | ne pas chercher de nouvelle version |
| `-version` | | afficher la version |

Codes de retour : `0` succès, `1` erreur, `2` mauvais arguments, `3` galerie générée mais certaines vidéos ont été ignorées.

Ouvre ensuite `pages/index.html`.

### Mises à jour

L'interface signale une nouvelle version au démarrage (désactivable) et se met à jour d'un clic, puis redémarre. En ligne de commande : `videogallery -update`. Seuls les fichiers déjà installés et fournis par la release sont remplacés, et l'entrée de menu Linux est rafraîchie. L'archive est vérifiée avec l'empreinte SHA-256 de la release ; elle n'est pas signée.

> La recherche interroge l'API publique de GitHub, sans authentification ; la seule information transmise est la version du programme (en-tête `User-Agent`), en plus de l'adresse IP inhérente à toute requête.

## Compiler soi-même

Prérequis : Go 1.22 pour le moteur. L'interface (`gui/`) demande Go 1.25, téléchargé automatiquement par la toolchain.

```bash
# moteur en ligne de commande (aucune dépendance système)
go build ./cmd/videogallery
go vet ./... && go test -race ./...    # le test d'intégration est ignoré sans ffmpeg

# interface graphique + archives Linux et Windows dans dist/
sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev zip
go install github.com/wailsapp/wails/v2/cmd/wails@latest
./build.sh                  # tests, puis les deux archives et SHA256SUMS
./build.sh --only linux     # une seule plateforme
```

La version Windows est cross-compilée depuis Linux, sans CGO.

## Sous le capot

| Dossier | Contenu |
|---|---|
| `cmd/videogallery` | ligne de commande |
| `internal/gallery` | moteur : analyse des vidéos, planches, pages HTML (templates embarqués) |
| `internal/prefs` | préférences JSON |
| `internal/update` | recherche et installation des mises à jour |
| `gui/` | interface [Wails v2](https://wails.io) (HTML/CSS/JS pur, sans npm), sous-module séparé |
| `packaging/linux/` | entrée de menu (`.desktop`) et `install.sh` |

Le moteur est volontairement léger : bibliothèque standard Go et une seule dépendance, [`golang.org/x/image`](https://pkg.go.dev/golang.org/x/image) (polices et rendu du texte des planches). La GUI ajoute Wails. Les licences des modules réellement liés sont dans `THIRD_PARTY_LICENSES.txt` de chaque archive.

## Historique

Projet né en 2013 en Perl + wxWidgets, réécrit en Go en 2026 (version 1.0.0 de ce dépôt). L'ancienne version Perl n'est plus dans ce dépôt.

## Licence

[MIT](LICENSE) © Shomolooma 2013-2026. Les bibliothèques tierces gardent leurs propres licences (voir `THIRD_PARTY_LICENSES.txt` dans chaque archive).
