import asyncio
import functools
from http.client import HTTPResponse
import json
import logging
import os
import subprocess
from pathlib import Path
import shutil
from typing import Any, Dict, List, Optional, Tuple
import urllib.request

import config
from core import CoreController
import dashboard
import decky
from decky import logger

from external import ExternalServer
import subscription
import upgrade
from metadata import PACKAGE_NAME
from settings import SettingsManager
import utils

# Имена внедряемых групп (см. override.yaml). GEEKCOM-VPN — select (на неё
# ссылаются правила), GEEKCOM-AUTO — пункт «Авто (быстрейшая)» внутри неё.
FORCE_GROUP = "GEEKCOM-VPN"
AUTO_NODE = "GEEKCOM-AUTO"


def _parse_version(v: str) -> Optional[Tuple[int, ...]]:
    """'v1.2.3' -> (1, 2, 3). None, если распарсить не удалось."""
    try:
        core = v.strip().lstrip("vV").split("-")[0].split("+")[0]
        return tuple(int(x) for x in core.split("."))
    except Exception:
        return None


def _is_newer(latest: str, current: str) -> bool:
    """latest СТРОГО новее current. Если версии парсятся — сравниваем численно
    (не предлагаем даунгрейд); иначе откатываемся на старое поведение (!=)."""
    lp, cp = _parse_version(latest), _parse_version(current)
    if lp is not None and cp is not None:
        return lp > cp
    return latest != current


