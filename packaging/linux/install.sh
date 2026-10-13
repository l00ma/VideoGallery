#!/usr/bin/env bash
#
# Installe (ou retire) l'entrée de menu « The Video Gallery » pour l'utilisateur
# courant, sans sudo. Les exécutables restent là où vous les avez extraits :
# ne déplacez pas ce dossier après l'installation, ou relancez ce script.
#
#   ./install.sh              installe l'entrée de menu et l'icône
#   ./install.sh --uninstall  les retire

set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DATA="${XDG_DATA_HOME:-$HOME/.local/share}"
APPS="$DATA/applications"
ICON_DIR="$DATA/icons/hicolor/256x256/apps"
DESKTOP="$APPS/videogallery.desktop"
ICON="$ICON_DIR/videogallery.png"

refresh_caches() {
	command -v update-desktop-database >/dev/null && update-desktop-database "$APPS" 2>/dev/null || true
	command -v gtk-update-icon-cache >/dev/null && gtk-update-icon-cache -f -t "$DATA/icons/hicolor" 2>/dev/null || true
}

case "${1:-}" in
	--uninstall)
		rm -f "$DESKTOP" "$ICON"
		refresh_caches
		echo "Entrée de menu retirée."
		exit 0 ;;
	"") ;;
	*) echo "usage : $0 [--uninstall]" >&2; exit 2 ;;
esac

EXE="$HERE/VideoGallery"
[ -x "$EXE" ] || { echo "erreur : $EXE introuvable ou non exécutable" >&2; exit 1; }
[ -f "$HERE/videogallery.png" ] || { echo "erreur : $HERE/videogallery.png introuvable" >&2; exit 1; }
[ -f "$HERE/videogallery.desktop.in" ] || { echo "erreur : modèle .desktop introuvable" >&2; exit 1; }

# Le champ Exec d'un .desktop est interprété : on refuse les caractères que la
# spécification oblige à échapper de façon délicate, plutôt que de mal les gérer.
case "$EXE" in
	*[\"\`\$\\]* | *$'\n'*)
		echo "erreur : le chemin $EXE contient un caractère non pris en charge (\" \` \$ \\)." >&2
		echo "Déplacez ce dossier vers un chemin plus simple puis relancez ce script." >&2
		exit 1 ;;
esac
exec_field="\"${EXE//%/%%}\""   # entre guillemets (espaces) ; « % » doublé

mkdir -p "$APPS" "$ICON_DIR"
install -m 644 "$HERE/videogallery.png" "$ICON"

# Substitution ligne à ligne : plus sûr que sed (chemins contenant / ou &).
tmp="$(mktemp "$APPS/.videogallery.XXXXXX")"
trap 'rm -f "$tmp"' EXIT
while IFS= read -r line || [ -n "$line" ]; do
	if [ "$line" = "Exec=@EXEC@" ]; then
		printf 'Exec=%s\n' "$exec_field"
	else
		printf '%s\n' "$line"
	fi
done <"$HERE/videogallery.desktop.in" >"$tmp"
chmod 644 "$tmp"
mv "$tmp" "$DESKTOP"
trap - EXIT

refresh_caches
echo "Installé :"
echo "  $DESKTOP"
echo "  $ICON"
echo "« The Video Gallery » est maintenant dans le menu des applications."
echo "Si une fenêtre déjà ouverte garde l'ancienne icône, fermez-la et relancez-la."
