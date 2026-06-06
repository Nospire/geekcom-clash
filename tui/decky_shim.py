"""
Минимальный shim модуля `decky` для запуска кода плагина (config.py,
subscription.py, dashboard.py) ВНЕ среды Decky Loader — из desktop-CLI.

Плагин-код делает `import decky` и читает оттуда пути/логгер. В игровом
режиме их подставляет Decky Loader; на рабочем столе подставляем мы — те же
самые пути (~/homebrew/...), поэтому ctl и плагин работают с одними файлами.

ctl ставит этот модуль в sys.modules как "decky" ДО импорта py_modules.
"""
import logging
import os

_HB = os.path.expanduser("~/homebrew")
_PKG = "GeekcomClash"


def _p(env, *parts):
    return os.environ.get(env) or os.path.join(_HB, *parts)


DECKY_PLUGIN_DIR = _p("DECKY_PLUGIN_DIR", "plugins", _PKG)
DECKY_PLUGIN_RUNTIME_DIR = _p("DECKY_PLUGIN_RUNTIME_DIR", "data", _PKG)
DECKY_PLUGIN_SETTINGS_DIR = _p("DECKY_PLUGIN_SETTINGS_DIR", "settings", _PKG)
DECKY_PLUGIN_LOG_DIR = _p("DECKY_PLUGIN_LOG_DIR", "logs", _PKG)
DECKY_PLUGIN_VERSION = os.environ.get("DECKY_PLUGIN_VERSION", "tui")
DECKY_USER = os.environ.get("DECKY_USER", os.environ.get("USER", "deck"))

logger = logging.getLogger("geekcom-clash-ctl")
if not logger.handlers:
    logging.basicConfig(level=logging.INFO, format="%(levelname)s: %(message)s")


def emit(*_args, **_kwargs):
    # В CLI событий фронту нет — глушим.
    return None
