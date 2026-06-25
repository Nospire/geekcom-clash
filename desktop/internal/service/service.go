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

func configHome() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return x
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config")
}

// InstallUnit пишет systemd --user юнит geekcom-clash.service. ЕДИНЫЙ источник
// истины: ExecStartPre регенит конфиг через ЭТОТ ЖЕ Go-движок (не через старый
// ctl), а пути берёт из env, который плагин/деплой даёт движку:
//   GEEKCOM_CLASH_DIR          — settings-каталог (откуда regen читает config.json)
//   GEEKCOM_CLASH_MIHOMO       — бинарь mihomo
//   GEEKCOM_CLASH_RESOURCE_DIR — data-каталог (running_config + mihomo -d)
//   GEEKCOM_CLASH_DASHBOARD_DIR — (опц.) каталог веб-дашборда для external-ui
func InstallUnit() error {
	dataDir := os.Getenv("GEEKCOM_CLASH_DIR")
	mihomo := os.Getenv("GEEKCOM_CLASH_MIHOMO")
	res := os.Getenv("GEEKCOM_CLASH_RESOURCE_DIR")
	if mihomo == "" || res == "" {
		return fmt.Errorf("install-unit: нужны GEEKCOM_CLASH_MIHOMO и GEEKCOM_CLASH_RESOURCE_DIR")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	running := filepath.Join(res, "running_config.yaml")
	pre := fmt.Sprintf("%s regen -out %s", exe, running)
	if dash := os.Getenv("GEEKCOM_CLASH_DASHBOARD_DIR"); dash != "" {
		pre += " -dashboard-dir " + dash
	}
	unit := fmt.Sprintf(`[Unit]
Description=Geekcom Clash (mihomo) core
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
Environment=GEEKCOM_CLASH_DIR=%s
Environment=GEEKCOM_CLASH_MIHOMO=%s
Environment=GEEKCOM_CLASH_RESOURCE_DIR=%s
ExecStartPre=%s
ExecStart=%s -f %s -d %s
Restart=on-failure
RestartSec=2

[Install]
WantedBy=default.target
`, dataDir, mihomo, res, pre, mihomo, running, res)

	dir := filepath.Join(configHome(), "systemd", "user")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, Unit), []byte(unit), 0o644); err != nil {
		return err
	}
	systemctl("daemon-reload")
	systemctl("enable", Unit)
	return nil
}

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
