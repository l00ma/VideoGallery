package main

import (
	"flag"
	"strings"

	"videogallery/internal/gallery"
)

// helpText renvoie l'aide complète (-h, -help). Les valeurs par défaut sont
// lues dans les options déclarées, pour ne jamais diverger du code ; un test
// vérifie que chaque option y est documentée.
func helpText(fs *flag.FlagSet, prog string) string {
	def := func(name string) string { return fs.Lookup(name).DefValue }
	r := strings.NewReplacer(
		"{prog}", prog,
		"{version}", gallery.Version,
		"{formats}", gallery.Formats,
		"{cols}", def("cols"),
		"{rows}", def("rows"),
		"{frame-width}", def("frame-width"),
		"{bg}", def("bg"),
		"{fg}", def("fg"),
		"{sort}", def("sort"),
	)
	return r.Replace(helpTemplate)
}

const helpTemplate = `The Video Gallery {version} : galerie HTML statique pour un dossier de vidéos.

UTILISATION
  {prog} [options] <dossier des vidéos>

  Pour chaque vidéo, le programme extrait 9 captures (planche de 3 × 3) et
  crée une page avec un lecteur ; un index de miniatures les regroupe.
  La galerie est écrite dans <dossier des vidéos>/pages/ : ouvrez-y
  index.html avec un navigateur, aucun serveur n'est nécessaire.

  Formats acceptés : {formats} (les sous-dossiers sont ignorés).
  ffmpeg et ffprobe sont requis : ils sont cherchés à côté du programme,
  puis dans le PATH (Ubuntu : sudo apt install ffmpeg).

ÉCRIRE LES OPTIONS
  - Les options se placent avant ou après le dossier des vidéos.
  - Une option qui attend une valeur s'écrit, au choix :
      -cols 3    -cols=3    --cols 3    --cols=3
    (un ou deux tirets, valeur séparée par une espace ou par =).
  - Une option sans valeur (-open, -force...) s'écrit seule pour l'activer.
    Pour la désactiver explicitement, il faut écrire -open=false :
    « -open false » ne marche PAS, false serait pris pour le nom du dossier.
  - Une valeur qui contient des espaces doit être entre guillemets :
      -name "Été 2024"
    Sans eux, 2024 serait pris pour le nom du dossier.
  - Une valeur qui commence par # (les couleurs) doit être entre
    guillemets, ou collée à l'option par = : -bg '#3d3846' ou -bg=#3d3846.
    Sans cela, le terminal prend # pour le début d'un commentaire et
    ignore la suite de la ligne.
  - Si le nom du dossier commence par un tiret, placez -- juste avant lui :
      {prog} -cols 3 -- -mon-dossier
    -- signifie « ce qui suit n'est pas une option ». Les options placées
    après le dossier restent, elles, reconnues.

EXEMPLES
  {prog} ~/Vidéos/Vacances
      Galerie avec les réglages par défaut, dans ~/Vidéos/Vacances/pages/.

  {prog} -sort date-desc -cols 3 -rows 4 -open ~/Vidéos/Vacances
      Vidéos les plus récentes d'abord, 3 miniatures par ligne et 4 lignes
      par page, puis ouverture de la galerie dans le navigateur.

  {prog} -name "Été 2024" -bg '#3d3846' -fg '#baaf91' ~/Vidéos/Vacances
      Titre personnalisé, fond sombre et texte clair. Les guillemets autour
      du titre (espace) et des couleurs (#) sont nécessaires ; voir
      « Écrire les options » ci-dessus.

  {prog} -out /media/cle/galerie ~/Vidéos/Vacances
      Galerie écrite ailleurs que dans le dossier des vidéos.

GALERIE
  -name TEXTE       Titre affiché en haut des pages.
                    Défaut : nom du dossier des vidéos.

  -out DOSSIER      Dossier où écrire la galerie.
                    Défaut : <dossier des vidéos>/pages.
                    Par sécurité, un dossier existant qui contient autre
                    chose qu'une galerie est refusé : la génération y
                    remplace des pages et y supprime des images.

  -sort ORDRE       Ordre des vidéos dans l'index et dans la navigation
                    précédent / suivant. Défaut : {sort}.
                      name            nom, de A à Z
                      name-desc       nom, de Z à A
                      date-desc       plus récentes d'abord
                      date            plus anciennes d'abord
                      duration-desc   plus longues d'abord
                      duration        plus courtes d'abord
                      size-desc       plus lourdes d'abord
                      size            plus légères d'abord
                      none            ordre du système de fichiers
                    La date est celle du tournage quand la vidéo l'indique
                    (téléphones, caméras), sinon celle du fichier.

  -no-sort          Ancien nom de « -sort none », conservé pour les scripts
                    existants.

  -open             Ouvre la galerie dans le navigateur par défaut à la fin.

MISE EN PAGE
  -cols N           Nombre de miniatures par ligne dans l'index.
                    Défaut : {cols}.

  -rows N           Nombre de lignes de miniatures par page d'index.
                    Défaut : {rows}. Au-delà de cols × rows vidéos, l'index
                    est découpé en plusieurs pages.

  -frame-width PX   Largeur, en pixels, de chacune des 9 captures de la
                    planche d'une vidéo (de 80 à 1920 ; la hauteur suit le
                    format de la vidéo). Défaut : {frame-width}.

  -bg COULEUR       Couleur de fond des pages, au format #rrggbb.
                    Défaut : {bg}.
                    S'écrit -bg '#3d3846' (entre guillemets) ou
                    -bg=#3d3846 (collée par =, guillemets inutiles).

  -fg COULEUR       Couleur du texte, au format #rrggbb. Défaut : {fg}.
                    S'écrit -fg '#baaf91' ou -fg=#baaf91.

                    Attention : -bg #3d3846, sans guillemets ni =, ne
                    marche pas. Le terminal prend # pour le début d'un
                    commentaire, l'option reste sans valeur et le
                    programme répond « cette option attend une valeur ».

GÉNÉRATION
  -force            Régénère toutes les images, même celles qui sont à jour.
                    Sans cette option, seules les images absentes ou plus
                    anciennes que leur vidéo sont refaites : relancer la
                    commande sur le même dossier est presque instantané.

  -jobs N           Nombre de vidéos traitées en même temps.
                    Défaut : autant que de processeurs.

MISES À JOUR
  -update           Télécharge et installe la dernière version publiée sur
                    GitHub, puis quitte (aucune galerie n'est générée).

  -no-update-check  Ne cherche pas de nouvelle version. Sans cette option,
                    le programme vérifie au plus une fois par jour et le
                    signale dans le terminal.

DIVERS
  -version          Affiche la version, puis quitte.

  -h, -help         Affiche cette aide.

CODES DE RETOUR
  0   galerie générée
  1   erreur : dossier introuvable, ffmpeg absent, valeur refusée (couleur,
      largeur, ordre de tri...)
  2   utilisation incorrecte : dossier manquant, option inconnue ou mal
      écrite (-cols trois)
  3   galerie générée, mais des vidéos illisibles ont été ignorées
`
