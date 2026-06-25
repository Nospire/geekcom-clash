#!/usr/bin/bash

set -e

AUTHOR="Nospire"
REPO_NAME="geekcom-clash"
PACKAGE="GeekcomClash"
GITHUB_BASE_URL=${OVERRIDE_GITHUB_BASE_URL:-"https://github.com"}
API_BASE_URL=${OVERRIDE_API_BASE_URL:-"https://api.github.com"}
SCRIPT_URL="${GITHUB_BASE_URL}/${AUTHOR}/${REPO_NAME}/raw/refs/heads/main/install.sh"
BASE_DIR=${OVERRIDE_BASE_DIR:-"${HOME}/homebrew"}
PLUGIN_DIR=${OVERRIDE_PLUGIN_DIR:-"${BASE_DIR}/plugins/${PACKAGE}"}
DATA_DIR=${OVERRIDE_DATA_DIR:-"${BASE_DIR}/data/${PACKAGE}"}
CLEAN_DIRS=(
  "${PLUGIN_DIR}"
  "${DATA_DIR}"
  "${BASE_DIR}/settings/${PACKAGE}"
  "${BASE_DIR}/logs/${PACKAGE}"
)
SUDO="sudo"

# wget без read-таймаута виснет НАМЕРТВО, если CDN (release-assets) застопорится
# на середине файла. Оборачиваем все вызовы: таймаут на соединение/чтение +
# ретраи + докачка. Прозрачно для всех wget ниже по скрипту.
wget() { command wget --timeout=30 --tries=3 --retry-connrefused --continue "$@"; }

# Зеркало (РФ-достижимое) — пробуем ПЕРВЫМ, fallback на GitHub. GitHub раздаёт
# файлы релизов с Fastly (185.199.x), который РФ-провайдеры троттлят (крупное
# душится в ноль). Зеркало (gdt.geekcom.org/dl) синкает те же ассеты по тем же
# путям — достаточно подменить github.com → зеркало.
MIRROR_BASE="${OVERRIDE_MIRROR_BASE:-https://gdt.geekcom.org/dl}"
fetch() {  # fetch <github_url> <dest>  (без sudo; для root-путей качай в TEMP и mv)
  local url="$1" dest="$2" murl
  if [ -n "$MIRROR_BASE" ]; then
    murl="${url/https:\/\/github.com/$MIRROR_BASE}"
    if [ "$murl" != "$url" ]; then
      echo "  Загрузка с зеркала: $(basename "$dest") ..."
      # -q --show-progress: только прогресс-бар (без verbose), чтобы не выглядело
      # как зависон при тихой загрузке больших файлов (zip/mihomo).
      if command wget -q --show-progress --timeout=20 --tries=2 -O "$dest" "$murl" && [ -s "$dest" ]; then
        return 0
      fi
      echo "  Зеркало недоступно, пробую GitHub ..."
    fi
  fi
  wget -O "$dest" "$url"
}

function usage() {
  echo "Usage: install.sh [options]"
  echo "       curl -L ${SCRIPT_URL} | bash [-s -- [options]]"
  echo
  echo "Options:"
  echo "  -h, --help                  Show this help message and exit"
  echo "  -y, --yes                   Assume yes for all prompts, except specified by args"
  echo "  -v, --version <version>     Specify version"
  echo "      --no-privilege          Run without sudo"
  echo "      --without-plugin        Skip installing ${PACKAGE} plugin"
  echo "      --without-binary        Skip installing Mihomo"
  echo "      --with-geo              Install MetaCubeX geo files (.dat, ~43MB) — по умолчанию НЕ ставятся"
  echo "      --without-geo           (legacy) Skip geo files explicitly"
  echo "      --without-dashboard     Skip installing dashboards"
  echo "      --without-desktop       Skip installing the desktop app (TUI/launcher)"
  echo "      --without-restart       Skip restarting Decky Loader"
  echo "      --clean                 Remove all plugin files (includes config) before installing"
  echo "      --clean-uninstall       Uninstall and remove all plugin files (includes config)"
  echo
  echo "Environment Variables:"
  echo '  OVERRIDE_GITHUB_BASE_URL    Override default: "https://github.com"'
  echo '  OVERRIDE_API_BASE_URL       Override default: "https://api.github.com"'
  echo '  OVERRIDE_BASE_DIR           Override default: "${HOME}/homebrew"'
  echo '  OVERRIDE_PLUGIN_DIR         Override default: "${BASE_DIR}/plugins/${PACKAGE}"'
  echo '  OVERRIDE_DATA_DIR           Override default: "${BASE_DIR}/data/${PACKAGE}"'
  echo
  echo "Examples:"
  echo "  Basic install:   curl -L ${SCRIPT_URL} | bash"
  echo "  Clean install:   curl -L ${SCRIPT_URL} | bash -s -- --clean"
  echo "  Clean uninstall: curl -L ${SCRIPT_URL} | bash -s -- --clean-uninstall"
  echo "  Update blobs:    curl -L ${SCRIPT_URL} | bash -s -- --without-plugin --without-restart"
  echo "  Nightly version: curl -L ${SCRIPT_URL} | bash -s -- --version nightly"
}

