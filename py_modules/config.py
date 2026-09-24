from enum import Enum
import os
from typing import Optional

from dashboard import BUILTIN_DASHBOARDS
from ruamel.yaml import YAML
import decky
from decky import logger
from ruamel.yaml.comments import CommentedMap

OVERRIDE_YAML = os.path.join(decky.DECKY_PLUGIN_DIR, 'override.yaml')

yaml = YAML()
yaml.width = float("inf")
yaml.preserve_quotes = True

class EnhancedMode(Enum):
    RedirHost = 'redir-host'
    FakeIP    = 'fake-ip'

# Режимы маршрутизации mihomo (ключ `mode`). Выбор пользователя хранится в
# настройках и пишется в конфиг при каждой генерации — иначе после выкл/вкл
# режим брался из подписки (обычно `rule`) и сбрасывался.
CLASH_MODES = ('rule', 'global', 'direct')
DEFAULT_MODE = 'rule'


def normalize_mode(mode: Optional[str]) -> str:
    mode = (mode or '').lower()
    return mode if mode in CLASH_MODES else DEFAULT_MODE


async def generate_config(
        ori_path: str,
        new_path: str,
        secret: str,
        override_dns: bool,
        enhanced_mode: EnhancedMode,
        controller_port: int,
        allow_remote_access: bool,
        dashboard_dir: str,
        dashboard: Optional[str],
        skip_steam_download: bool,
        mode: Optional[str] = None,
        ) -> None:
    with open(ori_path) as f:
        config = yaml.load(f)
    logger.debug(f'generate_config: config: {config}')
    with open(OVERRIDE_YAML) as f:
        override_config = yaml.load(f)
    logger.debug(f'generate_config: override_config: {override_config}')
    if override_dns:
        config['dns'] = override_config['dns-override']
        _merge_dict(config['dns'], override_config[f'{enhanced_mode.value}-dns'])
    if skip_steam_download:
        config['rules'] = override_config['skip-steam-rules'] + config['rules']

    # Обязательный роутинг системных сервисов через VPN.
    # Внедряем GEEKCOM-VPN (select, на неё ссылаются правила) + GEEKCOM-AUTO
    # (url-test, пункт «Авто» внутри неё) и префиксуем правила (высший приоритет).
    force_group = override_config['force-proxy-group']
    auto_group = override_config['force-auto-group']
    global_group = override_config['force-global-group']
    _migrate_legacy_sharelink(config, force_group['name'])
    groups = config.get('proxy-groups') or []
    existing = {g.get('name') for g in groups}
    prepend = [g for g in (force_group, auto_group, global_group) if g['name'] not in existing]
    if prepend:
        config['proxy-groups'] = prepend + list(groups)
    config['rules'] = override_config['force-proxy-rules'] + config['rules']

    if mode is not None:
        config['mode'] = normalize_mode(mode)

    config['external-controller'] = f'{"0.0.0.0" if allow_remote_access else "127.0.0.1"}:{controller_port}'
    config['secret'] = secret
    config['external-ui'] = dashboard_dir
    if dashboard is not None:
        config['external-ui-name'] = dashboard
        if dashboard in BUILTIN_DASHBOARDS:
            config['external-ui-url'] = BUILTIN_DASHBOARDS[dashboard]

    config['tun'] = override_config['tun-override']
    _merge_dict(config, override_config['always-override'])
    with open(new_path, 'w') as f:
        yaml.dump(config, f)

def _migrate_legacy_sharelink(config: CommentedMap, force_group_name: str) -> None:
    """Старые подписки из share-ссылок (sharelink.build_yaml до перехода на
    GEEKCOM-VPN) имели свои группы PROXY (select: ноды + DIRECT) и AUTO, а
    правило MATCH,PROXY. Выбор ноды в плагине управляет GEEKCOM-VPN, поэтому в
    режиме «Правила» он игнорировался, а PROXY мог остаться на DIRECT (выбор
    хранится в cache.db по имени группы, общий для всех подписок). Узнаём
    ровно этот сгенерированный нами шаблон и переводим MATCH на GEEKCOM-VPN."""
    groups = config.get('proxy-groups') or []
    if [g.get('name') for g in groups] != ['PROXY', 'AUTO']:
        return
    if [str(r) for r in (config.get('rules') or [])] != ['MATCH,PROXY']:
        return
    names = [p.get('name') for p in (config.get('proxies') or [])]
    if list(groups[0].get('proxies') or []) != names + ['DIRECT']:
        return
    config['proxy-groups'] = []
    config['rules'] = [f'MATCH,{force_group_name}']
    logger.info('generate_config: migrated legacy sharelink config to MATCH,' + force_group_name)


def _merge_dict(a: CommentedMap, b: CommentedMap) -> None:
    for k, v in b.items():
        a[k] = v
