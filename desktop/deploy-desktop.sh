#!/usr/bin/bash
# Деплой/обновление десктоп-набора Geekcom Clash — NIGHTLY (Go-движок + Fyne-GUI).
# Идемпотентен. Зовётся из двух мест (одна логика — нет рассинхрона):
#   1) плагин при загрузке (main.py _deploy_desktop) — как root, для GCC_USER;
#   2) вручную с рабочего стола.
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
SETTINGS_DIR="$TARGET_HOME/homebrew/settings/GeekcomClash"   # config.json (regen читает)
DATA_DIR="$TARGET_HOME/homebrew/data/GeekcomClash"           # running_config + mihomo -d
DASH_DIR="$DATA_DIR/dashboard"                               # external-ui
VERSION="${GCC_VERSION:-unknown}"

[ -d "$SRC" ] || { echo "deploy-desktop: no $SRC, skip"; exit 0; }
[ -f "$SRC/geekcom-clash" ] || { echo "deploy-desktop: no engine binary in $SRC, skip"; exit 0; }
[ -f "$SRC/geekcom-clash-gui" ] || { echo "deploy-desktop: no GUI binary in $SRC, skip"; exit 0; }

asuser() { if [ "$(id -un)" = "$TARGET_USER" ]; then "$@"; else runuser -u "$TARGET_USER" -- "$@"; fi; }
asroot() { if [ "$(id -u)" -eq 0 ]; then "$@"; else sudo "$@"; fi; }

# --- файлы приложения (как пользователь) ---
asuser mkdir -p "$APP_DIR" "$TARGET_HOME/.local/share/applications" "$TARGET_HOME/Desktop"
asuser cp -f "$SRC/geekcom-clash" "$SRC/geekcom-clash-gui" "$SRC/logo.png" "$APP_DIR/"
asuser chmod +x "$APP_DIR/geekcom-clash" "$APP_DIR/geekcom-clash-gui"

# --- launcher GUI: пробрасывает канонический env (settings/data/mihomo) ---
LAUNCH="$APP_DIR/geekcom-clash-gui-launch"
printf '#!/usr/bin/bash\nexport GEEKCOM_CLASH_DIR=%q\nexport GEEKCOM_CLASH_MIHOMO=%q\nexport GEEKCOM_CLASH_RESOURCE_DIR=%q\nexec %q "$@"\n' \
  "$SETTINGS_DIR" "$BIN_MIHOMO" "$DATA_DIR" "$APP_DIR/geekcom-clash-gui" | asuser tee "$LAUNCH" >/dev/null
asuser chmod +x "$LAUNCH"

# --- ярлык (GUI напрямую, не терминал) ---
DESKTOP="[Desktop Entry]
Type=Application
Name=Geekcom Clash
GenericName=VPN
Comment=Geekcom Clash VPN
Icon=$APP_DIR/logo.png
Exec=$LAUNCH
Terminal=false
Categories=Network;System;"
printf '%s\n' "$DESKTOP" | asuser tee "$TARGET_HOME/.local/share/applications/geekcom-clash.desktop" >/dev/null
printf '%s\n' "$DESKTOP" | asuser tee "$TARGET_HOME/Desktop/geekcom-clash.desktop" >/dev/null
asuser chmod +x "$TARGET_HOME/.local/share/applications/geekcom-clash.desktop" "$TARGET_HOME/Desktop/geekcom-clash.desktop"

# --- systemd --user юнит (ExecStartPre = ЭТОТ Go-движок regen, единый источник) ---
asuser env GEEKCOM_CLASH_DIR="$SETTINGS_DIR" GEEKCOM_CLASH_MIHOMO="$BIN_MIHOMO" \
  GEEKCOM_CLASH_RESOURCE_DIR="$DATA_DIR" GEEKCOM_CLASH_DASHBOARD_DIR="$DASH_DIR" \
  "$APP_DIR/geekcom-clash" install-unit || true

# --- setcap + sudoers-самохил (после апдейта mihomo cap слетает) ---
SETCAP=$(command -v setcap || echo /usr/bin/setcap)
asroot "$SETCAP" 'cap_net_admin,cap_net_raw=+ep' "$BIN_MIHOMO" || true
WRAP="$APP_DIR/geekcom-setcap"
printf '#!/usr/bin/bash\nexec "%s" "cap_net_admin,cap_net_raw=+ep" "%s"\n' "$SETCAP" "$BIN_MIHOMO" | asuser tee "$WRAP" >/dev/null
asuser chmod +x "$WRAP"
TMP=$(mktemp)
printf '%s ALL=(root) NOPASSWD: %s\n' "$TARGET_USER" "$WRAP" > "$TMP"
if asroot visudo -cf "$TMP" >/dev/null 2>&1; then
  # zz- чтобы сортировалось ПОСЛЕ /etc/sudoers.d/wheel (иначе %wheel с паролем перебивает)
  asroot install -m 0440 -o root -g root "$TMP" /etc/sudoers.d/zz-geekcom-clash
  asroot rm -f /etc/sudoers.d/geekcom-clash
fi
rm -f "$TMP"

# --- polkit (resolved без окна пароля) ---
asroot install -m 0644 "$SRC/49-geekcom-clash.rules" /etc/polkit-1/rules.d/49-geekcom-clash.rules || true

# --- права: дек-юзер должен писать настройки/конфиг ---
asroot chown -R "$TARGET_USER:$TARGET_USER" "$SETTINGS_DIR" "$DATA_DIR" 2>/dev/null || true

# --- стамп версии ---
printf '%s\n' "$VERSION" | asuser tee "$APP_DIR/.deployed-version" >/dev/null
echo "deploy-desktop: ok for $TARGET_USER ($VERSION)"