function prompt_continue() {
  local bypass_flag=$1
  if [ "$bypass_flag" = "true" ]; then
    echo "Skip this step."
    false
    return
  fi
  if [ "$YES_ALL" = "true" ]; then
    true
    return
  fi

  local response
  read -p "Do you want to continue? [Y/n] " response

  # Convert the response to lowercase
  response=${response,,}

  # Check the response
  if [[ -z "$response" || "$response" == "y" || "$response" == "yes" ]]; then
    true
  elif [[ "$response" == "n" || "$response" == "no" ]]; then
    echo "Skip this step."
    false
  else
    echo "Invalid response. Not continuing."
    false
  fi
}

function clean_all {
  # Полное удаление: плагин/данные/настройки + ВЕСЬ десктоп-набор (движок, GUI,
  # mihomo, лаунчер, стамп в APP_DIR), служба, ярлыки, sudoers/polkit.
  systemctl --user stop geekcom-clash.service 2>/dev/null || true
  systemctl --user disable geekcom-clash.service 2>/dev/null || true
  pkill -f geekcom-clash-gui 2>/dev/null || true
  for dir in "${CLEAN_DIRS[@]}" "${HOME}/.local/share/geekcom-clash"; do
    if [ -e "$dir" ]; then
      echo "Removing $dir ..."
      $SUDO rm -rf "$dir"
    fi
  done
  rm -f "${HOME}/.config/systemd/user/geekcom-clash.service" \
        "${HOME}/.local/share/applications/geekcom-clash.desktop" \
        "${HOME}/Desktop/geekcom-clash.desktop" 2>/dev/null || true
  systemctl --user daemon-reload 2>/dev/null || true
  $SUDO rm -f /etc/sudoers.d/zz-geekcom-clash /etc/sudoers.d/geekcom-clash \
              /etc/polkit-1/rules.d/49-geekcom-clash.rules 2>/dev/null || true
  echo "Удалено: плагин, данные, настройки, десктоп-набор, служба, ярлыки, права."
}

while [[ $# -gt 0 ]]; do
  key=$1
  case $key in
    -y|--yes)
      YES_ALL=true
      shift
      ;;
    -v|--version)
      SPECIFIED_VERSION=$2
      shift
      shift
      ;;
    --component)
      COMPONENT_ARG=$2
      shift
      shift
      ;;
    --no-privilege)
      SUDO=""
      shift
      ;;
    --without-plugin)
      WITHOUT_PLUGIN=true
      shift
      ;;
    --without-binary)
      WITHOUT_BINARY=true
      shift
      ;;
    --without-geo)
      WITHOUT_GEO=true
      shift
      ;;
    --with-geo)
      WITH_GEO=true
      shift
      ;;
    --without-dashboard)
      WITHOUT_DASHBOARD=true
      shift
      ;;
    --without-desktop)
      WITHOUT_DESKTOP=true
      shift
      ;;
    --without-restart)
      WITHOUT_RESTART=true
      shift
      ;;
    --clean)
      clean_all
      shift
      ;;
    --clean-uninstall)
      clean_all
      exit 0
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $key"
      echo
      usage
      exit 1
      ;;
  esac
done

if [ "$UID" -eq 0 ]; then
  echo "WARNING: Running as root."
  echo "This may cause permission issues."
  echo "If you insist to continue, please confirm homebrew path below is correct:"
  echo "${BASE_DIR}"
  echo "In most circumstances, this should NOT be: /root/homebrew"
  echo "You SHOULD run sudo with -E flag to preserve environment variables."
  echo
  if ! prompt_continue; then
    exit 1
  fi
fi

TEMP_DIR=$(mktemp -d)

function finish() {
    rm -rf "$TEMP_DIR"
}
trap finish EXIT

