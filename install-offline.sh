#!/bin/bash

set -e

PACKAGE="GeekcomClash"
BASE_DIR=${OVERRIDE_BASE_DIR:-"${HOME}/homebrew"}
PLUGIN_DIR="${BASE_DIR}/plugins"
DATA_DIR="${BASE_DIR}/data"

# Разбор флагов: -y/--yes — неинтерактивный режим (авто-подтверждение).
YES_ALL=false
while [[ $# -gt 0 ]]; do
  case $1 in
    -y|--yes) YES_ALL=true; shift ;;
    *) shift ;;
  esac
done

# Определяем prompt_continue ДО первого использования (root-ветка ниже её зовёт).
function prompt_continue() {
  if [ "$YES_ALL" = "true" ]; then
    return 0
  fi
  local response
  read -p "Do you want to continue? [Y/n] " response
  response=${response,,}
  if [[ -z "$response" || "$response" == "y" || "$response" == "yes" ]]; then
    return 0
  elif [[ "$response" == "n" || "$response" == "no" ]]; then
    return 1
  else
    echo "Invalid response. Not continuing."
    return 1
  fi
}

if [ "$UID" -eq 0 ]; then
  echo "WARNING: Running as root."
  echo "This may cause permission issues."
  echo "If you insist to continue, please confirm homebrew path below is correct:"
  echo "${BASE_DIR}"
  echo "In most circumstances, this should NOT be: /root/homebrew"
  echo "You SHOULD run sudo with -E flag to preserve environment variables, or use OVERRIDE_BASE_DIR to override it."
  echo
  if ! prompt_continue; then
    exit 1
  fi
fi

if [ ! -d "$BASE_DIR" ]; then
  echo "Directory ${BASE_DIR} does not exist."
  echo "Have you installed Decky Loader?"
  echo
  echo "Execute the following command to install Decky Loader:"
  echo "curl -L https://github.com/SteamDeckHomebrew/decky-installer/releases/latest/download/install_release.sh | sh"
  exit 1
fi

function mv_impl() {
  local src="$1"
  local dest="$2"
  local name"=$3"
  local full_dest="$2/$3"
  if [ -d "$full_dest" ]; then
    rm -rf "$full_dest" 2>/dev/null || sudo rm -rf "$full_dest"
  fi
  if [ ! -d "$dest" ]; then
    mkdir -p "$dest" 2>/dev/null || sudo mkdir -p "$dest"
  fi
  if ! mv "$src" "$full_dest" 2>/dev/null; then
    sudo mv "$src" "$full_dest"
    sudo chown -R "$(id -u):$(id -g)" "$full_dest"
  fi
}

REQUIREMENTS=("unzip")
for req in "${REQUIREMENTS[@]}"; do
  if ! command -v $req &> /dev/null; then
    echo "Error: $req is not installed"
    exit 1
  fi
done

TEMP_DIR=$(mktemp -d)
function finish() {
  rm -rf "$TEMP_DIR"
}
trap finish EXIT

echo "Unarchiving ..."
ZIPFILE="${TEMP_DIR}/archive.zip"
PAYLOAD_LINE=$(awk '/^__ZIPFILE_BELOW__/ { print NR + 1; exit 0; }' "$0")
tail -n +${PAYLOAD_LINE} "$0" > "${ZIPFILE}"
unzip -oq "${ZIPFILE}" -d "${TEMP_DIR}"

AUTHOR=$(cat "${TEMP_DIR}/${PACKAGE}/plugin.json" | grep "author" | cut -d '"' -f 4)
VERSION=$(cat "${TEMP_DIR}/${PACKAGE}/package.json" | grep "version" | cut -d '"' -f 4)
LICENSE=$(cat "${TEMP_DIR}/${PACKAGE}/package.json" | grep "license" | cut -d '"' -f 4)

echo "Installing ${PACKAGE} v${VERSION} by ${AUTHOR} ..."
echo "License: ${LICENSE}"
echo
echo "By confirming installation, you agree to the terms of the software license."
if ! prompt_continue; then
  exit 0
fi

mv_impl "${TEMP_DIR}/${PACKAGE}/data" "${DATA_DIR}" "${PACKAGE}"
mv_impl "${TEMP_DIR}/${PACKAGE}" "${PLUGIN_DIR}" "${PACKAGE}"

echo
echo "Installation complete!"
echo
echo "Restarting Decky Loader ..."
if prompt_continue; then
  sudo systemctl restart plugin_loader.service
fi

exit 0

__ZIPFILE_BELOW__
