"""
Парсер share-ссылок (vless:// vmess:// ss:// trojan:// hysteria2://) и
base64-подписок v2ray-формата → готовый Clash/Mihomo YAML-конфиг.

Зачем: поле импорта раньше делало urllib.urlopen(url) и падало на
`unknown url type: vless`, потому что vless:// — это не подписка (HTTP-адрес
со списком нод), а одна нода, закодированная в строку. Здесь мы парсим такие
строки (и base64-списки из них) сами и собираем валидный конфиг.
"""

import base64
import binascii
import json
import io
import urllib.parse
from typing import List, Optional, Tuple

from ruamel.yaml import YAML

SCHEMES = ("vless://", "vmess://", "ss://", "trojan://",
           "hysteria2://", "hy2://")

_yaml = YAML()
_yaml.width = float("inf")
_yaml.default_flow_style = False


# ──────────────────────────── helpers ────────────────────────────

def _b64decode(s: str) -> Optional[str]:
    """Декод base64 (обычный и url-safe), терпимо к отсутствию паддинга."""
    s = s.strip()
    if not s:
        return None
    for fn in (base64.b64decode, base64.urlsafe_b64decode):
        try:
            pad = "=" * (-len(s) % 4)
            return fn(s + pad).decode("utf-8", "replace")
        except (binascii.Error, ValueError):
            continue
    return None


def _name_from_fragment(url: str, fallback: str) -> str:
    frag = urllib.parse.urlparse(url).fragment
    name = urllib.parse.unquote(frag).strip() if frag else ""
    return name or fallback


def _qs(url: str) -> dict:
    """query-параметры share-ссылки в плоский dict (последнее значение)."""
    q = urllib.parse.urlparse(url).query
    out = {}
    for k, v in urllib.parse.parse_qsl(q, keep_blank_values=True):
        out[k] = v
    return out


def _truthy(v) -> bool:
    return str(v).lower() in ("1", "true", "yes")


def _split_lines(text: str) -> List[str]:
    """Разбить на строки share-ссылки (одна или несколько / base64-список)."""
    text = text.strip()
    # base64-подписка целиком? — декодируем и работаем с результатом
    if not any(text.lower().startswith(s) for s in SCHEMES):
        dec = _b64decode(text)
        if dec and any(s in dec for s in SCHEMES):
            text = dec
    lines = []
    for raw in text.replace("\r", "\n").split("\n"):
        ln = raw.strip()
        if ln and any(ln.lower().startswith(s) for s in SCHEMES):
            lines.append(ln)
    return lines


def looks_like_sharelink(text: str) -> bool:
    """Похоже ли это на share-ссылку / base64-список таких ссылок."""
    return bool(_split_lines(text))


# ──────────────────────────── parsers ────────────────────────────

def _net_opts(proxy: dict, params: dict, network: str) -> None:
    """Заполнить ws/grpc/http транспорт по query-параметрам."""
    host = params.get("host", "")
    path = params.get("path", "")
    if network in ("ws", "httpupgrade"):
        ws = {}
        if path:
            ws["path"] = path
        if host:
            ws["headers"] = {"Host": host}
        if ws:
            proxy["ws-opts"] = ws
    elif network == "grpc":
        svc = params.get("serviceName") or params.get("servicename") or path
        if svc:
            proxy["grpc-opts"] = {"grpc-service-name": svc}
    elif network in ("http", "h2"):
        h2 = {}
        if path:
            h2["path"] = path
        if host:
            h2["host"] = [h for h in host.split(",") if h]
        if h2:
            proxy["h2-opts"] = h2


def _tls_opts(proxy: dict, params: dict, default_sni: str = "") -> None:
    security = params.get("security", "").lower()
    sni = params.get("sni") or params.get("peer") or default_sni
    if security in ("tls", "reality", "xtls"):
        proxy["tls"] = True
    if sni:
        proxy["servername"] = sni
    fp = params.get("fp")
    if fp:
        proxy["client-fingerprint"] = fp
    alpn = params.get("alpn")
    if alpn:
        proxy["alpn"] = [a for a in alpn.split(",") if a]
    if _truthy(params.get("allowInsecure")) or _truthy(params.get("insecure")):
        proxy["skip-cert-verify"] = True
    if security == "reality":
        ro = {}
        if params.get("pbk"):
            ro["public-key"] = params["pbk"]
        if params.get("sid"):
            ro["short-id"] = params["sid"]
        if ro:
            proxy["reality-opts"] = ro