REQUIREMENTS=("curl" "unzip" "tar" "gzip")
for req in "${REQUIREMENTS[@]}"; do
  if ! command -v $req &> /dev/null; then
    echo "Error: $req is not installed"
    exit 1
  fi
done

# Channel selection (Release / Nightly). Skipped if --version is given or --yes.
# ВАЖНО: при `curl ... | bash` stdin занят телом скрипта, поэтому read читаем
# из /dev/tty — иначе ввод не считывается и канал молча уходит в Release.
# Если терминала нет (CI/пайп без tty) — пропускаем и берём Release по умолчанию.
if [ -z "${SPECIFIED_VERSION}" ] && [ "$YES_ALL" != "true" ] && [ -e /dev/tty ]; then
  echo
  echo "Install channel / Канал установки:"
  echo "  1) Release — stable (default / по умолчанию)"
  echo "  2) Nightly — latest test build / свежая тестовая сборка"
  read -p "Choose [1/2]: " CHANNEL < /dev/tty || CHANNEL=""
  case "${CHANNEL,,}" in
    2|n|nightly)
      SPECIFIED_VERSION="nightly"
      echo "  -> Nightly"
      ;;
    *)
      echo "  -> Release"
      ;;
  esac
fi

# Component selection (all / plugin only / desktop-GUI only).
APP_DIR="${HOME}/.local/share/geekcom-clash"
export GCC_WITH_GUI=1
DESKTOP_ONLY=false
DESKTOP_SRC="${PLUGIN_DIR}"          # откуда deploy-desktop берёт бинари
DESKTOP_MIHOMO=""                    # пусто = дефолт deploy-desktop ($PLUGIN_DIR/bin/mihomo)
MIHOMO_BIN_DIR="${PLUGIN_DIR}/bin"   # куда класть mihomo

function set_component() {
  case "$1" in
    plugin)
      echo "  -> Только плагин Decky"; export GCC_WITH_GUI=0 ;;
    desktop)
      echo "  -> Только десктоп-GUI (standalone)"
      DESKTOP_ONLY=true; WITHOUT_RESTART=true
      DESKTOP_SRC="${TEMP_DIR}/${PACKAGE}"     # бинари из распакованного зипа (без homebrew/plugins)
      DESKTOP_MIHOMO="${APP_DIR}/mihomo"
      MIHOMO_BIN_DIR="${APP_DIR}" ;;
    *)
      echo "  -> Всё" ;;
  esac
}

if [ -n "$COMPONENT_ARG" ]; then
  set_component "$COMPONENT_ARG"
elif [ "$YES_ALL" != "true" ] && [ -e /dev/tty ]; then
  echo
  echo "Install components / Что установить:"
  echo "  1) Всё — плагин Decky (Game Mode) + десктоп-GUI (default)"
  echo "  2) Только плагин Decky (Game Mode)"
  echo "  3) Только десктоп-GUI (Desktop Mode, без плагина)"
  read -p "Choose [1/2/3]: " COMP < /dev/tty || COMP=""
  case "$COMP" in
    2) set_component plugin ;;
    3) set_component desktop ;;
    *) set_component all ;;
  esac
fi

echo "LEGAL NOTICE:"
echo "By confirming installation, you agree to the terms of the software and service license."
echo
if [ -n "${SPECIFIED_VERSION}" ]; then
  echo "Installing $REPO_NAME (${SPECIFIED_VERSION}) ..."
else
  echo "Installing $REPO_NAME ..."
fi
if prompt_continue $WITHOUT_PLUGIN; then
  if [ -z "${SPECIFIED_VERSION}" ]; then
    API_URL="${API_BASE_URL}/repos/${AUTHOR}/${REPO_NAME}/releases/latest"
    RELEASE=$(curl -s "$API_URL")
    MESSAGE=$(echo "${RELEASE}" | grep '"message"' | cut -d '"' -f 4)
    RELEASE_VERSION=$(echo "${RELEASE}" | grep '"tag_name"' | cut -d '"' -f 4)
    RELEASE_URL=$(echo "${RELEASE}" | grep "browser_download_url.*GeekcomClash.zip\"" | cut -d '"' -f 4)

    if [[ "${MESSAGE}" != "" ]]; then
      echo "Github Error: ${MESSAGE}" >&2
      exit 1
    fi
    echo "Version: ${RELEASE_VERSION}"
  else
    RELEASE_URL="${GITHUB_BASE_URL}/${AUTHOR}/${REPO_NAME}/releases/download/${SPECIFIED_VERSION}/${PACKAGE}.zip"
  fi
  if [ -z "${RELEASE_URL}" ]; then
    echo "Failed to get latest release" >&2
    exit 1
  fi

  DL_DEST="${TEMP_DIR}/${PACKAGE}.zip"
  fetch "${RELEASE_URL}" "${DL_DEST}"
  unzip -oq "${DL_DEST}" -d "${TEMP_DIR}"
  if [ "$DESKTOP_ONLY" != "true" ]; then
    $SUDO rm -rf "${PLUGIN_DIR}"
    $SUDO mv "${TEMP_DIR}/${PACKAGE}" "${PLUGIN_DIR}"
  fi
  # desktop-only: оставляем распакованным в ${TEMP_DIR}/${PACKAGE} (DESKTOP_SRC)
