import os
import shutil
import struct
import subprocess
import sys
from pathlib import Path

from PIL import Image


APP_NAME = "选中翻译"
ROOT_DIR = Path(__file__).resolve().parent
DIST_DIR = ROOT_DIR / "dist"
BUILD_DIR = ROOT_DIR / "build"
SPEC_PATH = ROOT_DIR / f"{APP_NAME}.spec"
ICON_PNG = ROOT_DIR / "icon.png"
ICON_ICO = ROOT_DIR / "icon.ico"
MODELS_DIR = ROOT_DIR / "models"
PYTHON_DIR = Path(sys.executable).resolve().parent
SITE_PACKAGES_DIR = PYTHON_DIR / "Lib" / "site-packages"


def _configure_stdio() -> None:
    for stream_name in ("stdout", "stderr"):
        stream = getattr(sys, stream_name, None)
        if stream and hasattr(stream, "reconfigure"):
            stream.reconfigure(encoding="utf-8", errors="replace")


def _print_step(index: int, total: int, message: str) -> None:
    print(f"[{index}/{total}] {message}...")


def _remove_if_exists(path: Path) -> None:
    if path.is_dir():
        shutil.rmtree(path)
    elif path.exists():
        path.unlink()


def _generate_bmp_ico(src: Path, dst: Path) -> None:
    """
    手动构造 ICO 文件，每个尺寸以 32-bit BMP + 正确 AND mask 写入。

    这是 Windows 桌面/资源管理器渲染 exe 图标最兼容的格式：
    - 32-bit BGRA 像素数据保留完整 alpha 通道
    - AND mask (1-bit) 标记透明像素，Windows 经典渲染路径必须依赖它
    - Pillow 默认 ICO 写入器的 AND mask 行填充有 bug，会导致透明区变白底

    AND mask 规范：1=透明，0=不透明；每行按 4 字节对齐；图像整体自下而上存储。
    """
    icon_image = Image.open(src).convert("RGBA")
    sizes = [(16, 16), (24, 24), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)]

    frames: list[bytes] = []
    for w, h in sizes:
        resized = icon_image.resize((w, h), Image.Resampling.LANCZOS)
        pixels = list(resized.getdata())  # [(R,G,B,A), ...]

        # AND mask 每行字节数（按 4 字节对齐）
        mask_row_size = ((w + 31) // 32) * 4
        xor_size = w * 4 * h
        mask_size = mask_row_size * h
        bi_size_image = xor_size + mask_size

        # BITMAPINFOHEADER (40 字节)
        bmp_header = struct.pack(
            "<IiiHHIIiiII",
            40,             # biSize
            w,              # biWidth
            h * 2,          # biHeight (XOR + AND 合并高度)
            1,              # biPlanes
            32,             # biBitCount
            0,              # biCompression (BI_RGB)
            bi_size_image,  # biSizeImage
            0,              # biXPelsPerMeter
            0,              # biYPelsPerMeter
            0,              # biClrUsed
            0,              # biClrImportant
        )

        # XOR 数据：BGRA，自下而上
        xor_data = bytearray()
        for y in range(h - 1, -1, -1):
            for x in range(w):
                r, g, b, a = pixels[y * w + x]
                xor_data += bytes((b, g, r, a))

        # AND mask：1-bit，自下而上，每行 4 字节对齐；1=透明，0=不透明
        and_data = bytearray()
        for y in range(h - 1, -1, -1):
            row = bytearray(mask_row_size)
            for x in range(w):
                _, _, _, a = pixels[y * w + x]
                if a < 128:  # 透明
                    byte_idx = x // 8
                    bit_idx = 7 - (x % 8)
                    row[byte_idx] |= (1 << bit_idx)
            and_data += row

        frames.append(bytes(bmp_header) + bytes(xor_data) + bytes(and_data))

    # 组装 ICO
    num = len(frames)
    data_offset = 6 + num * 16  # ICONDIR(6) + 每条目(16)

    entries: list[bytes] = []
    offset = data_offset
    for i, (w, h) in enumerate(sizes):
        frame_bytes = frames[i]
        entries.append(struct.pack(
            "<BBBBHHII",
            0 if w >= 256 else w,   # width (0 = 256)
            0 if h >= 256 else h,   # height (0 = 256)
            0,                       # colorCount (无调色板)
            0,                       # reserved
            1,                       # planes
            32,                      # bitCount
            len(frame_bytes),        # bytesInRes
            offset,                  # imageOffset
        ))
        offset += len(frame_bytes)

    with open(dst, "wb") as f:
        f.write(struct.pack("<HHH", 0, 1, num))   # ICONDIR: reserved=0, type=1(ICO), count
        f.write(b"".join(entries))
        for frame in frames:
            f.write(frame)

    print(f"  已生成 {num} 个尺寸 (BMP-32 + AND mask): {', '.join(f'{s[0]}x{s[1]}' for s in sizes)}")


def _run(cmd: list[str]) -> None:
    env = os.environ.copy()
    env.setdefault("PYTHONUTF8", "1")
    env.setdefault("PYTHONIOENCODING", "utf-8")
    subprocess.run(cmd, cwd=ROOT_DIR, check=True, env=env)


def _pyinstaller_command() -> list[str]:
    dll_dir = PYTHON_DIR / "DLLs"
    tcl_dir = PYTHON_DIR / "tcl"
    return [
        sys.executable,
        "-m",
        "PyInstaller",
        "--windowed",
        "-y",
        "--name",
        APP_NAME,
        "--icon",
        str(ICON_ICO),
        "--add-binary",
        f"{dll_dir / '_tkinter.pyd'};.",
        "--add-binary",
        f"{dll_dir / 'tcl86t.dll'};.",
        "--add-binary",
        f"{dll_dir / 'tk86t.dll'};.",
        "--add-data",
        f"{tcl_dir / 'tcl8.6'};_tcl_data",
        "--add-data",
        f"{tcl_dir / 'tk8.6'};_tk_data",
        "--add-data",
        "translator.py;.",
        "--add-data",
        "hotkey_manager.py;.",
        "--add-data",
        "popup_window.py;.",
        "--add-data",
        "tray_icon.py;.",
        "--add-data",
        "icon.png;.",
        "--add-data",
        f"{SITE_PACKAGES_DIR / 'llama_cpp'};llama_cpp",
        "main.py",
    ]


def main() -> int:
    total_steps = 5
    _configure_stdio()

    print("========================================")
    print("  translate-plugin build")
    print("========================================")
    print()

    _print_step(1, total_steps, "clean old files")
    _remove_if_exists(BUILD_DIR)
    _remove_if_exists(DIST_DIR)
    _remove_if_exists(SPEC_PATH)
    _remove_if_exists(ICON_ICO)

    _print_step(2, total_steps, "generate icon.ico from icon.png (BMP-32 + AND mask)")
    _generate_bmp_ico(ICON_PNG, ICON_ICO)

    _print_step(3, total_steps, "run PyInstaller")
    _run(_pyinstaller_command())

    _print_step(4, total_steps, "copy model files")
    shutil.copytree(MODELS_DIR, DIST_DIR / APP_NAME / "models")

    _print_step(5, total_steps, "clean temp files")
    _remove_if_exists(BUILD_DIR)
    _remove_if_exists(SPEC_PATH)

    print()
    print("========================================")
    print(f"  build complete")
    print(f"  output: {DIST_DIR / APP_NAME}")
    print(f"  exe: {DIST_DIR / APP_NAME / f'{APP_NAME}.exe'}")
    print("========================================")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except subprocess.CalledProcessError as exc:
        print()
        print(f"[ERROR] command failed with exit code {exc.returncode}")
        raise SystemExit(exc.returncode)
