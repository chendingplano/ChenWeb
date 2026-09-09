"""
3D DSLR camera - exploded parts view.

Builds an interactive Plotly figure of a DSLR-style camera with its main
parts pulled apart along their mounting axes. Each exploded part has a
dotted leader line back to where it attaches and a text label; hovering a
part shows its name.

Run:
    uv run --no-project --with plotly --with numpy python draw-3d.py

Writes camera-3d.html next to this file and opens it in the browser.
"""

import os

import numpy as np
import plotly.graph_objects as go

# --- palette -------------------------------------------------------------

BODY = "#7c8288"        # magnesium chassis
GRIP = "#41464b"        # rubberised hand grip
PRISM = "#5b6167"       # pentaprism hump
LENS_BARREL = "#33383d"
RING_DARK = "#8a9198"   # ribbed focus ring
METAL_LT = "#d0d4d8"    # bright dial
METAL_MD = "#a6acb2"    # satin metal ring / socket
GLASS = "#5194cf"       # coated front element
ACCENT = "#c74d2d"      # shutter release
SCREEN = "#12161b"      # LCD
PANEL = "#697079"       # doors / eyepiece

LEADER = "#9aa0a6"
ANCHOR = "#5f6469"
LABEL = "#1d1f21"

# Triangle indices shared by every 8-vertex solid (box and frustum), wound
# so every face normal points outward.
_HEX_FACES = (
    [0, 0, 4, 4, 0, 0, 2, 2, 0, 0, 1, 1],  # i
    [2, 3, 5, 6, 1, 5, 3, 7, 4, 7, 2, 6],  # j
    [1, 2, 6, 7, 5, 4, 7, 6, 7, 3, 6, 5],  # k
)


# --- geometry ----------------------------------------------------------

def _box(center, size):
    cx, cy, cz = center
    sx, sy, sz = size
    x0, x1 = cx - sx / 2, cx + sx / 2
    y0, y1 = cy - sy / 2, cy + sy / 2
    z0, z1 = cz - sz / 2, cz + sz / 2
    xs = [x0, x1, x1, x0, x0, x1, x1, x0]
    ys = [y0, y0, y1, y1, y0, y0, y1, y1]
    zs = [z0, z0, z0, z0, z1, z1, z1, z1]
    return (xs, ys, zs), _HEX_FACES


def _frustum(center, bottom, top, height, shift=(0.0, 0.0)):
    """8-vertex solid with a smaller, optionally offset top face - the
    pentaprism hump."""
    cx, cy, cz = center
    bx, by = bottom
    tx, ty = top
    dx, dy = shift
    z0, z1 = cz - height / 2, cz + height / 2
    xs = [cx - bx / 2, cx + bx / 2, cx + bx / 2, cx - bx / 2,
          cx - tx / 2 + dx, cx + tx / 2 + dx, cx + tx / 2 + dx, cx - tx / 2 + dx]
    ys = [cy - by / 2, cy - by / 2, cy + by / 2, cy + by / 2,
          cy - ty / 2 + dy, cy - ty / 2 + dy, cy + ty / 2 + dy, cy + ty / 2 + dy]
    zs = [z0, z0, z0, z0, z1, z1, z1, z1]
    return (xs, ys, zs), _HEX_FACES


def _cylinder(center, radius, height, axis, n=56):
    cx, cy, cz = center
    t = np.linspace(0.0, 2.0 * np.pi, n, endpoint=False)
    ring_u, ring_v = radius * np.cos(t), radius * np.sin(t)
    w0, w1 = -height / 2.0, height / 2.0

    u = np.concatenate([ring_u, ring_u, [0.0, 0.0]])
    v = np.concatenate([ring_v, ring_v, [0.0, 0.0]])
    w = np.concatenate([np.full(n, w0), np.full(n, w1), [w0, w1]])

    if axis == "z":
        x, y, z = u, v, w
    elif axis == "y":
        x, y, z = u, w, v
    else:  # "x"
        x, y, z = w, u, v
    x, y, z = x + cx, y + cy, z + cz

    a = np.arange(n)
    b = (a + 1) % n
    bottom_c, top_c = 2 * n, 2 * n + 1
    i = np.concatenate([a, a, np.full(n, bottom_c), np.full(n, top_c)])
    j = np.concatenate([b, n + b, b, a + n])
    k = np.concatenate([n + b, n + a, a, b + n])
    return (x, y, z), (i, j, k)


def _make_solid(kind, params, center):
    if kind == "box":
        return _box(center, params)
    if kind == "frustum":
        bottom, top, height, shift = params
        return _frustum(center, bottom, top, height, shift)
    if kind == "cyl":
        radius, height, axis = params
        return _cylinder(center, radius, height, axis)
    raise ValueError(f"unknown solid kind: {kind}")


