"""Génère le logo de Terminal (un logiciel Arkerion) : symbole, nom, avatar, carte de partage, icône.

    python docs/brand/build.py

La police Arkerion Sans est lue dans le dépôt Arkerion-Docs
(variable d'environnement ARKERION_FONTS, ou dossier frère ../arkerion-docs/src/fonts).
Tout le texte est vectorisé : les fichiers produits n'ont besoin d'aucune police.

Règles reprises de l'identité Arkerion (Arkerion-Docs, pages Logo et Nebula) :
grille de 64 unités, modules aux coins arrondis (8 dehors, 6 dedans),
arrondi signature de 18 en haut à droite du module principal, logo en noir
#0D0E10 ou en blanc uniquement, nom composé en Arkerion Sans resserré (-25).
"""

import os
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from arkbrand import (  # noqa: E402
    Font,
    Scene,
    compose,
    ops_bounds,
    ops_from_d,
    rect_d,
    scale,
    transform_ops,
    translate,
)

OUT = Path(__file__).parent
ROOT = OUT.parent.parent


def find_fonts():
    candidates = [
        os.environ.get("ARKERION_FONTS"),
        ROOT.parent / "arkerion-docs" / "src" / "fonts",
        ROOT.parent / "Arkerion-Docs" / "src" / "fonts",
        Path("/home/claude/orvaen/arkerion-docs/src/fonts"),
    ]
    for c in candidates:
        if c and (Path(c) / "ArkerionSans-Regular.ttf").exists():
            return Path(c)
    sys.exit("Polices Arkerion introuvables : définis ARKERION_FONTS (dossier src/fonts d'Arkerion-Docs).")


FONTS = find_fonts()
SANS = Font(FONTS / "ArkerionSans-Regular.ttf")
TRACK = -25  # composition resserrée du nom, comme les logos Arkerion et Nebula

# Logo : noir et blanc uniquement
INK = "#0D0E10"
WHITE = "#FFFFFF"


# ---------------------------------------------------------------------------
# Le symbole « Invite »
#
# Un module principal, l'écran, avec l'arrondi signature d'Arkerion en haut à droite.
# L'invite de commande y est creusée en modules : un chevron de trois carrés en
# diagonale et le curseur. Même grille, mêmes arrondis que Structure et Amas.

SCREEN = "M26 18H64A18 18 0 0 1 82 36V74A8 8 0 0 1 74 82H26A8 8 0 0 1 18 74V26A8 8 0 0 1 26 18Z"
MODULE, MODULE_R = 10, 3
PROMPT = (  # (x, y, largeur, hauteur) des modules de l'invite
    (29, 35, MODULE, MODULE),
    (39, 45, MODULE, MODULE),
    (29, 55, MODULE, MODULE),
    (53, 55, 16, MODULE),  # curseur
)


def symbol_ops():
    d = SCREEN + "".join(rect_d(x, y, w, h, MODULE_R) for x, y, w, h in PROMPT)
    return ops_from_d(d)


def place(ops, x, y, size):
    """Place un dessin de la grille 18..82 dans un carré (x, y, size)."""
    return transform_ops(ops, compose(translate(x, y), compose(scale(size / 64), translate(-18, -18))))


# ---------------------------------------------------------------------------
# Fichiers du logo


def write(name, content):
    (OUT / name).write_text(content, encoding="utf-8")
    print("  ", name)


def name_ops(size=1000, x=0, y=0):
    return SANS.layout("Terminal", size, x, y, tracking=TRACK)