def _parse_vless(url: str, idx: int) -> Optional[dict]:
    u = urllib.parse.urlparse(url)
    if not u.hostname or not u.port:
        return None
    p = _qs(url)
    network = (p.get("type") or "tcp").lower()
    proxy = {
        "name": _name_from_fragment(url, f"vless-{idx}"),
        "type": "vless",
        "server": u.hostname,
        "port": int(u.port),
        "uuid": urllib.parse.unquote(u.username or ""),
        "network": "ws" if network == "httpupgrade" else network,
        "udp": True,
    }
    flow = p.get("flow")
    if flow:
        proxy["flow"] = flow
    _tls_opts(proxy, p, default_sni=p.get("host", ""))
    _net_opts(proxy, p, network)
    return proxy


def _parse_vmess(url: str, idx: int) -> Optional[dict]:
    body = url[len("vmess://"):]
    dec = _b64decode(body)
    if not dec:
        return None
    try:
        cfg = json.loads(dec)
    except (json.JSONDecodeError, ValueError):
        return None
    server = cfg.get("add")
    port = cfg.get("port")
    if not server or not port:
        return None
    network = str(cfg.get("net", "tcp")).lower()
    proxy = {
        "name": str(cfg.get("ps") or f"vmess-{idx}").strip() or f"vmess-{idx}",
        "type": "vmess",
        "server": server,
        "port": int(port),
        "uuid": cfg.get("id", ""),
        "alterId": int(cfg.get("aid", 0) or 0),
        "cipher": cfg.get("scy") or "auto",
        "network": "ws" if network == "httpupgrade" else network,
        "udp": True,
    }
    if str(cfg.get("tls", "")).lower() in ("tls", "reality"):
        proxy["tls"] = True
        sni = cfg.get("sni") or cfg.get("host")
        if sni:
            proxy["servername"] = sni
        if cfg.get("alpn"):
            proxy["alpn"] = [a for a in str(cfg["alpn"]).split(",") if a]
        if cfg.get("fp"):
            proxy["client-fingerprint"] = cfg["fp"]
    params = {"host": cfg.get("host", ""), "path": cfg.get("path", ""),
              "serviceName": cfg.get("path", "")}
    _net_opts(proxy, params, network)
    return proxy


def _parse_ss(url: str, idx: int) -> Optional[dict]:
    name = _name_from_fragment(url, f"ss-{idx}")
    body = url[len("ss://"):]
    if "#" in body:
        body = body.split("#", 1)[0]
    query = ""
    if "?" in body:
        body, query = body.split("?", 1)
    # форматы: BASE64(method:pass)@host:port  |  BASE64(method:pass@host:port)
    method = password = host = None
    port = None
    if "@" in body:
        userinfo, hostport = body.rsplit("@", 1)
        creds = _b64decode(userinfo) or urllib.parse.unquote(userinfo)
        if ":" in creds:
            method, password = creds.split(":", 1)
        if ":" in hostport:
            host, port_s = hostport.rsplit(":", 1)
            port = int(port_s) if port_s.isdigit() else None
    else:
        dec = _b64decode(body)
        if dec and "@" in dec and ":" in dec:
            creds, hostport = dec.rsplit("@", 1)
            method, password = creds.split(":", 1)
            host, port_s = hostport.rsplit(":", 1)
            port = int(port_s) if port_s.isdigit() else None
    if not (method and host and port):
        return None
    proxy = {
        "name": name,
        "type": "ss",
        "server": host,
        "port": port,
        "cipher": method,
        "password": password or "",
        "udp": True,
    }
    # SIP003 plugin (obfs / v2ray-plugin) — по возможности
    if query:
        qp = dict(urllib.parse.parse_qsl(query, keep_blank_values=True))
        plug = qp.get("plugin", "")
        if plug:
            parts = plug.split(";")
            pname = parts[0]
            opts = dict(o.split("=", 1) for o in parts[1:] if "=" in o)
            if pname in ("obfs-local", "simple-obfs"):
                proxy["plugin"] = "obfs"
                proxy["plugin-opts"] = {
                    "mode": opts.get("obfs", "http"),
                    "host": opts.get("obfs-host", ""),
                }
            elif pname == "v2ray-plugin":
                proxy["plugin"] = "v2ray-plugin"
                proxy["plugin-opts"] = {
                    "mode": opts.get("mode", "websocket"),
                    "host": opts.get("host", ""),
                    "path": opts.get("path", "/"),
                    "tls": "tls" in opts,
                }
    return proxy


