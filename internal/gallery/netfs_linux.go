package gallery

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// networkTypes : systèmes de fichiers réseau reconnus dans /proc/self/mountinfo.
var networkTypes = map[string]bool{
	"nfs": true, "nfs4": true, "cifs": true, "smb3": true, "smbfs": true,
	"ncpfs": true, "afs": true, "ceph": true, "glusterfs": true, "davfs": true,
	// montages FUSE : Nautilus (smb://, sftp://...), sshfs, rclone
	"fuse.gvfsd-fuse": true, "fuse.sshfs": true, "fuse.rclone": true, "fuse.s3fs": true,
}

// networkFS indique si dir est sur un partage réseau, et lequel (type du
// montage, par exemple « cifs »).
func networkFS(dir string) (string, bool) {
	path, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	if p, err := filepath.EvalSymlinks(path); err == nil {
		path = p
	}
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return "", false
	}
	defer f.Close()

	// Le montage qui contient dir est celui dont le point de montage est le
	// plus long préfixe de son chemin.
	best, fstype := -1, ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		mnt, typ, ok := parseMountinfo(sc.Text())
		if ok && len(mnt) > best && within(path, mnt) {
			best, fstype = len(mnt), typ
		}
	}
	return fstype, networkTypes[fstype]
}

// parseMountinfo lit le point de montage (5e champ) et le type (premier champ
// après « - ») d'une ligne de /proc/self/mountinfo.
func parseMountinfo(line string) (mnt, fstype string, ok bool) {
	fields := strings.Fields(line)
	if len(fields) < 5 {
		return "", "", false
	}
	for i := 5; i < len(fields)-1; i++ {
		if fields[i] == "-" {
			return unescapeMount(fields[4]), fields[i+1], true
		}
	}
	return "", "", false
}

// unescapeMount décode les octets échappés en octal par le noyau (« \040 »
// pour une espace).
func unescapeMount(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+4 <= len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// within : path est mnt ou se trouve dessous.
func within(path, mnt string) bool {
	return mnt == "/" || path == mnt || strings.HasPrefix(path, mnt+"/")
}
