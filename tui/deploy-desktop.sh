#!/usr/bin/bash
# Деплой/обновление десктоп-набора Geekcom Clash. Идемпотентен.
#
# Зовётся из двух мест (поэтому одна логика — нет рассинхрона):
#   1) install.sh — как пользователь (deck), root-части через sudo;
#   2) плагин при загрузке (main.py) — как root, деплоит для GCC_USER.
#
# Параметры через env:
#   GCC_USER       — целевой пользователь (по умолчанию текущий)
#   GCC_PLUGIN_DIR — каталог плагина (по умолчанию ~target/homebrew/plugins/GeekcomClash)
#   GCC_VERSION    — версия (пишется в стамп, чтобы плагин не редеплоил каждый раз)
set -e
export PATH="/usr/sbin:/usr/bin:/sbin:/bin:${PATH}"

TARGET_USER="${GCC_USER:-$(id -un)}"
TARGET_HOME=$(getent passwd "$TARGET_USER" | cut -d: -f6)
PLUGIN_DIR="${GCC_PLUGIN_DIR:-$TARGET_HOME/homebrew/plugins/GeekcomClash}"
SRC="$PLUGIN_DIR/desktop"
APP_DIR="$TARGET_HOME/.local/share/geekcom-clash"
BIN_MIHOMO="$PLUGIN_DIR/bin/mihomo"
VERSION="${GCC_VERSION:-unknown}"

[ -d "$SRC" ] || { echo "deploy-desktop: no $SRC, skip"; exit 0; }

# Выполнить от имени целевого пользователя / от root — независимо от того,
# кто нас запустил.
asuser() { if [ "$(id -un)" = "$TARGET_USER" ]; then "$@"; else runuser -u "$TARGET_USER" -- "$@"; fi; }
asroot() { if [ "$(id -u)" -eq 0 ]; then "$@"; else sudo "$@"; fi; }

# --- файлы приложения (как пользователь) ---
asuser mkdir -p "$APP_DIR" "$TARGET_HOME/.local/share/applications" "$TARGET_HOME/Desktop"
asuser cp -f "$SRC/geekcom-clash-tui" "$SRC/geekcom-clash-ctl" "$SRC/decky_shim.py" "$SRC/logo.png" "$APP_DIR/"
asuser chmod +x "$APP_DIR/geekcom-clash-tui" "$APP_DIR/geekcom-clash-ctl"

# --- ярлык ---
DESKTOP="[Desktop Entry]
Type=Application
Name=Geekcom Clash
GenericName=VPN
Comment=Geekcom Clash VPN
Icon=$APP_DIR/logo.png
Exec=konsole --hide-menubar -p TerminalColumns=84 -p TerminalRows=42 -e $APP_DIR/geekcom-clash-tui
Terminal=false
Categories=Network;System;"
printf '%s\n' "$DESKTOP" | asuser tee "$TARGET_HOME/.local/share/applications/geekcom-clash.desktop" >/dev/null
printf '%s\n' "$DESKTOP" | asuser tee "$TARGET_HOME/Desktop/geekcom-clash.desktop" >/dev/null
asuser chmod +x "$TARGET_HOME/.local/share/applications/geekcom-clash.desktop" "$TARGET_HOME/Desktop/geekcom-clash.desktop"

# --- systemd --user юнит (пути проставит ctl) ---
asuser "$APP_DIR/geekcom-clash-ctl" install-unit || true

# --- setcap + sudoers-самохил (после апдейта mihomo) ---
SETCAP=$(command -v setcap || echo /usr/bin/setcap)
asroot "$SETCAP" 'cap_net_admin,cap_net_raw=+ep' "$BIN_MIHOMO" || true
WRAP="$APP_DIR/geekcom-setcap"
printf '#!/usr/bin/bash\nexec "%s" "cap_net_admin,cap_net_raw=+ep" "%s"\n' "$SETCAP" "$BIN_MIHOMO" | asuser tee "$WRAP" >/dev/null
asuser chmod +x "$WRAP"
TMP=$(mktemp)
printf '%s ALL=(root) NOPASSWD: %s\n' "$TARGET_USER" "$WRAP" > "$TMP"
if asroot visudo -cf "$TMP" >/dev/null 2>&1; then
  # ВАЖНО: имя файла должно сортироваться ПОСЛЕ /etc/sudoers.d/wheel
  # (%wheel ALL=(ALL) ALL — с паролем). sudo применяет последнее совпадение,
  # поэтому при имени "geekcom-clash" (до "wheel") правило wheel перебивало
  # наш NOPASSWD. Префикс "zz-" гарантирует, что наше правило — последнее.
  asroot install -m 0440 -o root -g root "$TMP" /etc/sudoers.d/zz-geekcom-clash
  asroot rm -f /etc/sudoers.d/geekcom-clash   # снести старое (битое) имя
fi
rm -f "$TMP"

# --- polkit (без окна пароля для resolved) ---
asroot install -m 0644 "$SRC/49-geekcom-clash.rules" /etc/polkit-1/rules.d/49-geekcom-clash.rules || true

# --- права: ctl (пользователь) должен писать настройки/конфиг ---
asroot chown -R "$TARGET_USER:$TARGET_USER" \
  "$TARGET_HOME/homebrew/settings/GeekcomClash" "$TARGET_HOME/homebrew/data/GeekcomClash" 2>/dev/null || true

# --- стамп версии (чтобы плагин не редеплоил каждую загрузку) ---
printf '%s\n' "$VERSION" | asuser tee "$APP_DIR/.deployed-version" >/dev/null

echo "deploy-desktop: ok for $TARGET_USER ($VERSION)"