fi

echo "Installing Binaries ..."
if prompt_continue $WITHOUT_BINARY; then
  BIN_DIR="${MIHOMO_BIN_DIR}"
  # desktop-only: mihomo в APP_DIR (user-owned, без sudo), иначе в plugin/bin (root)
  if [ "$DESKTOP_ONLY" = "true" ]; then BSUDO=""; else BSUDO="$SUDO"; fi
  $BSUDO mkdir -p "${BIN_DIR}"
	echo "Installing Mihomo ..."

  RELEASE=$(curl -s "${API_BASE_URL}/repos/MetaCubeX/mihomo/releases/latest")
  MESSAGE=$(echo "${RELEASE}" | grep '"message"' | cut -d '"' -f 4)
  RELEASE_VERSION=$(echo "${RELEASE}" | grep '"tag_name"' | cut -d '"' -f 4)
	RELEASE_URL=$(echo "${RELEASE}" | grep "browser_download_url.*mihomo-linux-amd64-${RELEASE_VERSION}.gz\"" | cut -d '"' -f 4);

  if [[ "${MESSAGE}" != "" ]]; then
    echo "Github Error: ${MESSAGE}" >&2
    exit 1
  fi
  if [ -z "${RELEASE_URL}" ]; then
    echo "Failed to get latest release" >&2
    exit 1
  fi
  echo "Version: ${RELEASE_VERSION}"

  DL_DEST="${TEMP_DIR}/mihomo.gz"
  INSTALL_DEST="${BIN_DIR}/mihomo"
  fetch "${RELEASE_URL}" "${DL_DEST}"
	gzip -d "${DL_DEST}"
  $BSUDO rm -f "${INSTALL_DEST}"
  $BSUDO mv "${TEMP_DIR}/mihomo" "${INSTALL_DEST}"
	$BSUDO chmod +x "${INSTALL_DEST}"

  # fail-loud: убедиться, что mihomo реально на месте и не обрезан (≥ 10 МБ).
  # Иначе при троттле получалась «тихая» полу-установка без mihomo → VPN не
  # включался (FileNotFoundError у тестера). Лучше упасть внятно, чем молча.
  sz=$(stat -c%s "${INSTALL_DEST}" 2>/dev/null || echo 0)
  if [ ! -s "${INSTALL_DEST}" ] || [ "${sz}" -lt 10000000 ]; then
    echo "" >&2
    echo "ОШИБКА: mihomo не докачался (${INSTALL_DEST}, ${sz} б)." >&2
    echo "Скорее всего сеть/троттлинг прервали загрузку. Запустите установку ещё раз." >&2
    exit 1
  fi
fi