class Plugin:
    # Asyncio-compatible long-running code, executed in a task when the plugin is loaded
    async def _main(self):
        self.settings = SettingsManager(
            name="config", settings_directory=decky.DECKY_PLUGIN_SETTINGS_DIR
        )
        logger.info(f"starting {PACKAGE_NAME} ...")
        try:
            upgrade.initialize_plugin()
        except Exception as e:
            logger.error(f"initialize_plugin: failed with {e}")
            logger.debug(f"stack trace: {utils.get_traceback(e)}")

        # Миграция осиротевшего вложенного слоя 'settings' (старый формат) в
        # top-level — чтобы плагин и десктопный TUI видели ОДИН список подписок.
        self._migrate_legacy_layer()

        self._set_default("subscriptions", {})
        self._set_default("secret", utils.rand_thing())
        self._set_default("override_dns", True)
        self._set_default("enhanced_mode", config.EnhancedMode.FakeIP.value)
        self._set_default("controller_port", 9090)
        self._set_default("external_port", 50581)
        self._set_default("allow_remote_access", False)
        self._set_default("autostart", False)
        self._set_default("timeout", 15.0)
        self._set_default("user_agent_override", "")
        self._set_default("debounce_time", 10.0)
        self._set_default("disable_verify", False)
        self._set_default("external_run_bg", False)
        self._set_default("auto_check_update", True)
        self._set_default("auto_update_subscription", False)
        self._set_default("skip_steam_download", False)
        self._set_default("current_node", None)  # None = «Авто (быстрейшая)»
        self._set_default("log_level", logging.getLevelName(logging.INFO))

        level = self._get("log_level")
        logger.setLevel(logging.getLevelNamesMapping()[level])
        logger.info(f"log level set to {level}")
        logger.debug(f"os: {os.uname()}")
        logger.debug(f"environments: {os.environ}")
        logger.debug(f"settings: {self.settings.settings}")

        utils.init_ssl_context(self._get("disable_verify"))

        self.core = CoreController()
        self.core.set_exit_callback(lambda x: decky.emit("core_exit", x))

        # Раскатать/обновить десктоп-набор (TUI/ярлык/юнит/setcap/polkit) —
        # покрывает апдейт через страницу плагина. Awaited, чтобы юнит существовал
        # до автозапуска (единая служба).
        try:
            await self._deploy_desktop()
        except Exception as e:
            logger.error(f"deploy_desktop failed: {e}")

        if self._get("autostart"):
            await self.set_core_status(True)

        self.external = ExternalServer()
        from aiohttp import web
        async def _download_callback(request: web.Request) -> web.Response:
            import http
            try:
                link = request.query.get("link")
                if link is None:
                    raise ValueError("missing link query")
                success, error = await self.download_subscription(link)
                if not success:
                    raise RuntimeError(error)
                return web.Response(status=http.HTTPStatus.OK)
            except ValueError as e:
                logger.error(f"external_callback: value error {e}")
                return web.json_response({"error": str(e)}, status=http.HTTPStatus.BAD_REQUEST)
            except RuntimeError as e:
                logger.error(f"external_callback: runtime error {e}")
                return web.json_response({"error": str(e)}, status=http.HTTPStatus.INTERNAL_SERVER_ERROR)
        self.external.register_callback("/download_sub", _download_callback)
        async def _upload_callback(request: web.Request) -> web.Response:
            import http
            try:
                post_data = await request.post()
                file_field = post_data.get("file")
                if file_field is None or not hasattr(file_field, "filename"):
                    raise ValueError("missing file")
                file_name = file_field.filename or ""
                file_bytes = file_field.file.read()
                if not file_bytes:
                    raise ValueError("empty file")
                success, error = await self.import_subscription_file(file_name, file_bytes)
                if not success:
                    raise RuntimeError(error)
                return web.Response(status=http.HTTPStatus.OK)
            except ValueError as e:
                logger.error(f"external_upload_callback: value error {e}")
                return web.json_response({"error": str(e)}, status=http.HTTPStatus.BAD_REQUEST)
            except RuntimeError as e:
                logger.error(f"external_upload_callback: runtime error {e}")
                return web.json_response({"error": str(e)}, status=http.HTTPStatus.INTERNAL_SERVER_ERROR)
        self.external.register_callback("/upload_sub", _upload_callback)
        if self._get("external_run_bg"):
            await self.set_external_status(True)

        if self._get("auto_check_update"):
            await self.check_update()

    # Function called first during the unload process, utilize this to handle your plugin being removed
    async def _unload(self):
        if self.core.is_running:
            await self.core.stop()

    def generate_config(self):
        return config.generate_config(
            subscription.get_path(self._get("current")),
            CoreController.CONFIG_PATH,
            self._get("secret"),
            self._get("override_dns"),
            config.EnhancedMode(self._get("enhanced_mode")),
            self._get("controller_port"),
            self._get("allow_remote_access"),
            str(dashboard.DASHBOARD_DIR),
            self._get("dashboard", True),
            self._get("skip_steam_download"),
        )

    async def get_core_status(self) -> bool:
        is_running = self.core.is_running
        logger.debug(f"get_core_status: {is_running}")
        return is_running

    async def _deploy_desktop(self):
        """Развернуть десктоп-набор для пользователя (плагин работает как root)."""
        try:
            script = os.path.join(decky.DECKY_PLUGIN_DIR, "desktop", "deploy-desktop.sh")
            if not os.path.exists(script):
                return
            import pwd
            user = os.environ.get("DECKY_USER", "deck")
            try:
                home = pwd.getpwnam(user).pw_dir
            except KeyError:
                home = f"/home/{user}"
            # Версию берём из package.json (её бампит CI), а НЕ из
            # DECKY_PLUGIN_VERSION: после оффлайн-mv Decky может держать в реестре
            # старую версию → стамп ложно совпадёт и автодеплой пропустится.
            try:
                with open(os.path.join(decky.DECKY_PLUGIN_DIR, "package.json")) as pf:
                    version = str(json.load(pf).get("version") or decky.DECKY_PLUGIN_VERSION)
            except Exception:
                version = str(decky.DECKY_PLUGIN_VERSION)
            stamp = os.path.join(home, ".local", "share", "geekcom-clash", ".deployed-version")
            try:
                with open(stamp) as f:
                    if f.read().strip() == version:
                        return  # уже развёрнуто этой версии
            except Exception:
                pass
            logger.info(f"deploy-desktop: deploying for {user} ({version})")
            # ВАЖНО: asyncio-спавн в decky-рантайме ненадёжен (логируется, но не
            # выполняется), поэтому запускаем blocking subprocess.run в треде через
            # run_in_executor. bash зовём напрямую (deploy-desktop.sh отрабатывает
            # от root и с минимальным окружением), но PATH задаём явно.
            # Decky инжектит свой LD_LIBRARY_PATH (бандленная libreadline и пр.) →
            # системный bash падает с symbol lookup error. Чистим его для дочерних.
            env = dict(os.environ)
            env.pop("LD_LIBRARY_PATH", None)
            env.pop("LD_PRELOAD", None)
            env["PATH"] = "/usr/sbin:/usr/bin:/sbin:/bin"
            env["GCC_USER"] = user
            env["GCC_PLUGIN_DIR"] = decky.DECKY_PLUGIN_DIR
            env["GCC_VERSION"] = version
            loop = asyncio.get_event_loop()
            r = await loop.run_in_executor(None, functools.partial(
                subprocess.run, ["/usr/bin/bash", script], env=env,
                capture_output=True, text=True, timeout=90))
            logger.info(f"deploy-desktop: rc={r.returncode}")
            if r.returncode != 0:
                logger.error(f"deploy-desktop OUT: {(r.stdout or '')[-400:]}")
                logger.error(f"deploy-desktop ERR: {(r.stderr or '')[-400:]}")
        except Exception as e:
            logger.error(f"deploy_desktop failed: {e}")

    async def set_core_status(self, status: bool) -> Tuple[bool, Optional[str]]:
        gobin = self._engine_bin()
        try:
            if status:
                # NIGHTLY: запуск через Go-движок (ensureCaps + systemctl start;
                # конфиг генерит ExecStartPre юнита = ctl regen → тоже Go).
                # Заодно чиним дыру: cap-самохил после апдейта mihomo на пути
                # плагина (раньше был только в TUI).
                if gobin:
                    await self._run_engine(gobin, "start")
                else:
                    await self.core.start()
                await self._apply_node_selection()
            else:
                if gobin:
                    await self._run_engine(gobin, "stop")
                else:
                    await self.core.stop()
        except Exception as e:
            logger.error(f"set_core_status: failed with {e}")
            logger.debug(f"stack trace: {utils.get_traceback(e)}")
            return False, str(e)
        return True, None

    async def restart_core(self) -> None:
        # Перезапуск единой службы: ExecStartPre пере-генерит конфиг по текущим
        # настройкам (плагин и TUI используют один geekcom-clash.service).
        logger.debug("restarting core (unit) ...")
        await self.core.restart()
        await self._apply_node_selection()

    # --- Выбор ноды (группа GEEKCOM-VPN) ------------------------------------
    def _controller_request(self, method: str, path: str, body: Optional[dict] = None) -> dict:
        """Запрос к external-controller mihomo (localhost). Блокирующий —
        вызывать через run_in_executor из async-кода."""
        port = self._get("controller_port")
        secret = self._get("secret")
        data = json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(
            f"http://127.0.0.1:{port}{path}",
            data=data,
            method=method,
            headers={"Authorization": f"Bearer {secret}", "Content-Type": "application/json"},
        )
        with urllib.request.urlopen(req, timeout=3) as resp:
            raw = resp.read()
            return json.loads(raw) if raw else {}

    async def get_nodes(self) -> Dict[str, Any]:
        """Ноды группы GEEKCOM-VPN + текущий выбор. Работает и до включения VPN
        (имена нод из yaml текущей подписки), и после (живой API с пингами)."""
        saved = self._get("current_node", True) or AUTO_NODE
        if self.core.is_running:
            try:
                loop = asyncio.get_event_loop()
                g = await loop.run_in_executor(
                    None, lambda: self._controller_request("GET", f"/proxies/{FORCE_GROUP}"))
                return {"members": g.get("all", []), "current": g.get("now", saved), "running": True}
            except Exception as e:
                logger.debug(f"get_nodes: api failed {e}")
        members = [AUTO_NODE]
        current_sub = self._get("current", True)
        if current_sub:
            try:
                from ruamel.yaml import YAML
                with open(subscription.get_path(current_sub)) as f:
                    doc = YAML().load(f)
                members += [p.get("name") for p in (doc.get("proxies") or []) if p.get("name")]
            except Exception as e:
                logger.debug(f"get_nodes: parse failed {e}")
        return {"members": members, "current": saved, "running": False}

    async def set_node(self, name: str) -> bool:
        """Зафиксировать ноду: сохранить в настройках (предвыбор) и, если ядро
        запущено, применить через API сразу."""
        self.settings.setSetting("current_node", name)
        if self.core.is_running:
            try:
                loop = asyncio.get_event_loop()
                await loop.run_in_executor(
                    None, lambda: self._controller_request("PUT", f"/proxies/{FORCE_GROUP}", {"name": name}))
            except Exception as e:
                logger.error(f"set_node: api failed {e}")
                return False
        return True

    async def _apply_node_selection(self) -> None:
        """Применить сохранённый выбор ноды после старта (предвыбор «до
        включения»). Ждём готовности контроллера несколько секунд."""
        node = self._get("current_node", True)
        if not node:
            return
        loop = asyncio.get_event_loop()
        for _ in range(15):
            try:
                await loop.run_in_executor(
                    None, lambda: self._controller_request("PUT", f"/proxies/{FORCE_GROUP}", {"name": node}))
                logger.info(f"applied saved node selection: {node}")
                return
            except Exception:
                await asyncio.sleep(0.4)
        logger.warning(f"_apply_node_selection: controller not ready, skipped {node}")

    def _engine_bin(self) -> Optional[str]:
        """Путь к Go-движку (geekcom-clash). None, если не задеплоен."""
        import pwd
        user = os.environ.get("DECKY_USER", "deck")
        try:
            home = pwd.getpwnam(user).pw_dir
        except KeyError:
            home = f"/home/{user}"
        gobin = os.path.join(home, ".local", "share", "geekcom-clash", "geekcom-clash")
        return gobin if os.path.exists(gobin) else None

    def _engine_argv(self, gobin: str, *args: str) -> list:
        """Команда запуска движка ОТ ИМЕНИ дек-юзера (плагин — root). Через
        runuser: тогда systemctl --user работает, файлы создаются дек-овнед,
        а setcap-wrapper берёт NOPASSWD дека. env-переменные движка передаём
        внутрь (runuser сбрасывает окружение)."""
        user = os.environ.get("DECKY_USER", "deck")
        pairs = [
            f"GEEKCOM_CLASH_DIR={decky.DECKY_PLUGIN_SETTINGS_DIR}",
            f"GEEKCOM_CLASH_MIHOMO={os.path.join(decky.DECKY_PLUGIN_DIR, 'bin', 'mihomo')}",
            f"GEEKCOM_CLASH_RESOURCE_DIR={decky.DECKY_PLUGIN_RUNTIME_DIR}",
        ]
        return ["runuser", "-u", user, "--", "env", *pairs, gobin, *args]

    def _engine_base_env(self) -> dict:
        """Чистое окружение для subprocess: decky инжектит LD_* (ломает bash/
        runuser), убираем."""
        env = dict(os.environ)
        env.pop("LD_LIBRARY_PATH", None)
        env.pop("LD_PRELOAD", None)
        env["PATH"] = "/usr/sbin:/usr/bin:/sbin:/bin"
        return env

    async def _run_engine(self, gobin: str, *args: str) -> subprocess.CompletedProcess:
        """Запустить движок (как дек-юзер); бросить при ненулевом коде."""
        loop = asyncio.get_event_loop()
        r = await loop.run_in_executor(None, functools.partial(
            subprocess.run, self._engine_argv(gobin, *args), env=self._engine_base_env(),
            capture_output=True, text=True, timeout=60))
        if r.returncode != 0:
            raise RuntimeError((r.stderr or r.stdout or "").strip() or f"engine {args[0]} rc={r.returncode}")
        return r

    async def get_engine_version(self) -> str:
        """Версия Go-движка. Пусто, если бинаря нет (релиз без движка) — тогда
        строка в About не показывается."""
        gobin = self._engine_bin()
        if not gobin:
            return ""
        try:
            r = await self._run_engine(gobin, "version")
            return r.stdout.strip()
        except Exception as e:
            logger.error(f"get_engine_version: {e}")
            return ""

    async def kill_core(self) -> bool:
        return CoreController.kill(self._get("timeout"))

    async def get_config(self) -> dict:
        config = {
            "status": self.core.is_running,
            "current": self._get("current", True),
            "secret": self._get("secret"),
            "override_dns": self._get("override_dns"),
            "enhanced_mode": self._get("enhanced_mode"),
            "allow_remote_access": self._get("allow_remote_access"),
            "autostart": self._get("autostart"),
            "dashboard": self._get("dashboard", True),
            "controller_port": self._get("controller_port"),
            "skip_steam_download": self._get("skip_steam_download"),
        }
        logger.debug(config)
        return config

    async def get_config_value(self, key: str):
        value = self.settings.getSetting(key)
        logger.debug(f"get_config_value: {key} => {value}")
        return value

    async def set_config_value(self, key: str, value: Any):
        PERMITTED_KEYS = [
            "override_dns",
            "enhanced_mode",
            "allow_remote_access",
            "autostart",
            "dashboard",
            "external_run_bg",
            "auto_check_update",
            "auto_update_subscription",
            "skip_steam_download",
        ]
        if key not in PERMITTED_KEYS:
            logger.error(f"set_config_value: not permitted key {key}")
            return
        self.settings.setSetting(key, value)
        logger.debug(f"set_config_value: {key} => {value}")

    async def upgrade(self, res: str, version: str) -> Tuple[bool, Optional[str]]:
        if res not in upgrade.RESOURCE_TYPE_VALUES:
            logger.error(f"upgrade: invalid resource {res}")
            return False, "invalid resource"
        res_type = upgrade.ResourceType(res)
        try:
            await upgrade.upgrade(res_type, version)
        except Exception as e:
            logger.error(f"upgrade: failed with {e}")
            logger.debug(f"stack trace: {utils.get_traceback(e)}")
            return False, str(e)
        return True, None

    async def cancel_upgrade(self, res: str) -> None:
        if res not in upgrade.RESOURCE_TYPE_VALUES:
            logger.error(f"cancel_upgrade: invalid resource {res}")
            return
        res_type = upgrade.ResourceType(res)
        upgrade.cancel_upgrade(res_type)

    async def get_version(self, res: str) -> str:
        if res not in upgrade.RESOURCE_TYPE_VALUES:
            logger.error(f"get_version: invalid resource {res}")
            return ""
        res_type = upgrade.ResourceType(res)
        try:
            match res_type:
                case upgrade.ResourceType.PLUGIN:
                    version = decky.DECKY_PLUGIN_VERSION
                    if version[0].isdigit():
                        version = "v" + version
                case upgrade.ResourceType.CORE:
                    version = CoreController.get_version()
        except Exception as e:
            logger.error(f"get_version: {res} failed with {type(e)} {e}")
            logger.debug(f"stack trace: {utils.get_traceback(e)}")
            return ""
        logger.debug(f"get_version: {res} {version}")
        return version

    async def get_latest_version(self, res: str) -> str:
        if res not in upgrade.RESOURCE_TYPE_VALUES:
            logger.error(f"get_latest_version: invalid resource {res}")
            return ""
        res_type = upgrade.ResourceType(res)
        try:
            version = await upgrade.get_latest_version(res_type, self._get("timeout"), self._get("debounce_time"))
        except Exception as e:
            logger.error(f"get_latest_version: failed with {e}")
            logger.debug(f"stack trace: {utils.get_traceback(e)}")
            return ""
        logger.debug(f"get_latest_version: {res} {version}")
        return version

    async def is_upgrading(self, res: str) -> bool:
        if res not in upgrade.RESOURCE_TYPE_VALUES:
            logger.error(f"is_upgrading: invalid resource {res}")
            return False
        res_type = upgrade.ResourceType(res)
        rtn = upgrade.is_upgrading(res_type)
        logger.debug(f"is_upgrading: {res} {rtn}")
        return rtn

    async def get_dashboard_list(self) -> List[str]:
        dashboard_list = dashboard.get_dashboard_list()
        logger.debug(f"get_dashboard_list: {dashboard_list}")
        return dashboard_list

    async def get_subscription_list(self) -> Dict[str, str]:
        # Перечитываем с диска: подписку могли добавить из десктопного TUI.
        self._reload_settings()
        subs: subscription.SubscriptionDict = self.settings.getSetting("subscriptions")
        logger.debug(f"get_subscription_list: {subs}")
        return subs

    async def update_subscription(self, name: str) -> Tuple[bool, Optional[str]]:
        logger.info(f"update_subscription: updating {name}")
        subs: subscription.SubscriptionDict = self.settings.getSetting("subscriptions")
        if name not in subs:
            logger.error(f"update_subscription: {name} not found")
            return False, "subscription not found"
        if subs[name].startswith("local://"):
            logger.error(f"update_subscription: {name} is local subscription")
            return False, "local subscription"
        result = await subscription.update_sub(
            name,
            subs[name],
            self._get("timeout"),
            self._get("user_agent_override"),
        )
        if result is None:
            if self.core.is_running and name == self._get("current"):
                await self.restart_core()
            return True, None
        else:
            return False, result

    async def update_all_subscriptions(self) -> None:
        subs: subscription.SubscriptionDict = self.settings.getSetting("subscriptions")
        current = self._get("current", True)
        remote_subs = [(name, url) for name, url in subs.items() if not url.startswith("local://")]
        if len(remote_subs) == 0:
            return

        results = await asyncio.gather(*[
            subscription.update_sub(
                name,
                url,
                self._get("timeout"),
                self._get("user_agent_override"),
            )
            for name, url in remote_subs
        ])

        current_updated = False
        for (name, _), error in zip(remote_subs, results):
            if error is not None:
                logger.error(f"update_all_subscriptions: failed to update {name}: {error}")
                continue
            if name == current:
                current_updated = True

        if self.core.is_running and current_updated:
            await self.restart_core()

    async def duplicate_subscription(self, name: str) -> None:
        subs: subscription.SubscriptionDict = self.settings.getSetting("subscriptions")
        new_name = subscription.duplicate_sub(subs, name)
        if new_name is not None:
            logger.info(f"duplicated subscription: {name} to {new_name}")
            subs[new_name] = subs[name]
            self.settings.setSetting("subscriptions", subs)

    async def edit_subscription(self, name: str, new_name: str, new_url: str) -> None:
        subs: subscription.SubscriptionDict = self.settings.getSetting("subscriptions")
        new_name = utils.sanitize_filename(new_name)
        logger.info(f"edit_subscription: {name} => {new_name}, {new_url}")
        if name in subs:
            if new_name == name:
                subs[name] = new_url
            elif new_name in subs:
                logger.error(f"edit_subscription: duplicated name {new_name}")
                return
            else:
                try:
                    new_path = subscription.get_path(new_name)
                    if os.path.exists(new_path):
                        os.remove(new_path)
                    shutil.move(subscription.get_path(name), new_path)
                except Exception as e:
                    logger.error(f"edit_subscription: move error {e}")
                    logger.debug(f"stack trace: {utils.get_traceback(e)}")
                    return
                subs.pop(name)
                subs[new_name] = new_url
            self.settings.setSetting("subscriptions", subs)
        else:
            logger.error(f"edit_subscription: {name} not found")

    async def download_subscription(self, url: str) -> Tuple[bool, Optional[str]]:
        self._reload_settings()
        gobin = self._engine_bin()
        if gobin:
            # NIGHTLY: добавление подписки делает Go-движок (парс/скачивание/
            # валидация/сохранение/регистрация в config.json).
            try:
                r = await self._run_engine(gobin, "add-sub", url)
                data = json.loads((r.stdout or "").strip().splitlines()[-1])
            except Exception as e:
                logger.error(f"download_subscription (engine): {e}")
                return False, str(e)
            self._reload_settings()  # Go сам записал подписку в config.json
            if data.get("ok"):
                name = data["result"][0]
                await decky.emit("sub_update", name)
                return True, None
            return False, str(data.get("result"))

        # fallback: Python-движок (релиз без Go-бинаря)
        subs: subscription.SubscriptionDict = self.settings.getSetting("subscriptions")
        ok, data = subscription.download_sub(
            url,
            subs,
            self._get("timeout"),
            self._get("user_agent_override"),
        )
        if ok:
            name, url = data
            subs[name] = url
            self.settings.setSetting("subscriptions", subs)
            if self.settings.getSetting("current") is None:
                self.settings.setSetting("current", name)
            await decky.emit("sub_update", name)
            return True, None
        else:
            return False, data # type: ignore

    async def import_subscription_file(self, file_name: str, file_bytes: bytes) -> Tuple[bool, Optional[str]]:
        self._reload_settings()
        subs: subscription.SubscriptionDict = self.settings.getSetting("subscriptions")
        ok, data = subscription.import_sub(file_name, file_bytes, subs)
        if ok:
            name, url = data
            subs[name] = url
            self.settings.setSetting("subscriptions", subs)
            if self.settings.getSetting("current") is None:
                self.settings.setSetting("current", name)
            await decky.emit("sub_update", name)
            return True, None
        else:
            return False, data # type: ignore

    async def remove_subscription(self, name: str) -> None:
        logger.info(f"removing subscription: {name}")
        self._reload_settings()
        subs: subscription.SubscriptionDict = self.settings.getSetting("subscriptions")
        if name in subs:
            subs.pop(name)
            try:
                os.remove(subscription.get_path(name))
            except Exception as e:
                logger.error(f"remove_subscription: {e}")
                logger.debug(f"stack trace: {utils.get_traceback(e)}")
            if self.settings.getSetting("current") == name:
                self.settings.setSetting("current", None)
            self.settings.setSetting("subscriptions", subs)

    async def set_current(self, name: str) -> bool:
        logger.debug(f"setting current to: {name}")
        self._reload_settings()
        if name in self.settings.getSetting("subscriptions"):
            self.settings.setSetting("current", name)
            return True
        else:
            return False

    async def reorder_subscriptions(self, names: List[str]) -> None:
        self._reload_settings()
        subs: subscription.SubscriptionDict = self.settings.getSetting("subscriptions")
        logger.debug(f"reorder_subscriptions: current: {subs.keys()} target: {names}")
        if set(names) != set(subs.keys()):
            logger.error(f"reorder_subscriptions: unmatched target")
            return
        self.settings.setSetting("subscriptions", {k: subs[k] for k in names})

    async def get_ip(self) -> str:
        return utils.get_ip()

    async def install_geos(self) -> Tuple[bool, str]:
        try:
            await upgrade.download_geos()
        except Exception as e:
            logger.debug(f"stack trace: {utils.get_traceback(e)}")
            return False, str(e)
        return True, ""

    async def install_dashboards(self) -> Tuple[bool, str]:
        try:
            await upgrade.download_dashboards()
        except Exception as e:
            logger.debug(f"stack trace: {utils.get_traceback(e)}")
            return False, str(e)
        return True, ""

    async def set_external_status(self, status: bool) -> None:
        if status:
            await self.external.run(self._get("external_port"))
        else:
            await self.external.stop()

    async def check_update(self) -> None:
        name_map = {
            upgrade.ResourceType.PLUGIN: "GeekcomClash",
            upgrade.ResourceType.CORE: "Mihomo",
        }
        for res in upgrade.RESOURCE_TYPE_ENUMS:
            current = await self.get_version(res.value)
            latest = await self.get_latest_version(res.value)
            if current.startswith("v") and latest.startswith("v") and _is_newer(latest, current):
                logger.info(f"check_update: {res} {current} => {latest}")
                await decky.emit("upgrade_notice", f"{name_map[res]}: {current} => {latest}")

    def _get(self, key: str, allow_none: bool = False) -> Any:
        if allow_none:
            return self.settings.getSetting(key)
        else:
            value = self.settings.getSetting(key)
            if value is None:
                raise ValueError(f'Value of "{key}" is None')
            return value

    def _set_default(self, key: str, value: Any) -> None:
        if self.settings.getSetting(key) is None:
            self.settings.setSetting(key, value)

    def _migrate_legacy_layer(self) -> None:
        """Старый upstream-формат хранил настройки во вложенном ключе 'settings';
        текущий плагин (Decky SettingsManager) работает плоско (top-level), из-за
        чего подписки из старого слоя осиротели и расходились с десктопным TUI.
        Переносим вложенный слой наверх (top-level в приоритете) и удаляем его."""
        nested = self.settings.getSetting("settings")
        if not isinstance(nested, dict):
            return
        nsubs = nested.get("subscriptions") or {}
        merged = {**nsubs, **(self.settings.getSetting("subscriptions") or {})}
        for key in ("current", "dashboard"):
            if not self.settings.getSetting(key) and nested.get(key):
                self.settings.setSetting(key, nested[key])
        # удаляем осиротевший блок и форсим запись через setSetting подписок
        self.settings.settings.pop("settings", None)
        self.settings.setSetting("subscriptions", merged)
        logger.info("migrated legacy nested 'settings' layer to top-level")

    def _reload_settings(self) -> None:
        """Перечитать config.json с диска — десктопный TUI/ctl пишет тот же файл
        в другом процессе, и без перечитывания плагин не увидит его изменений
        (а на следующем commit ещё и затрёт их). Decky кладёт всё в .settings."""
        path = os.path.join(decky.DECKY_PLUGIN_SETTINGS_DIR, "config.json")
        try:
            with open(path) as f:
                self.settings.settings = json.load(f)
        except Exception as e:
            logger.debug(f"_reload_settings: {e}")
