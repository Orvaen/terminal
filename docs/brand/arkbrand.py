"""Petite boîte à outils de dessin vectoriel pour les visuels Arkerion.

Tout est construit en formes pleines : chemins SVG, rectangles arrondis et texte
vectorisé à partir des polices Arkerion (le texte devient des contours, les SVG
s'affichent donc partout sans la police). Le même dessin peut être exporté en
SVG et rendu en PNG (rendu maison, anticrénelé par suréchantillonnage), ce qui
sert aux aperçus, à l'avatar, à la carte de partage et à l'icône Windows.
"""

from __future__ import annotations

import math
from dataclasses import dataclass, field

from fontTools.pens.basePen import BasePen
from fontTools.pens.recordingPen import RecordingPen
from fontTools.pens.transformPen import TransformPen
from fontTools.svgLib.path import parse_path
from fontTools.ttLib import TTFont

# ---------------------------------------------------------------------------
# Géométrie


def rect_d(x, y, w, h, r=0):
    """Rectangle arrondi en chemin SVG (r limité à la moitié du plus petit côté)."""
    r = min(r, w / 2, h / 2)
    if r <= 0:
        return f"M{x} {y}H{x + w}V{y + h}H{x}Z"
    return (
        f"M{x + r} {y}H{x + w - r}A{r} {r} 0 0 1 {x + w} {y + r}"
        f"V{y + h - r}A{r} {r} 0 0 1 {x + w - r} {y + h}"
        f"H{x + r}A{r} {r} 0 0 1 {x} {y + h - r}V{y + r}A{r} {r} 0 0 1 {x + r} {y}Z"
    )


def ops_from_d(d, transform=(1, 0, 0, 1, 0, 0)):
    rec = RecordingPen()
    parse_path(d, TransformPen(rec, transform))
    return rec.value


def compose(t1, t2):
    """Matrice t1 appliquée après t2 (format affine fontTools a b c d e f)."""
    a1, b1, c1, d1, e1, f1 = t1
    a2, b2, c2, d2, e2, f2 = t2
    return (
        a1 * a2 + c1 * b2,
        b1 * a2 + d1 * b2,
        a1 * c2 + c1 * d2,
        b1 * c2 + d1 * d2,
        a1 * e2 + c1 * f2 + e1,
        b1 * e2 + d1 * f2 + f1,
    )


def translate(x, y):
    return (1, 0, 0, 1, x, y)


def scale(s, sy=None):
    return (s, 0, 0, s if sy is None else sy, 0, 0)


def transform_ops(ops, t):
    rec = RecordingPen()
    pen = TransformPen(rec, t)
    for op, args in ops:
        getattr(pen, op)(*args)
    return rec.value


def ops_bounds(ops):
    xs, ys = [], []
    for _, args in ops:
        for pt in args:
            if pt is not None:
                xs.append(pt[0])
                ys.append(pt[1])
    return min(xs), min(ys), max(xs), max(ys)


def fmt(v):
    s = f"{v:.2f}".rstrip("0").rstrip(".")
    return "0" if s in ("-0", "") else s


def d_from_ops(ops):
    out = []
    for op, args in ops:
        pts = " ".join(f"{fmt(x)} {fmt(y)}" for x, y in args) if args else ""
        if op == "moveTo":
            out.append("M" + pts)
        elif op == "lineTo":
            out.append("L" + pts)
        elif op == "curveTo":
            out.append("C" + pts)
        elif op == "qCurveTo":
            # suite de quadratiques TrueType : on explicite les points implicites
            pts_list = list(args)
            on_last = pts_list[-1]
            offs = pts_list[:-1]
            for i, off in enumerate(offs):
                if i < len(offs) - 1:
                    nxt = offs[i + 1]
                    on = ((off[0] + nxt[0]) / 2, (off[1] + nxt[1]) / 2)
                else:
                    on = on_last
                out.append(f"Q{fmt(off[0])} {fmt(off[1])} {fmt(on[0])} {fmt(on[1])}")
        elif op in ("closePath", "endPath"):
            out.append("Z")
    return "".join(out)


# ---------------------------------------------------------------------------
# Texte vectorisé