def build_logo():
    print("Logo")
    for variant, color in (("noir", INK), ("blanc", WHITE)):
        # symbole seul (grille 18..82)
        s = Scene(64, 64, title="Terminal, symbole")
        s.add(symbol_ops(), color, rule="evenodd")
        write(f"terminal-symbole-{variant}.svg", s.svg(display_width=512, viewbox=(18, 18, 64, 64)))

        # logo complet : même construction que Nebula (symbole de 1000 unités, nom à 1250)
        word, _ = name_ops()
        wb = ops_bounds(word)
        logo = Scene(0, 0, title="Terminal")
        logo.add(place(symbol_ops(), 0, -850, 1000), color, rule="evenodd")
        logo.add(transform_ops(word, translate(1250, 0)), color)
        width = 1250 + wb[2]
        write(f"terminal-logo-{variant}.svg", logo.svg(display_width=width / 5, viewbox=(0, -850, width, 1000)))

        # nom seul
        nom = Scene(0, 0, title="Terminal")
        nom.add(word, color)
        pad = 20
        vb = (wb[0] - pad, wb[1] - pad, wb[2] - wb[0] + 2 * pad, wb[3] - wb[1] + 2 * pad)
        write(f"terminal-nom-{variant}.svg", nom.svg(display_width=vb[2] / 4, viewbox=vb))

    # avatar carré (fond plein), comme nebula-avatar
    av = Scene(100, 100, background=WHITE, title="Terminal")
    av.add(place(symbol_ops(), 50 - 32 * 0.82, 50 - 32 * 0.82, 64 * 0.82), INK, rule="evenodd")
    write("terminal-avatar-noir.svg", av.svg(display_width=1024))
    av.png(OUT / "terminal-avatar-noir.png", 1024)
    print("   terminal-avatar-noir.png")


def build_icon():
    """Icône Windows : symbole noir bordé de blanc, lisible sur barre des tâches claire ou sombre."""
    print("Icône")
    from PIL import Image

    sizes = [256, 64, 48, 40, 32, 24, 20, 16]
    images = []
    for px in sizes:
        rim = 2.2 if px <= 32 else 1.4  # liseré blanc, en unités de la grille
        sc = Scene(64 + 2 * rim, 64 + 2 * rim)
        outer = transform_ops(
            ops_from_d("M26 18H64A18 18 0 0 1 82 36V74A8 8 0 0 1 74 82H26A8 8 0 0 1 18 74V26A8 8 0 0 1 26 18Z"),
            compose(translate(32 + rim, 32 + rim), compose(scale((64 + 2 * rim) / 64), translate(-50, -50))),
        )
        sc.add(outer, WHITE)
        sc.add(transform_ops(symbol_ops(), translate(rim - 18, rim - 18)), INK, rule="evenodd")
        tmp = OUT / f".icon-{px}.png"
        images.append(sc.png(tmp, px, supersample=8))
        tmp.unlink()
    images[0].save(OUT / "terminal.ico", sizes=[(s, s) for s in sizes], append_images=images[1:])
    print("   terminal.ico")


# ---------------------------------------------------------------------------
# Carte de partage


def text(scene, font, s, size, x, y, color, tracking=0):
    ops, _ = font.layout(s, size, 0, 0, tracking=tracking)
    scene.add(transform_ops(ops, translate(x, y)), color)


def lockup(scene, x, y, height, color, center=False):
    """Logo complet (symbole + nom) de hauteur de symbole `height`."""
    k = height / 1000
    word, _ = name_ops()
    wb = ops_bounds(word)
    width = (1250 + wb[2]) * k
    if center:
        x -= width / 2
    scene.add(place(symbol_ops(), x, y, height), color, rule="evenodd")
    scene.add(transform_ops(word, compose(translate(x + 1250 * k, y + 850 * k), scale(k))), color)


def build_share():
    print("Carte de partage")
    W, H = 1280, 640
    s = Scene(W, H, background=INK, title="Terminal, un logiciel Arkerion")
    lockup(s, W / 2, 251, 138, WHITE, center=True)
    text(s, SANS, "Un logiciel Arkerion", 26, 80, 567, "#8A8794", TRACK)
    s.png(OUT / "terminal-partage.png", W)
    print("   terminal-partage.png")


if __name__ == "__main__":
    build_logo()
    build_icon()
    build_share()
    print("Terminé.")