def _mesh(coords, faces, color, name, opacity, shiny):
    x, y, z = coords
    i, j, k = faces
    lighting = dict(ambient=0.66, diffuse=0.8, specular=0.15,
                    roughness=0.5, fresnel=0.1)
    if shiny:
        lighting = dict(ambient=0.45, diffuse=0.7, specular=0.9,
                        roughness=0.15, fresnel=0.4)
    return go.Mesh3d(
        x=x, y=y, z=z, i=i, j=j, k=k,
        color=color, opacity=opacity, name=name,
        flatshading=True, hoverinfo="name", showscale=False,
        lighting=lighting, lightposition=dict(x=110, y=180, z=210),
    )


# --- the camera --------------------------------------------------------
# X = width (right +), Y = depth (lens points -Y), Z = height (up +).
# center   : assembled position
# explode  : offset the part slides along for the exploded view
# label    : offset of the text label from the exploded position

PARTS = [
    dict(name="Body chassis", kind="box", params=(10.0, 4.2, 6.6),
         center=(0.0, 0.0, 0.0), explode=(0.0, 0.0, 0.0),
         label=(1.5, 0.0, -5.6), color=BODY, opacity=1.0),
    dict(name="Hand grip", kind="box", params=(2.4, 4.6, 6.2),
         center=(5.9, -0.5, -0.2), explode=(5.0, 0.0, 0.0),
         label=(0.0, 0.0, 4.4), color=GRIP, opacity=1.0),
    dict(name="Pentaprism hump", kind="frustum",
         params=((4.4, 3.8), (2.6, 2.6), 2.6, (0.0, 0.5)),
         center=(-1.0, 0.2, 4.5), explode=(0.0, 0.0, 3.4),
         label=(-4.3, 0.0, 1.1), color=PRISM, opacity=1.0),

    # lens group, forward along -Y
    dict(name="Lens mount ring", kind="cyl", params=(2.3, 0.5, "y"),
         center=(-1.2, -2.2, -0.2), explode=(0.0, -2.4, 0.0),
         label=(-2.5, 0.0, 3.4), color=METAL_MD, opacity=1.0),
    dict(name="Lens barrel", kind="cyl", params=(2.0, 3.0, "y"),
         center=(-1.2, -3.6, -0.2), explode=(0.0, -3.2, 0.0),
         label=(-3.8, 0.0, -2.8), color=LENS_BARREL, opacity=1.0),
    dict(name="Focus ring", kind="cyl", params=(2.1, 0.9, "y"),
         center=(-1.2, -3.0, -0.2), explode=(0.0, -6.0, 0.0),
         label=(0.0, 0.0, 3.6), color=RING_DARK, opacity=1.0),
    dict(name="Front element", kind="cyl", params=(1.6, 0.3, "y"),
         center=(-1.2, -4.8, -0.2), explode=(0.0, -6.7, 0.0),
         label=(-2.0, -1.2, 2.4), color=GLASS, opacity=0.92, shiny=True),

    # top deck, up along +Z
    dict(name="Shutter button", kind="cyl", params=(0.5, 0.45, "z"),
         center=(5.5, -1.6, 3.05), explode=(0.0, 0.0, 4.2),
         label=(2.0, -0.2, 0.3), color=ACCENT, opacity=1.0),
    dict(name="Mode dial", kind="cyl", params=(1.15, 0.8, "z"),
         center=(-3.8, 0.3, 3.6), explode=(-1.4, 0.0, 3.6),
         label=(-3.0, 0.0, 0.7), color=METAL_LT, opacity=1.0),
    dict(name="Hot shoe", kind="box", params=(1.7, 1.5, 0.55),
         center=(-1.0, 0.6, 6.0), explode=(0.0, 0.0, 5.4),
         label=(0.0, 0.0, 1.4), color=PANEL, opacity=1.0),

    # back, along +Y
    dict(name="LCD screen", kind="box", params=(6.4, 0.35, 4.4),
         center=(-0.6, 2.25, 0.1), explode=(0.0, 5.4, 0.0),
         label=(2.4, 1.2, 3.0), color=SCREEN, opacity=1.0, shiny=True),
    dict(name="Viewfinder eyepiece", kind="box", params=(1.7, 0.7, 1.2),
         center=(-1.0, 1.9, 4.6), explode=(2.8, 1.0, 2.2),
         label=(2.2, 0.0, 0.6), color=PANEL, opacity=1.0),

    # underside
    dict(name="Tripod socket", kind="cyl", params=(0.5, 0.55, "z"),
         center=(-0.5, 0.0, -3.5), explode=(-1.4, 0.0, -4.0),
         label=(0.0, 0.0, -1.9), color=METAL_MD, opacity=1.0),
    dict(name="Battery door", kind="box", params=(3.4, 3.2, 0.35),
         center=(2.2, 0.1, -3.45), explode=(2.6, 0.0, -3.4),
         label=(2.4, 0.0, -0.9), color=PANEL, opacity=1.0),
]