class Font:
    def __init__(self, path):
        self.tt = TTFont(path)
        self.upm = self.tt["head"].unitsPerEm
        self.cmap = self.tt.getBestCmap()
        self.glyphs = self.tt.getGlyphSet()
        self.hmtx = self.tt["hmtx"]
        self.kern = self._read_kerning()
        self._cache = {}

    def _read_kerning(self):
        kern = {}
        if "GPOS" not in self.tt:
            return kern
        for lookup in self.tt["GPOS"].table.LookupList.Lookup:
            if lookup.LookupType != 2:
                continue
            for st in lookup.SubTable:
                if st.Format == 1:
                    for first, ps in zip(st.Coverage.glyphs, st.PairSet):
                        for rec in ps.PairValueRecord:
                            v = getattr(rec.Value1, "XAdvance", 0) or 0
                            if v:
                                kern[(first, rec.SecondGlyph)] = v
                elif st.Format == 2:
                    c1 = st.ClassDef1.classDefs
                    c2 = st.ClassDef2.classDefs
                    for first in st.Coverage.glyphs:
                        row = st.Class1Record[c1.get(first, 0)]
                        for second in self.tt.getGlyphOrder():
                            v = getattr(row.Class2Record[c2.get(second, 0)].Value1, "XAdvance", 0) or 0
                            if v:
                                kern[(first, second)] = v
        return kern

    def glyph_ops(self, name):
        if name not in self._cache:
            rec = RecordingPen()
            self.glyphs[name].draw(rec)
            self._cache[name] = rec.value
        return self._cache[name]

    def layout(self, text, size, x=0, y=0, tracking=0):
        """Contours du texte. (x, y) = début de la ligne de base, y vers le bas (SVG)."""
        k = size / self.upm
        ops = []
        pen_x = 0.0
        prev = None
        for ch in text:
            name = self.cmap.get(ord(ch))
            if name is None:
                raise ValueError(f"glyphe absent : {ch!r}")
            if prev is not None:
                pen_x += self.kern.get((prev, name), 0)
            t = (k, 0, 0, -k, x + pen_x * k, y)
            ops += transform_ops(self.glyph_ops(name), t)
            pen_x += self.hmtx[name][0] + tracking
            prev = name
        return ops, pen_x * k

    def width(self, text, size, tracking=0):
        return self.layout(text, size, tracking=tracking)[1]


# ---------------------------------------------------------------------------
# Scène


@dataclass
class Shape:
    ops: list
    fill: str
    opacity: float = 1.0
    rule: str = "nonzero"  # "evenodd" pour un dessin à creux (le symbole)


@dataclass
class Scene:
    width: float
    height: float
    background: str | None = None
    shapes: list = field(default_factory=list)
    title: str = ""

    def add(self, ops, fill, opacity=1.0, rule="nonzero"):
        self.shapes.append(Shape(ops, fill, opacity, rule))
        return self

    def add_d(self, d, fill, transform=(1, 0, 0, 1, 0, 0), opacity=1.0, rule="evenodd"):
        return self.add(ops_from_d(d, transform), fill, opacity, rule)

    def add_rect(self, x, y, w, h, r, fill, opacity=1.0):
        return self.add_d(rect_d(x, y, w, h, r), fill, opacity=opacity)

    # -- SVG -------------------------------------------------------------
    def svg(self, display_width=None, viewbox=None):
        vb = viewbox or (0, 0, self.width, self.height)
        dw = display_width or vb[2]
        dh = dw * vb[3] / vb[2]
        parts = [
            f'<svg xmlns="http://www.w3.org/2000/svg" width="{fmt(dw)}" height="{fmt(dh)}" '
            f'viewBox="{" ".join(fmt(v) for v in vb)}"'
            + (f' role="img" aria-label="{self.title}"><title>{self.title}</title>' if self.title else ">")
        ]
        if self.background:
            parts.append(f'<rect x="{fmt(vb[0])}" y="{fmt(vb[1])}" width="{fmt(vb[2])}" height="{fmt(vb[3])}" fill="{self.background}"/>')
        # formes consécutives de même couleur regroupées en un seul chemin
        groups = []
        for s in self.shapes:
            key = (s.fill, s.opacity, s.rule)
            if groups and groups[-1][0] == key and s.rule == "nonzero":
                groups[-1][1].append(s.ops)
            else:
                groups.append((key, [s.ops]))
        for (fill, op, rule), opss in groups:
            d = "".join(d_from_ops(o) for o in opss)
            extra = f' fill-opacity="{fmt(op)}"' if op < 1 else ""
            parts.append(f'<path fill="{fill}"{extra} fill-rule="{rule}" d="{d}"/>')
        parts.append("</svg>")
        return "".join(parts)

    # -- PNG -------------------------------------------------------------
    def png(self, path, out_width, viewbox=None, supersample=4, background=None):
        from PIL import Image, ImageChops, ImageDraw

        vb = viewbox or (0, 0, self.width, self.height)
        k = out_width / vb[2]
        out_h = round(vb[3] * k)
        W, H = out_width * supersample, out_h * supersample
        ks = k * supersample
        bg = background if background is not None else self.background
        img = Image.new("RGBA", (W, H), _rgba(bg) if bg else (0, 0, 0, 0))
        for s in self.shapes:
            polys = [p for p in _flatten(s.ops, ks, -vb[0] * ks, -vb[1] * ks) if len(p) >= 3]
            if s.rule == "evenodd":
                mask = Image.new("1", (W, H), 0)
                for poly in polys:
                    m = Image.new("1", (W, H), 0)
                    ImageDraw.Draw(m).polygon(poly, fill=1)
                    mask = ImageChops.logical_xor(mask, m)
            else:
                # non nul : union des contours d'un sens, moins celle des contours de l'autre sens
                pos, neg = Image.new("1", (W, H), 0), Image.new("1", (W, H), 0)
                main = None
                for poly in polys:
                    a = _area(poly)
                    if main is None and a:
                        main = 1 if a > 0 else -1
                    ImageDraw.Draw(pos if a * (main or 1) >= 0 else neg).polygon(poly, fill=1)
                mask = ImageChops.subtract(pos.convert("L"), neg.convert("L")).convert("1")
            mask = mask.convert("L")
            if s.opacity < 1:
                mask = mask.point(lambda v: int(v * s.opacity))
            layer = Image.new("RGBA", (W, H), _rgba(s.fill))
            img = Image.composite(layer, img, mask) if s.opacity >= 1 else Image.alpha_composite(
                img, Image.merge("RGBA", (*layer.split()[:3], mask))
            )
        img = img.resize((out_width, out_h), Image.LANCZOS)
        img.save(path)
        return img


