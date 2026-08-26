#!/usr/bin/env python3
"""k10s logo generator - every logo file in this directory is emitted from here.

The mark is a ship's helm wheel (the Kubernetes helm, the ⎈ in the README)
with a mouse pointer clicking its rim: "the Kubernetes terminal UI you can
click". Colours are the tokyo-night theme shipped in internal/theme.

    just logo          # rewrite the SVGs and re-export every PNG
    python3 assets/logo/generate.py   # SVGs only (run from the repo root)

Nothing here is hand-edited: tweak the numbers, re-run, commit the diff.
"""

import math, os

OUT = "assets/logo"
BG, FG, SUBTLE, BORDER = "#1a1b26", "#c0caf5", "#565f89", "#3b4261"
ACCENT, ACCENT2 = "#7aa2f7", "#bb9af7"
WHITE, INK = "#f2f6ff", "#1a1b26"
# darker gradient for light backgrounds - the theme accents wash out on white
ACCENT_L, ACCENT2_L = "#4c7fe0", "#9b6ff0"

# ---------------------------------------------------------------- primitives
def wheel(cx, cy, R, sw, hub, nub_r, nub_R, s_in, s_out, col, n=7, rot=-90):
    p = [f'<circle cx="{cx}" cy="{cy}" r="{R}" fill="none" stroke="{col}" stroke-width="{sw}"/>']
    for k in range(n):
        a = math.radians(rot + k * 360.0 / n)
        ca, sa = math.cos(a), math.sin(a)
        if s_out > s_in:
            p.append(f'<line x1="{cx+s_in*ca:.2f}" y1="{cy+s_in*sa:.2f}" '
                     f'x2="{cx+s_out*ca:.2f}" y2="{cy+s_out*sa:.2f}" '
                     f'stroke="{col}" stroke-width="{sw}" stroke-linecap="round"/>')
        if nub_r:
            p.append(f'<circle cx="{cx+nub_R*ca:.2f}" cy="{cy+nub_R*sa:.2f}" r="{nub_r}" fill="{col}"/>')
    if hub:
        p.append(f'<circle cx="{cx}" cy="{cy}" r="{hub}" fill="{col}"/>')
    return "\n    ".join(p)

CUR = "M0 0 L0 34 L8.4 25.6 L14.6 39.4 L21.4 36 L15 22.6 L26 22.6 Z"

def cursor(x, y, s, fill, halo=None, halo_w=0):
    g = f'<g transform="translate({x} {y}) scale({s})">'
    if halo:
        g += f'<path d="{CUR}" fill="{halo}" stroke="{halo}" stroke-width="{halo_w/s:.2f}" stroke-linejoin="round"/>'
    g += f'<path d="{CUR}" fill="{fill}"/></g>'
    return g

def grad(i="g", light=False):
    a, b = (ACCENT_L, ACCENT2_L) if light else (ACCENT, ACCENT2)
    return (f'<linearGradient id="{i}" x1="0" y1="0" x2="1" y2="1">'
            f'<stop offset="0" stop-color="{a}"/><stop offset="1" stop-color="{b}"/></linearGradient>')

def svg(vb, w, h, body, defs="", label="k10s"):
    d = f"<defs>{defs}</defs>" if defs else ""
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="{vb}" width="{w}" height="{h}" '
            f'role="img" aria-label="{label}">\n  {d}\n  <g>\n    {body}\n  </g>\n</svg>\n')

# ---------------------------------------------------------------- app icon
def icon(tile=True, mono=None, paper=BG, ink=WHITE, light=False):
    c = mono or "url(#g)"
    b = ""
    if tile:
        b += (f'<rect x="0" y="0" width="256" height="256" rx="58" fill="{BG}"/>'
              f'<rect x="0.75" y="0.75" width="254.5" height="254.5" rx="57.25" fill="none" '
              f'stroke="{BORDER}" stroke-width="1.5"/>\n    ')
    b += wheel(112, 112, 58, 11, 12, 6.5, 72, 21, 48, c)
    b += "\n    " + (cursor(156, 148, 1.6, mono) if mono
                     else cursor(156, 148, 1.6, ink, paper, 11))
    return svg("0 0 256 256", 256, 256, b, "" if mono else grad(light=light))