def _parse_trojan(url: str, idx: int) -> Optional[dict]:
    u = urllib.parse.urlparse(url)
    if not u.hostname or not u.port:
        return None
    p = _qs(url)
    network = (p.get("type") or "tcp").lower()
    proxy = {
        "name": _name_from_fragment(url, f"trojan-{idx}"),
        "type": "trojan",
        "server": u.hostname,
        "port": int(u.port),
        "password": urllib.parse.unquote(u.username or ""),
        "udp": True,
    }
    sni = p.get("sni") or p.get("peer")
    if sni:
        proxy["sni"] = sni
    if p.get("alpn"):
        proxy["alpn"] = [a for a in p["alpn"].split(",") if a]
    if p.get("fp"):
        proxy["client-fingerprint"] = p["fp"]
    if _truthy(p.get("allowInsecure")) or _truthy(p.get("insecure")):
        proxy["skip-cert-verify"] = True
    if network in ("ws", "grpc", "http", "h2"):
        proxy["network"] = "ws" if network == "httpupgrade" else network
        _net_opts(proxy, p, network)
    return proxy


def _parse_hysteria2(url: str, idx: int) -> Optional[dict]:
    u = urllib.parse.urlparse(url)
    if not u.hostname or not u.port:
        return None
    p = _qs(url)
    proxy = {
        "name": _name_from_fragment(url, f"hy2-{idx}"),
        "type": "hysteria2",
        "server": u.hostname,
        "port": int(u.port),
        "password": urllib.parse.unquote(u.username or u.password or ""),
    }
    sni = p.get("sni") or p.get("peer")
    if sni:
        proxy["sni"] = sni
    if _truthy(p.get("insecure")) or _truthy(p.get("allowInsecure")):
        proxy["skip-cert-verify"] = True
    if p.get("obfs"):
        proxy["obfs"] = p["obfs"]
        if p.get("obfs-password"):
            proxy["obfs-password"] = p["obfs-password"]
    return proxy


_DISPATCH = (
    ("vless://", _parse_vless),
    ("vmess://", _parse_vmess),
    ("ss://", _parse_ss),
    ("trojan://", _parse_trojan),
    ("hysteria2://", _parse_hysteria2),
    ("hy2://", _parse_hysteria2),
)


def _parse_one(line: str, idx: int) -> Optional[dict]:
    for scheme, fn in _DISPATCH:
        if line.lower().startswith(scheme):
            try:
                return fn(line, idx)
            except Exception:
                return None
    return None


def _dedup_names(proxies: List[dict]) -> None:
    seen = {}
    for pr in proxies:
        base = pr["name"]
        if base in seen:
            seen[base] += 1
            pr["name"] = f"{base} #{seen[base]}"
        else:
            seen[base] = 1


# ──────────────────────────── public API ────────────────────────────

def parse(text: str) -> Tuple[List[dict], str]:
    """
    Разобрать текст (одна ссылка / много / base64-список) в список clash-нод.
    Returns: (proxies, suggested_name)
    """
    lines = _split_lines(text)
    proxies = []
    for i, ln in enumerate(lines, 1):
        pr = _parse_one(ln, i)
        if pr:
            proxies.append(pr)
    _dedup_names(proxies)
    if not proxies:
        name = "sharelink"
    elif len(proxies) == 1:
        name = proxies[0]["name"]
    else:
        name = proxies[0]["name"] + f" +{len(proxies) - 1}"
    return proxies, name


def build_yaml(proxies: List[dict]) -> bytes:
    """Собрать валидный минимальный Clash-конфиг из списка нод."""
    names = [p["name"] for p in proxies]
    config = {
        "mixed-port": 7890,
        "mode": "rule",
        "proxies": proxies,
        "proxy-groups": [
            {
                "name": "PROXY",
                "type": "select",
                "proxies": names + ["DIRECT"],
            },
            {
                "name": "AUTO",
                "type": "url-test",
                "url": "http://www.gstatic.com/generate_204",
                "interval": 300,
                "lazy": True,
                "proxies": names or ["DIRECT"],
            },
        ],
        "rules": [
            "MATCH,PROXY",
        ],
    }
    buf = io.BytesIO()
    _yaml.dump(config, buf)
    return buf.getvalue()
