// Package service — управление systemd --user службой geekcom-clash.service
// (порт start/stop/restart/status из py_modules ctl). Один источник истины
// для жизненного цикла VPN: плагин и GUI зовут эти команды движка.
package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const Unit = "geekcom-clash.service"

// systemctl --user требует XDG_RUNTIME_DIR; в SSH/юните его может не быть.
func ensureRuntimeDir() {
	if os.Getenv("XDG_RUNTIME_DIR") == "" {
		os.Setenv("XDG_RUNTIME_DIR", fmt.Sprintf("/run/user/%d", os.Getuid()))
	}
}

func systemctl(args ...string) ([]byte, error) {
	ensureRuntimeDir()
	return exec.Command("systemctl", append([]string{"--user"}, args...)...).CombinedOutput()
}

func Start() error {
	ensureCaps()
	out, err := systemctl("start", Unit)
	if err != nil {
		return fmt.Errorf("systemctl start: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func Stop() error {
	out, err := systemctl("stop", Unit)
	if err != nil {
		return fmt.Errorf("systemctl stop: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func Restart() error {
	ensureCaps()
	out, err := systemctl("restart", Unit)
	if err != nil {
		return fmt.Errorf("systemctl restart: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func IsActive() bool {
	out, _ := systemctl("is-active", Unit)
	return strings.TrimSpace(string(out)) == "active"
}

// ensureCaps — если cap слетел (апдейт mihomo заменил бинарь), вернуть через
// setcap-wrapper (NOPASSWD-sudo, ставится установщиком). Best-effort, молча.
// mihomo и wrapper определяются по env/расположению бинаря.
func ensureCaps() {
	bin := os.Getenv("GEEKCOM_CLASH_MIHOMO")
	if bin == "" {
		return
	}
	if out, err := exec.Command("getcap", bin).Output(); err == nil && strings.Contains(string(out), "cap_net_admin") {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	wrapper := filepath.Join(filepath.Dir(exe), "geekcom-setcap")
	if _, err := os.Stat(wrapper); err != nil {
		return
	}
	_ = exec.Command("sudo", "-n", wrapper).Run()
}