# simplified mark for favicons / tiny sizes: rim + hub + pointer, no spokes/nubs
def favicon(tile=True, mono=None):
    c = mono or "url(#g)"
    b = ""
    if tile:
        b += f'<rect x="0" y="0" width="256" height="256" rx="56" fill="{BG}"/>\n    '
    b += wheel(110, 110, 66, 20, 20, 0, 0, 26, 50, c)
    b += "\n    " + (cursor(158, 158, 2.1, mono) if mono
                     else cursor(158, 158, 2.1, WHITE, BG, 18))
    return svg("0 0 256 256", 256, 256, b, "" if mono else grad())

# ---------------------------------------------------------------- wordmark
def letters(col):
    return (f'<g fill="none" stroke="{col}" stroke-width="16" stroke-linecap="round" stroke-linejoin="round">'
            '<path d="M24 32 L24 160"/><path d="M78 84 L28 130"/><path d="M48 110 L80 160"/>'
            '<path d="M114 56 L140 34 L140 160"/><path d="M112 160 L170 160"/>'
            '<path d="M422 96 C414 80 384 74 370 86 C354 100 362 116 386 121 '
            'C412 127 418 141 404 153 C388 166 362 160 356 145"/></g>')

def wordmark(mono=None, text=FG, paper=BG, ink=WHITE, light=False):
    c = mono or "url(#g)"
    b = letters(mono or text)
    b += "\n    " + wheel(262, 96, 52, 13, 11, 6, 66, 19, 44, c)
    b += "\n    " + (cursor(302, 128, 1.15, mono) if mono
                     else cursor(302, 128, 1.15, ink, paper, 9))
    return svg("0 0 452 192", 452, 192, b, "" if mono else grad(light=light))

# ---------------------------------------------------------------- social card
def social():
    W, H = 1280, 640
    mono = "ui-monospace,'SF Mono',Menlo,'DejaVu Sans Mono',monospace"
    dots = []
    for y in range(40, H, 40):
        for x in range(40, W, 40):
            dots.append(f'<circle cx="{x}" cy="{y}" r="1"/>')
    b = (f'<rect width="{W}" height="{H}" fill="{BG}"/>'
         f'<g fill="#2a2c3d">{"".join(dots)}</g>'
         f'<rect x="40" y="40" width="{W-80}" height="{H-80}" rx="28" fill="none" stroke="{BORDER}" stroke-width="2"/>'
         f'<g transform="translate(300 148) scale(1.5)">{letters(FG)}'
         f'{wheel(262, 96, 52, 13, 11, 6, 66, 19, 44, "url(#g)")}'
         f'{cursor(302, 128, 1.15, WHITE, BG, 9)}</g>'
         f'<text x="{W/2}" y="470" text-anchor="middle" font-family="{mono}" font-size="34" fill="{FG}">'
         f'The Kubernetes terminal UI you can click.</text>'
         f'<text x="{W/2}" y="524" text-anchor="middle" font-family="{mono}" font-size="22" fill="{SUBTLE}">'
         f'point-and-click  ·  searchable  ·  themeable  ·  cluster-aware AI</text>'
         f'<text x="{W/2}" y="566" text-anchor="middle" font-family="{mono}" font-size="20" fill="{ACCENT}">'
         f'single static binary  ·  macOS · Linux · Windows</text>')
    return svg(f"0 0 {W} {H}", W, H, b, grad(), "k10s - the Kubernetes terminal UI you can click")

os.makedirs(OUT, exist_ok=True)
files = {
    "mark.svg":            icon(),
    "mark-on-dark.svg":    icon(tile=False),
    "mark-on-light.svg":   icon(tile=False, paper="#ffffff", ink=INK, light=True),
    "mark-mono.svg":       icon(tile=False, mono="currentColor"),
    "favicon.svg":         favicon(),
    "favicon-mono.svg":    favicon(tile=False, mono="currentColor"),
    "wordmark-on-dark.svg":  wordmark(),
    "wordmark-on-light.svg": wordmark(text="#232433", paper="#ffffff", ink=INK, light=True),
    "wordmark-mono.svg":     wordmark(mono="currentColor"),
    "social-preview.svg":    social(),
}
for n, s in files.items():
    open(os.path.join(OUT, n), "w").write(s)
print("\n".join(files))