def build_figure():
    solids = []        # (coords, faces, part)
    leader_chains = []  # list of point lists
    anchors = []        # points
    labels = []         # (point, text)

    for part in PARTS:
        center = np.array(part["center"], dtype=float)
        placed = center + np.array(part["explode"], dtype=float)
        label_pos = placed + np.array(part["label"], dtype=float)
        exploded = float(np.linalg.norm(part["explode"])) > 1e-6

        coords, faces = _make_solid(part["kind"], part["params"], tuple(placed))
        solids.append((coords, faces, part))

        anchor = center if exploded else np.array([-5.0, 0.0, 1.8])
        leader_chains.append([anchor, placed, label_pos] if exploded
                             else [anchor, label_pos])
        anchors.append(anchor)
        labels.append((label_pos, part["name"]))

    # Centre everything on the origin so the framing does not depend on
    # hand-tuned camera offsets.
    every_x = np.concatenate(
        [np.asarray(c[0]) for c, _, _ in solids]
        + [np.array([p[0] for p in chain]) for chain in leader_chains])
    every_y = np.concatenate(
        [np.asarray(c[1]) for c, _, _ in solids]
        + [np.array([p[1] for p in chain]) for chain in leader_chains])
    every_z = np.concatenate(
        [np.asarray(c[2]) for c, _, _ in solids]
        + [np.array([p[2] for p in chain]) for chain in leader_chains])
    shift = np.array([
        (every_x.min() + every_x.max()) / 2,
        (every_y.min() + every_y.max()) / 2,
        (every_z.min() + every_z.max()) / 2,
    ])

    meshes = []
    for (xs, ys, zs), faces, part in solids:
        coords = (np.asarray(xs) - shift[0],
                  np.asarray(ys) - shift[1],
                  np.asarray(zs) - shift[2])
        meshes.append(_mesh(coords, faces, part["color"], part["name"],
                            part["opacity"], part.get("shiny", False)))

    leader_x, leader_y, leader_z = [], [], []
    for chain in leader_chains:
        for pt in chain:
            leader_x.append(pt[0] - shift[0])
            leader_y.append(pt[1] - shift[1])
            leader_z.append(pt[2] - shift[2])
        leader_x.append(None)
        leader_y.append(None)
        leader_z.append(None)

    anchor_x = [pt[0] - shift[0] for pt in anchors]
    anchor_y = [pt[1] - shift[1] for pt in anchors]
    anchor_z = [pt[2] - shift[2] for pt in anchors]

    label_x = [pt[0] - shift[0] for pt, _ in labels]
    label_y = [pt[1] - shift[1] for pt, _ in labels]
    label_z = [pt[2] - shift[2] for pt, _ in labels]
    label_text = [txt for _, txt in labels]

    leader_trace = go.Scatter3d(
        x=leader_x, y=leader_y, z=leader_z, mode="lines",
        line=dict(color=LEADER, width=3, dash="dot"),
        hoverinfo="skip", showlegend=False)
    anchor_trace = go.Scatter3d(
        x=anchor_x, y=anchor_y, z=anchor_z, mode="markers",
        marker=dict(size=3.5, color=ANCHOR),
        hoverinfo="skip", showlegend=False)
    label_trace = go.Scatter3d(
        x=label_x, y=label_y, z=label_z, mode="text", text=label_text,
        textfont=dict(size=12, color=LABEL, family="Arial, sans-serif"),
        textposition="middle center", hoverinfo="skip", showlegend=False)

    fig = go.Figure(data=meshes + [leader_trace, anchor_trace, label_trace])
    fig.update_layout(
        title=dict(text="DSLR Camera - Exploded Parts View",
                   x=0.5, y=0.9,
                   font=dict(size=20, color=LABEL, family="Arial, sans-serif")),
        paper_bgcolor="white",
        margin=dict(l=0, r=0, t=44, b=0),
        height=760,
        showlegend=False,
        scene=dict(
            xaxis=dict(visible=False),
            yaxis=dict(visible=False),
            zaxis=dict(visible=False),
            aspectmode="data",
            dragmode="orbit",
            bgcolor="white",
            camera=dict(eye=dict(x=0.88, y=-1.12, z=0.5),
                        center=dict(x=0.0, y=0.0, z=0.0),
                        up=dict(x=0.0, y=0.0, z=1.0)),
        ),
    )
    return fig


if __name__ == "__main__":
    figure = build_figure()
    out_path = os.path.join(os.path.dirname(os.path.abspath(__file__)),
                            "camera-3d.html")
    figure.write_html(out_path, include_plotlyjs=True, full_html=True,
                      config={"responsive": True})
    print(f"wrote {out_path}")
    try:
        figure.show()
    except Exception as exc:  # depends on a local browser being available
        print(f"(could not open a browser automatically: {exc})")
        print(f"open {out_path} manually")
