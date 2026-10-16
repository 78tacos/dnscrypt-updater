#!/usr/bin/env python3
"""Regenerate tray icons: shield + keyhole (encrypted DNS / security).

Writes internal/app/icon.png (32x32) and icon.ico (16/32/48).
Requires Pillow: pip install pillow
"""
from __future__ import annotations

import struct
import sys
from io import BytesIO
from pathlib import Path

try:
    from PIL import Image, ImageDraw
except ImportError:
    print("Pillow required: pip install pillow", file=sys.stderr)
    sys.exit(1)

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / "internal" / "app"


def make_icon(size: int) -> Image.Image:
    scale = 4
    S = size * scale
    img = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)

    rim = (70, 214, 155, 255)
    body = (18, 110, 88, 255)
    hole = (6, 24, 18, 255)

    def P(x: float, y: float) -> tuple[float, float]:
        return (x * (S - 1), y * (S - 1))

    d.polygon(
        [
            P(0.50, 0.05),
            P(0.90, 0.18),
            P(0.92, 0.50),
            P(0.70, 0.80),
            P(0.50, 0.95),
            P(0.30, 0.80),
            P(0.08, 0.50),
            P(0.10, 0.18),
        ],
        fill=rim,
    )
    d.polygon(
        [
            P(0.50, 0.12),
            P(0.82, 0.23),
            P(0.84, 0.50),
            P(0.66, 0.76),
            P(0.50, 0.88),
            P(0.34, 0.76),
            P(0.16, 0.50),
            P(0.18, 0.23),
        ],
        fill=body,
    )

    cx, cy = S * 0.50, S * 0.44
    r = S * 0.115
    d.ellipse([cx - r, cy - r, cx + r, cy + r], fill=hole)
    tw, bw = r * 0.70, r * 0.42
    top, bot = cy + r * 0.45, cy + r * 2.35
    d.polygon(
        [(cx - tw, top), (cx + tw, top), (cx + bw, bot), (cx - bw, bot)],
        fill=hole,
    )

    out = img.resize((size, size), Image.Resampling.LANCZOS)
    px = out.load()
    for y in range(size):
        for x in range(size):
            r0, g0, b0, a0 = px[x, y]
            if a0 < 28:
                px[x, y] = (0, 0, 0, 0)
            elif a0 < 200:
                px[x, y] = (r0, g0, b0, min(255, a0 + 40))
    return out


def png_bytes(im: Image.Image) -> bytes:
    buf = BytesIO()
    im.save(buf, format="PNG")
    return buf.getvalue()


def write_ico(path: Path, images: list[Image.Image]) -> None:
    entries: list[bytes] = []
    blobs: list[bytes] = []
    offset = 6 + 16 * len(images)
    for im in images:
        data = png_bytes(im)
        w = im.width if im.width < 256 else 0
        h = im.height if im.height < 256 else 0
        entries.append(struct.pack("<BBBBHHII", w, h, 0, 0, 1, 32, len(data), offset))
        blobs.append(data)
        offset += len(data)
    with path.open("wb") as f:
        f.write(struct.pack("<HHH", 0, 1, len(images)))
        for e in entries:
            f.write(e)
        for b in blobs:
            f.write(b)


def main() -> None:
    png16, png32, png48 = make_icon(16), make_icon(32), make_icon(48)
    png32.save(OUT / "icon.png", format="PNG", optimize=True)
    write_ico(OUT / "icon.ico", [png16, png32, png48])
    print(f"wrote {OUT / 'icon.png'} and {OUT / 'icon.ico'}")


if __name__ == "__main__":
    main()
