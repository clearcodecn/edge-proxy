package nginx

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ExecFunc is the subprocess runner. It returns combined output for diagnostics.
type ExecFunc func(name string, args ...string) ([]byte, error)

func defaultExec(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

type Util struct {
	ConfDir   string
	ReloadCmd string
	Exec      ExecFunc
}

func New(confDir, reloadCmd string) *Util {
	return &Util{
		ConfDir:   confDir,
		ReloadCmd: reloadCmd,
		Exec:      defaultExec,
	}
}

func (u *Util) Path(filename string) string {
	return filepath.Join(u.ConfDir, filename)
}

func (u *Util) Exists(filename string) bool {
	_, err := os.Stat(u.Path(filename))
	return err == nil
}

// WriteFile writes content atomically: write to .tmp then rename.
func (u *Util) WriteFile(filename string, content []byte) error {
	target := u.Path(filename)
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, content, 0644); err != nil {
		return fmt.Errorf("write tmp %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename %s -> %s: %w", tmp, target, err)
	}
	return nil
}

func (u *Util) RemoveFile(filename string) error {
	err := os.Remove(u.Path(filename))
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	return err
}

func (u *Util) ReadFile(filename string) ([]byte, error) {
	return os.ReadFile(u.Path(filename))
}

func (u *Util) TestConfig() error {
	out, err := u.Exec("nginx", "-t")
	if err != nil {
		return fmt.Errorf("nginx -t failed: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (u *Util) Reload() error {
	parts := strings.Fields(u.ReloadCmd)
	if len(parts) == 0 {
		return errors.New("nginx reload_cmd is empty")
	}
	out, err := u.Exec(parts[0], parts[1:]...)
	if err != nil {
		return fmt.Errorf("reload (%s) failed: %w\n%s", u.ReloadCmd, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// WriteAndApply writes filename, runs nginx -t and reload. On nginx -t failure
// it restores the previous content (or removes the file if it didn't exist).
func (u *Util) WriteAndApply(filename string, content []byte) error {
	target := u.Path(filename)
	var backup []byte
	var existed bool
	if existing, err := os.ReadFile(target); err == nil {
		backup = existing
		existed = true
	}
	if err := u.WriteFile(filename, content); err != nil {
		return err
	}
	if err := u.TestConfig(); err != nil {
		if existed {
			_ = u.WriteFile(filename, backup)
		} else {
			_ = u.RemoveFile(filename)
		}
		return err
	}
	return u.Reload()
}
