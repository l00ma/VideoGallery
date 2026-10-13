#!/usr/bin/env bash
#
# build.sh : construit les livrables de The Video Gallery dans dist/
#
#   ./build.sh                  tests, puis Linux et Windows
#   ./build.sh --only linux     une seule plateforme (linux | windows)
#   ./build.sh --skip-tests     saute gofmt / go vet / go test
#
# Produit, pour chaque plateforme, une archive contenant l'interface
# graphique (VideoGallery), la ligne de commande (videogallery), un LISEZMOI
# et les licences des composants tiers, plus un fichier SHA256SUMS.
#
# ffmpeg n'est PAS embarqué : voir le LISEZMOI de chaque archive.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT"

DIST="$ROOT/dist"
ONLY="all"
RUN_TESTS=1

while [ $# -gt 0 ]; do
	case "$1" in
		--only)
			ONLY="${2:-}"
			case "$ONLY" in linux | windows) ;; *) echo "--only attend « linux » ou « windows »" >&2; exit 2 ;; esac
			shift 2 ;;
		--skip-tests) RUN_TESTS=0; shift ;;
		-h | --help) sed -n '2,14p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
		*) echo "option inconnue : $1 (voir --help)" >&2; exit 2 ;;
	esac
done

step() { printf '\n\033[1;34m==> %s\033[0m\n' "$*"; }
fail() { printf '\033[1;31merreur :\033[0m %s\n' "$*" >&2; exit 1; }
want() { [ "$ONLY" = all ] || [ "$ONLY" = "$1" ]; }

# ---------- prérequis ----------

step "Vérification des prérequis"
command -v go >/dev/null || fail "go introuvable"
command -v zip >/dev/null || fail "zip introuvable (sudo apt install zip)"
command -v tar >/dev/null || fail "tar introuvable"
command -v sha256sum >/dev/null || fail "sha256sum introuvable"

WAILS="$(command -v wails || true)"
[ -n "$WAILS" ] || WAILS="$HOME/go/bin/wails"
[ -x "$WAILS" ] || fail "wails introuvable (go install github.com/wailsapp/wails/v2/cmd/wails@latest)"

if want linux; then
	pkg-config --exists gtk+-3.0 webkit2gtk-4.1 ||
		fail "bibliothèques de compilation manquantes (sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev)"
fi