def _area(poly):
    return sum(x1 * y2 - x2 * y1 for (x1, y1), (x2, y2) in zip(poly, poly[1:] + poly[:1])) / 2


def _rgba(c):
    c = c.lstrip("#")
    if len(c) == 6:
        c += "ff"
    return tuple(int(c[i : i + 2], 16) for i in range(0, 8, 2))


class _FlattenPen(BasePen):
    def __init__(self, k, dx, dy):
        super().__init__(None)
        self.k, self.dx, self.dy = k, dx, dy
        self.polys, self.cur = [], []

    def _pt(self, p):
        return (p[0] * self.k + self.dx, p[1] * self.k + self.dy)

    def _moveTo(self, p):
        if self.cur:
            self.polys.append(self.cur)
        self.cur = [self._pt(p)]

    def _lineTo(self, p):
        self.cur.append(self._pt(p))

    def _curveToOne(self, p1, p2, p3):
        p0 = self._getCurrentPoint()
        n = max(4, int(math.dist(p0, p3) * self.k / 3))
        for i in range(1, n + 1):
            t = i / n
            mt = 1 - t
            x = mt**3 * p0[0] + 3 * mt * mt * t * p1[0] + 3 * mt * t * t * p2[0] + t**3 * p3[0]
            y = mt**3 * p0[1] + 3 * mt * mt * t * p1[1] + 3 * mt * t * t * p2[1] + t**3 * p3[1]
            self.cur.append(self._pt((x, y)))

    def _qCurveToOne(self, p1, p2):
        p0 = self._getCurrentPoint()
        n = max(4, int(math.dist(p0, p2) * self.k / 3))
        for i in range(1, n + 1):
            t = i / n
            mt = 1 - t
            x = mt * mt * p0[0] + 2 * mt * t * p1[0] + t * t * p2[0]
            y = mt * mt * p0[1] + 2 * mt * t * p1[1] + t * t * p2[1]
            self.cur.append(self._pt((x, y)))

    def _closePath(self):
        if self.cur:
            self.polys.append(self.cur)
        self.cur = []

    _endPath = _closePath


def _flatten(ops, k, dx, dy):
    pen = _FlattenPen(k, dx, dy)
    for op, args in ops:
        getattr(pen, op)(*args)
    if pen.cur:
        pen.polys.append(pen.cur)
    return pen.polys


def round_poly(points, radii):
    """Polygone (orthogonal ou non) aux coins arrondis : convexes et concaves selon le sens du virage.
    points dans le sens horaire à l'écran (y vers le bas) ; radii = un rayon par sommet."""
    n = len(points)
    d = []
    for i in range(n):
        p0, p1, p2 = points[i - 1], points[i], points[(i + 1) % n]
        r = radii[i]
        v_in = (p1[0] - p0[0], p1[1] - p0[1])
        v_out = (p2[0] - p1[0], p2[1] - p1[1])
        l_in, l_out = math.hypot(*v_in), math.hypot(*v_out)
        u_in = (v_in[0] / l_in, v_in[1] / l_in)
        u_out = (v_out[0] / l_out, v_out[1] / l_out)
        cross = u_in[0] * u_out[1] - u_in[1] * u_out[0]
        cos_t = max(-1.0, min(1.0, -(u_in[0] * u_out[0] + u_in[1] * u_out[1])))
        half = math.acos(cos_t) / 2  # demi-angle intérieur au sommet
        cut = r / math.tan(half) if r and half > 1e-6 else 0
        a = (p1[0] - u_in[0] * cut, p1[1] - u_in[1] * cut)
        b = (p1[0] + u_out[0] * cut, p1[1] + u_out[1] * cut)
        d.append(("M" if i == 0 else "L") + f"{fmt(a[0])} {fmt(a[1])}")
        if cut:
            sweep = 1 if cross > 0 else 0
            d.append(f"A{fmt(r)} {fmt(r)} 0 0 {sweep} {fmt(b[0])} {fmt(b[1])}")
    return "".join(d) + "Z"


def rotate_about(deg, cx, cy):
    t = math.radians(deg)
    c, s = math.cos(t), math.sin(t)
    return compose(translate(cx, cy), compose((c, s, -s, c, 0, 0), translate(-cx, -cy)))
