"""Shared paths for source and PyInstaller runtime modes."""

import os
import sys
from pathlib import Path


def application_dir() -> Path:
    if getattr(sys, "frozen", False):
        return Path(sys.executable).resolve().parent
    return Path(__file__).resolve().parent.parent


def settings_path() -> Path:
    app_dir = application_dir()
    local = app_dir / "settings.json"
    return local


def log_path(filename: str) -> Path:
    path = settings_path().parent / filename
    path.parent.mkdir(parents=True, exist_ok=True)
    return path


def model_path(*parts: str) -> str:
    return os.fspath(application_dir().joinpath("models", *parts))