VERSION="$(sed -n 's/^const Version = "\(.*\)"/\1/p' internal/gallery/gallery.go)"
[ -n "$VERSION" ] || fail "version introuvable dans internal/gallery/gallery.go"
# Windows n'accepte qu'une version numérique : wails.json doit rester cohérent.
WJ_VERSION="$(sed -n 's/.*"productVersion": *"\([^"]*\)".*/\1/p' gui/wails.json)"
[ "$WJ_VERSION" = "${VERSION%%-*}" ] ||
	fail "gui/wails.json : productVersion=$WJ_VERSION, attendu ${VERSION%%-*} (version de gallery.go : $VERSION)"
echo "version : $VERSION"

# ---------- contrôle qualité ----------

if [ "$RUN_TESTS" = 1 ]; then
	step "gofmt, go vet, go test"
	unformatted="$(gofmt -l .)"
	[ -z "$unformatted" ] || fail "fichiers non formatés (gofmt -w) :
$unformatted"
	go vet ./...
	go test -race ./...
	# Le GUI est un autre module : on le vérifie pour la cible Windows (sans
	# CGO) ; la cible Linux est vérifiée par la compilation elle-même.
	(cd gui && GOOS=windows go vet -tags desktop,production .)
fi

# ---------- compilation ----------

# dist/ est entièrement produit par ce script.
step "Nettoyage de dist/"
rm -rf "$DIST"
mkdir -p "$DIST"

WAILS_FLAGS=(-s -skipbindings -clean -trimpath)

build_platform() { # $1 = linux | windows
	local os="$1" ext="" gui_out
	[ "$os" = windows ] && ext=".exe"

	step "Interface graphique ($os/amd64)"
	if [ "$os" = linux ]; then
		(cd gui && "$WAILS" build -platform linux/amd64 -tags webkit2_41 "${WAILS_FLAGS[@]}" -o VideoGallery)
	else
		(cd gui && "$WAILS" build -platform windows/amd64 "${WAILS_FLAGS[@]}" -o VideoGallery.exe)
	fi
	gui_out="gui/build/bin/VideoGallery$ext"
	[ -f "$gui_out" ] || fail "$gui_out n'a pas été produit"

	step "Ligne de commande ($os/amd64)"
	mkdir -p "$DIST/stage-$os"
	GOOS="$os" GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" \
		-o "$DIST/stage-$os/videogallery$ext" ./cmd/videogallery
	cp "$gui_out" "$DIST/stage-$os/VideoGallery$ext"
}

# ---------- licences des composants tiers ----------

# Liste « chemin@version dossier » des modules réellement liés dans un binaire.
# Les options de go list doivent précéder le paquet.
linked_modules() { # $1 = répertoire, $2 = GOOS, $3 = tags (peut être vide), $4 = paquet
	local dir="$1" goos="$2" tags="$3" pkg="$4"
	(cd "$dir" && GOOS="$goos" GOARCH=amd64 go list -tags "$tags" -deps \
		-f '{{with .Module}}{{if .Version}}{{.Path}}@{{.Version}} {{.Dir}}{{end}}{{end}}' "$pkg")
}

write_licenses() { # $1 = fichier de sortie, reste = plateformes
	local out="$1"
	shift
	{
		echo "The Video Gallery $VERSION - composants tiers"
		echo "=============================================="
		echo
		echo "Ce programme embarque les composants suivants, distribués sous leurs"
		echo "licences respectives (reproduites ci-dessous). Les polices utilisées"
		echo "pour le texte des planches de captures sont les polices Go"
		echo "(golang.org/x/image/font/gofont), sous licence BSD."
		echo
	} >"$out"

	{
		linked_modules . linux "" ./cmd/videogallery
		for os in "$@"; do
			if [ "$os" = linux ]; then
				linked_modules gui linux desktop,production,webkit2_41 .
			else
				linked_modules gui windows desktop,production .
			fi
		done
	} | grep -v '^videogallery@' | sort -u | while read -r mod dir; do
		lic="$(find "$dir" -maxdepth 1 -type f \( -iname 'LICENSE*' -o -iname 'COPYING*' \) | sort | head -1)"
		{
			echo "----------------------------------------------------------------"
			echo "$mod"
			echo "----------------------------------------------------------------"
			if [ -n "$lic" ]; then
				cat "$lic"
			else
				echo "(aucun fichier de licence trouvé dans le module)"
				echo "AVERTISSEMENT: licence manquante pour $mod" >&2
			fi
			echo
		} >>"$out"
	done
}

# ---------- LISEZMOI ----------

write_readme() { # $1 = linux | windows, $2 = fichier de sortie
	if [ "$1" = linux ]; then
		cat >"$2" <<EOF
The Video Gallery $VERSION  (Linux, 64 bits)
============================================

Génère une galerie HTML statique pour un répertoire de vidéos (mp4, webm, ogg).
La galerie s'ouvre directement dans le navigateur, sans serveur.

CONTENU
  VideoGallery   interface graphique
  videogallery   même moteur, en ligne de commande
  install.sh     ajoute « The Video Gallery » au menu des applications

PRÉREQUIS
  sudo apt install ffmpeg libwebkit2gtk-4.1-0 libgtk-3-0

  ffmpeg extrait les 9 captures de chaque vidéo. Il n'est pas fourni.

UTILISATION
  ./VideoGallery                       (interface graphique)
  ./videogallery --help                (ligne de commande)
  ./videogallery -cols 4 -rows 5 -name "Ma galerie" /chemin/vers/videos

La galerie est écrite dans le sous-répertoire « pages » du répertoire des
vidéos. Ouvrez pages/index.html. Les images déjà générées sont réutilisées :
relancer sur le même répertoire est quasi instantané.

MENU DES APPLICATIONS (optionnel, sans sudo)
  ./install.sh              ajoute l'entrée de menu et l'icône
  ./install.sh --uninstall  les retire
  Les exécutables restent dans ce dossier : ne le déplacez pas après
  l'installation (sinon, relancez ./install.sh).

Réglages de l'interface : ~/.config/VideoGallery/prefs.json

LICENCES
  Voir THIRD_PARTY_LICENSES.txt.
EOF
	else
		cat >"$2" <<EOF
The Video Gallery $VERSION  (Windows, 64 bits)
==============================================

Génère une galerie HTML statique pour un répertoire de vidéos (mp4, webm, ogg).
La galerie s'ouvre directement dans le navigateur, sans serveur.

CONTENU
  VideoGallery.exe   interface graphique
  videogallery.exe   même moteur, en ligne de commande

PRÉREQUIS
  1. ffmpeg et ffprobe (non fournis) : téléchargez un « build » Windows depuis
     https://ffmpeg.org/download.html puis copiez ffmpeg.exe et ffprobe.exe
     dans ce dossier, à côté de VideoGallery.exe. Le programme les y cherche
     en premier (sinon dans le PATH).
  2. Microsoft WebView2 Runtime : déjà présent sur Windows 11 et sur les
     Windows 10 à jour. Sinon : https://developer.microsoft.com/microsoft-edge/webview2/

UTILISATION
  Double-clic sur VideoGallery.exe, ou en ligne de commande :
  videogallery.exe -cols 4 -rows 5 -name "Ma galerie" C:\\chemin\\vers\\videos

La galerie est écrite dans le sous-répertoire « pages » du répertoire des
vidéos. Ouvrez pages\\index.html.

NOTE
  Ces exécutables ne sont pas signés : Windows SmartScreen peut afficher un
  avertissement au premier lancement (« Informations complémentaires » puis
  « Exécuter quand même »).

LICENCES
  Voir THIRD_PARTY_LICENSES.txt.
EOF
	fi
}

# ---------- assemblage ----------

package_platform() { # $1 = linux | windows
	local os="$1" name="VideoGallery-$VERSION-$1-amd64" stage="$DIST/stage-$1"
	step "Archive $os"
	write_readme "$os" "$stage/LISEZMOI.txt"
	if [ "$os" = linux ]; then
		# entrée de menu (le .desktop est complété par install.sh avec le chemin réel)
		cp packaging/linux/install.sh packaging/linux/videogallery.desktop.in "$stage/"
		cp gui/build/appicon.png "$stage/videogallery.png"
		chmod 755 "$stage/install.sh"
	fi
	write_licenses "$stage/THIRD_PARTY_LICENSES.txt" "$os"
	rm -rf "${DIST:?}/$name"
	mv "$stage" "$DIST/$name"
	(
		cd "$DIST"
		if [ "$os" = linux ]; then
			# propriétaire neutre : l'archive ne révèle pas le compte de build
			tar --owner=0 --group=0 --numeric-owner --sort=name -czf "$name.tar.gz" "$name"
		else
			zip -qr -X "$name.zip" "$name"
		fi
	)
	rm -rf "${DIST:?}/$name"
}

for os in linux windows; do
	want "$os" || continue
	build_platform "$os"
	package_platform "$os"
done

step "Empreintes SHA-256"
(cd "$DIST" && shopt -s nullglob && sha256sum -- *.tar.gz *.zip | tee SHA256SUMS)

step "Terminé"
ls -lh "$DIST" | tail -n +2 | awk '{print $5, $9}'
