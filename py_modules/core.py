import asyncio
import functools
import os
import pwd
import subprocess
from typing import Awaitable, Callable, Optional, List

import decky
from decky import logger

LAST_CORE_VERSION = "1.19.25"

ExitCallback = Callable[[Optional[int]], Awaitable[None]]

# Единая служба для плагина (игровой режим) и desktop-TUI: один mihomo под
# systemd --user юнитом geekcom-clash.service. Плагин работает как root, юнит
# принадлежит пользователю (DECKY_USER) → управляем через runuser + чистый env
# (Decky инжектит LD_LIBRARY_PATH, ломающий системные бинари).
UNIT = "geekcom-clash.service"


def _target_user() -> str:
    return os.environ.get("DECKY_USER", "deck")


def _clean_env(user: str) -> dict:
    env = {k: v for k, v in os.environ.items()
           if k not in ("LD_LIBRARY_PATH", "LD_PRELOAD")}
    env["PATH"] = "/usr/sbin:/usr/bin:/sbin:/bin"
    try:
        uid = pwd.getpwnam(user).pw_uid
    except KeyError:
        uid = 1000
    env["XDG_RUNTIME_DIR"] = f"/run/user/{uid}"
    return env


def _systemctl_user(*args: str, timeout: Optional[float] = 30) -> subprocess.CompletedProcess:
    """systemctl --user <args> для целевого пользователя."""
    user = _target_user()
    env = _clean_env(user)
    if os.geteuid() == 0:
        cmd = ["runuser", "-u", user, "--", "systemctl", "--user", *args]
    else:
        cmd = ["systemctl", "--user", *args]
    return subprocess.run(cmd, env=env, capture_output=True, text=True, timeout=timeout)


class CoreController:
    BIN_NAME = "mihomo"
    CORE_PATH = os.path.join(decky.DECKY_PLUGIN_DIR, "bin", BIN_NAME)
    CONFIG_PATH = os.path.join(decky.DECKY_PLUGIN_RUNTIME_DIR, "running_config.yaml")
    RESOURCE_DIR = decky.DECKY_PLUGIN_RUNTIME_DIR

    def __init__(self):
        self._exit_callback: Optional[ExitCallback] = None

    @property
    def is_running(self) -> bool:
        try:
            r = _systemctl_user("is-active", UNIT, timeout=10)
            return r.stdout.strip() == "active"
        except Exception as e:
            logger.error(f"is_running: {e}")
            return False

    async def _ctl(self, *args: str) -> subprocess.CompletedProcess:
        loop = asyncio.get_event_loop()
        return await loop.run_in_executor(None, functools.partial(_systemctl_user, *args))

    async def start(self) -> None:
        # Конфиг генерит ExecStartPre юнита (ctl regen) от имени пользователя.
        r = await self._ctl("start", UNIT)
        if r.returncode != 0:
            msg = (r.stderr or "").strip() or f"systemctl rc={r.returncode}"
            logger.error(f"core start failed: {msg}")
            raise RuntimeError(msg)
        logger.info("core started via geekcom-clash.service")

    async def stop(self) -> None:
        r = await self._ctl("stop", UNIT)
        if r.returncode != 0:
            logger.warning(f"core stop rc={r.returncode}: {(r.stderr or '').strip()}")

    async def restart(self) -> None:
        # restart перечитывает ExecStartPre (ctl regen) → свежий конфиг.
        r = await self._ctl("restart", UNIT)
        if r.returncode != 0:
            logger.error(f"core restart failed: {(r.stderr or '').strip()}")

    def set_exit_callback(self, callback: Optional[ExitCallback]):
        # systemd сам рестартит ядро (Restart=on-failure); отдельный монитор не нужен.
        self._exit_callback = callback

    @classmethod
    def _gen_cmd(cls, config_path: str) -> List[str]:
        return [cls.CORE_PATH, "-f", config_path, "-d", cls.RESOURCE_DIR]

    @classmethod
    def check_config(cls, config_path: str) -> bool:
        command = cls._gen_cmd(config_path)
        command.append("-t")
        logger.debug(f"check_config: {' '.join(command)}")
        try:
            return_code = subprocess.call(command)
        except Exception as e:
            logger.error(f"check_config: failed with {e}")
            raise
        logger.debug(f"check_config: return code: {return_code}")
        return return_code == 0

    @classmethod
    def get_version(cls) -> str:
        try:
            output = subprocess.check_output([cls.CORE_PATH, "-v"])
        except Exception as e:
            logger.error(f"get_version: failed with {str(e)}")
            return ""
        for s in output.decode().split(" "):
            if s.startswith("v"):
                global LAST_CORE_VERSION
                LAST_CORE_VERSION = s.strip()
                return LAST_CORE_VERSION
        return ""

    @classmethod
    def kill(cls, timeout: Optional[float] = None) -> bool:
        # Сначала останавливаем юнит, затем добиваем процесс на всякий случай.
        try:
            _systemctl_user("stop", UNIT, timeout=timeout or 15)
        except Exception as e:
            logger.warning(f"kill: unit stop failed: {e}")
        try:
            subprocess.run(["pkill", "-KILL", "-x", cls.BIN_NAME],
                           capture_output=True, text=True, timeout=timeout)
        except Exception as e:
            logger.error(f"kill core: failed with {e}")
            return False
        return True
