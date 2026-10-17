package gallery

import "testing"

func TestParseMountinfo(t *testing.T) {
	for line, want := range map[string][2]string{
		"36 35 98:0 / /mnt/Public rw,relatime shared:1 - cifs //192.168.1.102/public rw,vers=3.1.1": {"/mnt/Public", "cifs"},
		"40 30 0:50 / /media/Mes\\040vidéos rw - ext4 /dev/sdb1 rw":                                 {"/media/Mes vidéos", "ext4"},
		"50 30 0:60 / /run/user/1000/gvfs rw,nosuid master:3 - fuse.gvfsd-fuse gvfsd-fuse rw":       {"/run/user/1000/gvfs", "fuse.gvfsd-fuse"},
	} {
		mnt, typ, ok := parseMountinfo(line)
		if !ok || mnt != want[0] || typ != want[1] {
			t.Errorf("%q : %q, %q, %v ; attendu %q", line, mnt, typ, ok, want)
		}
	}
	if !within("/mnt/Public/films", "/mnt/Public") || within("/mnt/Publicité", "/mnt/Public") {
		t.Error("within : préfixe de chemin mal reconnu")
	}
}

func TestNetworkFSLocal(t *testing.T) {
	if fs, ok := networkFS(t.TempDir()); ok {
		t.Errorf("dossier temporaire pris pour un partage réseau (%s)", fs)
	}
}
