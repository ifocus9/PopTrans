"""
config_manager.py — 配置管理模块

管理应用配置（自定义快捷键等），持久化到 settings.json。
"""

import json
import os
import sys
import logging
from pathlib import Path
from typing import Dict, Any

logger = logging.getLogger(__name__)

DEFAULT_CONFIG: Dict[str, Any] = {
    "hotkey": "<ctrl>+<alt>+q",
    "hotkey_display": "Ctrl+Alt+Q",
}


def _get_config_path() -> Path:
    """获取配置文件路径（与 exe / 脚本同目录）"""
    if getattr(sys, "frozen", False):
        base_dir = Path(os.path.dirname(sys.executable))
    else:
        base_dir = Path(os.path.dirname(os.path.abspath(__file__)))
    return base_dir / "settings.json"


def load_config() -> Dict[str, Any]:
    """加载配置，不存在或损坏时返回默认值"""
    config_path = _get_config_path()
    if config_path.exists():
        try:
            with open(config_path, "r", encoding="utf-8") as f:
                user_cfg = json.load(f)
            merged = {**DEFAULT_CONFIG, **user_cfg}
            logger.info(f"已加载配置: {config_path}")
            return merged
        except (json.JSONDecodeError, IOError) as e:
            logger.warning(f"配置读取失败，使用默认值: {e}")
    return DEFAULT_CONFIG.copy()


def save_config(config: Dict[str, Any]) -> bool:
    """保存配置到文件"""
    config_path = _get_config_path()
    try:
        with open(config_path, "w", encoding="utf-8") as f:
            json.dump(config, f, indent=2, ensure_ascii=False)
        logger.info(f"配置已保存: {config_path}")
        return True
    except IOError as e:
        logger.error(f"保存配置失败: {e}")
        return False


def get_hotkey() -> str:
    """获取 pynput 格式的快捷键字符串"""
    return load_config().get("hotkey", DEFAULT_CONFIG["hotkey"])


def get_hotkey_display() -> str:
    """获取用于 UI 显示的快捷键文本"""
    return load_config().get("hotkey_display", DEFAULT_CONFIG["hotkey_display"])


def set_hotkey(hotkey: str, display: str) -> bool:
    """更新快捷键并持久化"""
    config = load_config()
    config["hotkey"] = hotkey
    config["hotkey_display"] = display
    return save_config(config)

