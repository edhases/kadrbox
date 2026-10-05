"""Generate Kadrbox app icons for every platform from one master PNG.

Run from the repository root:  python scripts/branding/generate_icons.py

Why a script and not committed binaries alone: every platform asks for the
same artwork at a different size, and regenerating by hand is how a build
ends up with a 48px icon that is subtly different from the 192px one. The
master lives in the repo so this stays reproducible.

The source artwork already carries its own rounded-square tile, which is
what Android's pre-8 launcher icons and Windows both want as-is. Android's
adaptive icon is the awkward one: the system masks a 108dp canvas down to
about 72dp, so the tile goes in scaled to the safe zone and the background
colour is sampled from inside the tile so the area around it matches.
"""

from __future__ import annotations

import subprocess
import sys
from pathlib import Path

from PIL import Image

ROOT = Path(__file__).resolve().parents[2]
FRONTEND = ROOT / "frontend"
BRANDING = FRONTEND / "assets" / "branding"
MASTER = BRANDING / "kadrbox_icon_1024.png"

# Android launcher icon sizes, in dp, per density bucket.
ANDROID_DENSITIES = {"mdpi": 1, "hdpi": 1.5, "xhdpi": 2, "xxhdpi": 3, "xxxhdpi": 4}

# Adaptive icon canvas is 108dp regardless of density; only the bucket changes.
ADAPTIVE_DP = 108

# Android guarantees the centre 72dp of that canvas survives any mask shape.
SAFE_FRACTION = 72 / 108

ICO_SIZES = [16, 24, 32, 48, 64, 128, 256]
LINUX_SIZES = [16, 32, 48, 64, 128, 256, 512]


def load_master() -> Image.Image:
    if not MASTER.exists():
        sys.exit(f"master icon missing: {MASTER}\nrestore it before regenerating")
    return Image.open(MASTER).convert("RGBA")


def tile_bounds(img: Image.Image) -> tuple[int, int, int, int]:
    """Bounding box of the non-transparent artwork."""
    bbox = img.getchannel("A").getbbox()
    if bbox is None:
        sys.exit("master icon is fully transparent")
    return bbox


def tile_colour(img: Image.Image) -> str:
    """Sample the tile's own background colour, for the adaptive background.

    Taken from a point inside the tile but away from the glyph and the
    wordmark, so the ring the adaptive mask exposes matches the artwork
    instead of forming a visible seam around it.
    """
    left, top, right, bottom = tile_bounds(img)
    x = left + int((right - left) * 0.06)
    y = top + int((bottom - top) * 0.06)
    r, g, b, _ = img.getpixel((x, y))
    return f"#{r:02X}{g:02X}{b:02X}"


def save_png(img: Image.Image, path: Path, size: int | tuple[int, int]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    if isinstance(size, tuple):
        out = img.resize(size, Image.LANCZOS)
    else:
        out = img.resize((size, size), Image.LANCZOS)
    out.save(path, format="PNG", optimize=True)


def write_ios_android() -> None:
    master = load_master()
    res = FRONTEND / "android" / "app" / "src" / "main" / "res"
    bounds = tile_bounds(master)

    for density, scale in ANDROID_DENSITIES.items():
        # Legacy launcher icon: the artwork exactly as designed.
        save_png(master, res / f"mipmap-{density}" / "ic_launcher.png",
                 int(48 * scale))

        # Adaptive foreground: transparent 108dp canvas, tile inside the
        # guaranteed-visible zone so the wordmark is never clipped by a mask.
        canvas_px = int(ADAPTIVE_DP * scale)
        side = int(canvas_px * SAFE_FRACTION)
        tile = master.crop(bounds).resize((side, side), Image.LANCZOS)
        canvas = Image.new("RGBA", (canvas_px, canvas_px), (0, 0, 0, 0))
        canvas.alpha_composite(tile, ((canvas_px - side) // 2, (canvas_px - side) // 2))
        save_png(canvas, res / f"drawable-{density}" / "ic_launcher_foreground.png",
                 (canvas_px, canvas_px))

    colour = tile_colour(master)
    colors = res / "values" / "colors.xml"
    text = colors.read_text(encoding="utf-8")
    import re
    text = re.sub(
        r'<color name="ic_launcher_background">[^<]*</color>',
        f'<color name="ic_launcher_background">{colour}</color>',
        text,
    )
    colors.write_text(text, encoding="utf-8", newline="\n")
    print(f"  android: 5 densities, adaptive background {colour}")


def write_windows() -> None:
    master = load_master()
    bounds = tile_bounds(master)
    tile = master.crop(bounds)
    target = FRONTEND / "windows" / "runner" / "resources" / "app_icon.ico"
    target.parent.mkdir(parents=True, exist_ok=True)
    tile.save(target, format="ICO", sizes=[(s, s) for s in ICO_SIZES])
    print(f"  windows: app_icon.ico with {len(ICO_SIZES)} sizes")


def write_linux() -> None:
    master = load_master()
    bounds = tile_bounds(master)
    tile = master.crop(bounds)
    icons = FRONTEND / "linux" / "flutter" / "assets" / "icons"
    for size in LINUX_SIZES:
        save_png(tile, icons / f"{size}x{size}.png", size)

    desktop = FRONTEND / "linux" / "com.edhases.kadrbox.desktop"
    desktop.write_text(
        "[Desktop Entry]\n"
        "Name=Kadrbox\n"
        "Comment=Media player with cross-device progress sync and watch parties\n"
        "Type=Application\n"
        "Exec=kadrbox\n"
        "Icon=kadrbox\n"
        "Terminal=false\n"
        "Categories=AudioVideo;Video;Player;\n"
        "StartupWMClass=kadrbox\n",
        encoding="utf-8",
        newline="\n",
    )
    print(f"  linux:   {len(LINUX_SIZES)} PNGs + .desktop entry")


def write_windows_runner_resources() -> None:
    """Flutter's Windows template expects the ICO at a fixed relative path."""
    master = load_master()
    bounds = tile_bounds(master)
    tile = master.crop(bounds)
    target = FRONTEND / "windows" / "runner" / "resources" / "app_icon.ico"
    tile.save(target, format="ICO", sizes=[(s, s) for s in ICO_SIZES])


def main() -> None:
    BRANDING.mkdir(parents=True, exist_ok=True)
    write_ios_android()
    write_windows()
    write_linux()
    print("done")


if __name__ == "__main__":
    main()