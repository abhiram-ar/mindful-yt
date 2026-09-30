package hosts

import (
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

// Path is where Windows reads its hosts file: the folder named in the
// registry, normally C:\Windows\System32\drivers\etc.
func Path() string {
	dir := `%SystemRoot%\System32\drivers\etc`
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Services\Tcpip\Parameters`, registry.QUERY_VALUE)
	if err == nil {
		if value, _, err := key.GetStringValue("DataBasePath"); err == nil && value != "" {
			dir = value
		}
		key.Close()
	}
	if expanded, err := registry.ExpandString(dir); err == nil {
		dir = expanded
	}
	return filepath.Join(dir, "hosts")
}

// FlushDNS clears Windows's DNS cache, so the block works straight away.
func FlushDNS() { flush("ipconfig", "/flushdns") }