# Geo-файлы (MetaCubeX, ~43 МБ) по умолчанию НЕ ставим: наш конфиг маршрутит
# через RULE-SET rule-providers (ru-domains/ru-geoip/rknasnblock и пр.), а не
# через GEOIP/GEOSITE — китайские .dat ему не нужны (мёртвый груз от DeckyClash).
# Включить можно явно: --with-geo (если у тебя кастомный конфиг с GEOIP/GEOSITE).
if [ "$WITH_GEO" = "true" ]; then
echo "Installing Geo Files ..."
if prompt_continue $WITHOUT_GEO; then
  $SUDO mkdir -p "${DATA_DIR}"

  echo "Downloading geoip.metadb ..."
  RELEASE_URL="${GITHUB_BASE_URL}/MetaCubeX/meta-rules-dat/releases/download/latest/geoip.metadb"
  DEST="${DATA_DIR}/geoip.metadb"
  $SUDO rm -f "${DEST}"
	fetch "${RELEASE_URL}" "${TEMP_DIR}/geodl" && $SUDO mv "${TEMP_DIR}/geodl" "${DEST}"

  echo "Downloading asn.mmdb ..."
  RELEASE_URL="${GITHUB_BASE_URL}/MetaCubeX/meta-rules-dat/releases/download/latest/GeoLite2-ASN.mmdb"
  DEST="${DATA_DIR}/asn.mmdb"
  $SUDO rm -f "${DEST}"
	fetch "${RELEASE_URL}" "${TEMP_DIR}/geodl" && $SUDO mv "${TEMP_DIR}/geodl" "${DEST}"

  echo "Downloading geoip.dat ..."
  RELEASE_URL="${GITHUB_BASE_URL}/MetaCubeX/meta-rules-dat/releases/download/latest/geoip.dat"
  DEST="${DATA_DIR}/geoip.dat"
  $SUDO rm -f "${DEST}"
	fetch "${RELEASE_URL}" "${TEMP_DIR}/geodl" && $SUDO mv "${TEMP_DIR}/geodl" "${DEST}"

  echo "Downloading geosite.dat ..."
  RELEASE_URL="${GITHUB_BASE_URL}/MetaCubeX/meta-rules-dat/releases/download/latest/geosite.dat"
  DEST="${DATA_DIR}/geosite.dat"
  $SUDO rm -f "${DEST}"
	fetch "${RELEASE_URL}" "${TEMP_DIR}/geodl" && $SUDO mv "${TEMP_DIR}/geodl" "${DEST}"
fi
else
  echo "Geo Files: пропущены (маршрутизация через RULE-SET, китайские .dat не нужны; включить — --with-geo)."
fi

echo "Installing Dashboards ..."
if prompt_continue $WITHOUT_DASHBOARD; then
  DASHBOARD_DIR="${DATA_DIR}/dashboard"
  $SUDO mkdir -p "${DASHBOARD_DIR}"
  # Дашборды (веб-панель) — best-effort: если какой-то не скачался (троттл Fastly),
  # НЕ валим всю установку. Загрузка через fetch (зеркало первым).
  set +e
  install_dash() {  # $1=url  $2=папка-внутри-зипа  $3=целевое-имя
    local url="$1" inner="$2" name="$3" tmp="${TEMP_DIR}/${name}.zip"
    echo "Installing ${name}..."
    if fetch "$url" "$tmp" && unzip -oq "$tmp" -d "${TEMP_DIR}"; then
      $SUDO rm -rf "${DASHBOARD_DIR}/${name}"
      $SUDO mv "${TEMP_DIR}/${inner}" "${DASHBOARD_DIR}/${name}"
    else
      echo "  ${name} пропущен (не скачался) — не критично, веб-панель опциональна."
    fi
  }
  install_dash "${GITHUB_BASE_URL}/haishanh/yacd/archive/refs/heads/gh-pages.zip" "yacd-gh-pages" "yacd"
  install_dash "${GITHUB_BASE_URL}/MetaCubeX/metacubexd/archive/refs/heads/gh-pages.zip" "metacubexd-gh-pages" "metacubexd"
  install_dash "${GITHUB_BASE_URL}/Zephyruso/zashboard/releases/latest/download/dist-cdn-fonts.zip" "dist" "zashboard"
  set -e
fi

echo "Installing Desktop App ..."
if prompt_continue $WITHOUT_DESKTOP; then
  DEPLOY="${DESKTOP_SRC}/desktop/deploy-desktop.sh"
  if [ -f "$DEPLOY" ]; then
    VER=$(grep '"version"' "${DESKTOP_SRC}/package.json" 2>/dev/null | head -1 | cut -d'"' -f4)
    GCC_USER="$(id -un)" GCC_PLUGIN_DIR="${DESKTOP_SRC}" GCC_VERSION="${VER}" \
      GCC_WITH_GUI="${GCC_WITH_GUI}" ${DESKTOP_MIHOMO:+GCC_MIHOMO="${DESKTOP_MIHOMO}"} \
      bash "$DEPLOY" || echo "  desktop deploy failed (non-fatal)"
    if [ "$GCC_WITH_GUI" = "1" ]; then
      echo "  Готово: ярлык «Geekcom Clash» на рабочем столе и в меню приложений."
    else
      echo "  Готово: движок развёрнут (без десктоп-GUI)."
    fi
  else
    echo "  (desktop files not in this build, skipping)"
  fi
fi

echo "Installation complete."
echo
echo "Restarting Decky Loader ..."
if prompt_continue $WITHOUT_RESTART; then
  $SUDO systemctl restart plugin_loader.service
fi
