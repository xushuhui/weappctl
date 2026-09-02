package config

import "os"

func writeFileErr(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
