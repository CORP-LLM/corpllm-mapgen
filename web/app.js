'use strict';

// ── State ─────────────────────────────────────────────────────────────────────
let terrain = null;
let edgeById        = new Map();
let cellById        = new Map();
let smoothedCells   = new Map(); // cellID -> [pt, pt, ...] smoothed polygon
let smoothedEdges   = new Map(); // edgeID -> [pt, pt, ...] smoothed polyline

// Render settings (mutable — bound to UI controls).
const render_ = {
  subdiv:       6,     // points inserted per original edge
  amount:       0.12,  // perpendicular offset as fraction of edge length
  showEdges:    true,
  smoothCurves: false,
  hillshade:    true,  // simulated NW lighting on land cells
  contours:     false, // every-0.1 elevation contour lines
};
const view = { x: 0, y: 0, scale: 1 };
let dragging = false;
let dragStart = { x: 0, y: 0 };
let viewAtDrag = { x: 0, y: 0 };
let touchRef = null; // { dist, cx, cy, vx, vy, vs }

// ── Canvas ────────────────────────────────────────────────────────────────────
const canvas = document.getElementById('map-canvas');
const ctx = canvas.getContext('2d');

function resizeCanvas() {
  const wrap = document.getElementById('canvas-wrap');
  canvas.width = wrap.clientWidth;
  canvas.height = wrap.clientHeight;
  terrain ? render() : renderEmpty();
}

window.addEventListener('resize', resizeCanvas);

// ── Colors ────────────────────────────────────────────────────────────────────
const C = {
  bg:         '#050a14',
  land:       '#1b2e1b',
  landEdge:   '#203525',
  water:      '#0a1828',
  waterEdge:  '#0c1e2c',
  lake:       '#0e2238',
  lakeEdge:   '#102640',
  riverCell:  '#1a4e72',
  riverEdge:  '#1c5880',
  coast:      '#3a9ab8',
};

// Interpolate an [r,g,b] color from a list of stops.
function stopRgb(t, stops) {
  t = Math.max(stops[0].t, Math.min(stops[stops.length - 1].t, t));
  for (let i = 0; i < stops.length - 1; i++) {
    const a = stops[i], b = stops[i + 1];
    if (t >= a.t && t <= b.t) {
      const k = (t - a.t) / (b.t - a.t);
      return [
        a.r + (b.r - a.r) * k,
        a.g + (b.g - a.g) * k,
        a.b + (b.b - a.b) * k,
      ];
    }
  }
  return [stops[0].r, stops[0].g, stops[0].b];
}

function rgbToCss(rgb) {
  return `rgb(${Math.round(rgb[0])},${Math.round(rgb[1])},${Math.round(rgb[2])})`;
}

function mulRgb(rgb, k) {
  return [
    Math.max(0, Math.min(255, rgb[0] * k)),
    Math.max(0, Math.min(255, rgb[1] * k)),
    Math.max(0, Math.min(255, rgb[2] * k)),
  ];
}

function hexRgb(hex) {
  const n = parseInt(hex.slice(1), 16);
  return [(n >> 16) & 0xff, (n >> 8) & 0xff, n & 0xff];
}

// Land: dark green (low) → olive → khaki → warm peaks.
function landRgb(elev) {
  return stopRgb(elev, [
    { t: 0.00, r:  22, g:  48, b:  22 },
    { t: 0.45, r:  58, g:  72, b:  38 },
    { t: 0.80, r:  96, g:  92, b:  62 },
    { t: 1.00, r: 150, g: 138, b: 110 },
  ]);
}

// Water depth: shallow (near land) is brighter teal, deep ocean is near-black.
function waterRgb(elev) {
  const d = Math.max(0, Math.min(1, -elev));
  return stopRgb(d, [
    { t: 0.00, r: 32, g: 74, b: 96 },
    { t: 0.40, r: 16, g: 42, b: 66 },
    { t: 1.00, r:  5, g: 14, b: 28 },
  ]);
}

// ── Hillshade ────────────────────────────────────────────────────────────────
// For each land cell, average the elevation-weighted displacement vectors to
// its neighbors to get a slope vector (points uphill). Dot with the incoming
// light direction to get a shading factor ∈ roughly [-0.08, 0.08]. Applied as
// a multiplicative brightness when rendering cell fills.
const cellShade = new Map();

function computeHillshade() {
  cellShade.clear();
  if (!terrain) return;
  // Toward-source direction for NW light = pointing NW = (-, -).
  // A slope whose uphill direction points toward the light is bright.
  const sourceX = -Math.SQRT1_2, sourceY = -Math.SQRT1_2;
  for (const c of terrain.cells) {
    if (c.terrain !== 'land') {
      cellShade.set(c.id, 0);
      continue;
    }
    let sx = 0, sy = 0, n = 0;
    for (const nbId of c.neighbors) {
      const nb = cellById.get(nbId);
      if (!nb || nb.terrain !== 'land') continue; // ignore coast drop-offs
      const dx = nb.center.x - c.center.x;
      const dy = nb.center.y - c.center.y;
      const len = Math.hypot(dx, dy);
      if (len < 0.001) continue;
      const dElev = (nb.elevation || 0) - (c.elevation || 0);
      sx += (dx / len) * dElev;
      sy += (dy / len) * dElev;
      n++;
    }
    if (n > 0) { sx /= n; sy /= n; }
    // dot(slope_uphill, toward_source): + when facing light, − when in shadow.
    let shade = sx * sourceX + sy * sourceY;
    // Clamp to reasonable range so a single steep cell doesn't blow out.
    if (shade > 0.15) shade = 0.15;
    if (shade < -0.15) shade = -0.15;
    cellShade.set(c.id, shade);
  }
}

// ── Organic smoothing ─────────────────────────────────────────────────────────
// Each Voronoi edge gets subdivided with perpendicular noise offsets. Noise
// depends only on canonical-sorted edge endpoints, so adjacent cells agree on
// the shared edge — topology preserved, no gaps.

function noise2D(x, y) {
  return Math.sin(x * 0.11 + y * 0.17) * 0.55
       + Math.sin(x * 0.27 - y * 0.19) * 0.35
       + Math.sin(x * 0.43 + y * 0.51) * 0.10;
}

function subdividedEdge(a, b) {
  // Clamp AND snap endpoints to map bounds — Voronoi clipping leaves float
  // slop that shows up as 1-pixel stair-steps along the border at high zoom.
  // Any vertex within SNAP of a border is pulled onto it exactly so all
  // adjacent cells share identical border coordinates.
  const bw = terrain ? terrain.bounds.width : Infinity;
  const bh = terrain ? terrain.bounds.height : Infinity;
  const SNAP = 6.0;
  const fix = p => {
    let x = Math.max(0, Math.min(bw, p.x));
    let y = Math.max(0, Math.min(bh, p.y));
    if (x < SNAP) x = 0;
    else if (x > bw - SNAP) x = bw;
    if (y < SNAP) y = 0;
    else if (y > bh - SNAP) y = bh;
    return { x, y };
  };
  a = fix(a);
  b = fix(b);

  // Canonical order → identical result regardless of traversal direction.
  const reversed = a.x > b.x || (a.x === b.x && a.y > b.y);
  const v1 = reversed ? b : a;
  const v2 = reversed ? a : b;
  const dx = v2.x - v1.x, dy = v2.y - v1.y;
  const len = Math.hypot(dx, dy);
  // Early returns must honor the caller's direction (a → b), not the
  // canonical sorted one. Returning [b, a] here was skipping vertices in
  // adjacent cell polygons and leaving visible triangular gaps.
  if (len < 0.5 || render_.subdiv < 2) return [a, b];

  // Any endpoint on the map border → keep edge straight.
  const m = 1.5;
  const atBorderA = v1.x <= m || v1.x >= bw - m || v1.y <= m || v1.y >= bh - m;
  const atBorderB = v2.x <= m || v2.x >= bw - m || v2.y <= m || v2.y >= bh - m;
  if (atBorderA || atBorderB) return [a, b];

  const px = -dy / len, py = dx / len;
  const pts = [v1];
  for (let k = 1; k < render_.subdiv; k++) {
    const t = k / render_.subdiv;
    const mx = v1.x + dx * t;
    const my = v1.y + dy * t;
    const n = noise2D(mx, my);
    let nx = mx + px * n * render_.amount * len;
    let ny = my + py * n * render_.amount * len;
    // Clamp to map bounds so wiggle can never leak across the border.
    if (terrain) {
      if (nx < 0) nx = 0; else if (nx > bw) nx = bw;
      if (ny < 0) ny = 0; else if (ny > bh) ny = bh;
    }
    pts.push({ x: nx, y: ny });
  }
  pts.push(v2);
  if (reversed) pts.reverse();
  return pts;
}

function preprocessSmooth() {
  smoothedEdges.clear();
  smoothedCells.clear();
  for (const e of terrain.edges) {
    smoothedEdges.set(e.id, subdividedEdge(e.vertices[0], e.vertices[1]));
  }
  for (const c of terrain.cells) {
    const v = c.vertices;
    if (!v || v.length < 3) continue;
    const poly = [];
    for (let i = 0; i < v.length; i++) {
      const pts = subdividedEdge(v[i], v[(i + 1) % v.length]);
      for (let j = 0; j < pts.length - 1; j++) poly.push(pts[j]);
    }
    smoothedCells.set(c.id, poly);
  }
}

function pathPolyline(pts) {
  ctx.moveTo(pts[0].x, pts[0].y);
  for (let i = 1; i < pts.length; i++) ctx.lineTo(pts[i].x, pts[i].y);
}

// Smooth a closed polygon with quadratic curves through edge midpoints.
// Control points are the polygon vertices; the curve passes through midpoints.
// This rounds off the subdivision sawtooth into flowing curves.
function pathSmoothClosed(pts) {
  const n = pts.length;
  if (n < 3) return pathPolyline(pts);
  const m0x = (pts[0].x + pts[1].x) / 2;
  const m0y = (pts[0].y + pts[1].y) / 2;
  ctx.moveTo(m0x, m0y);
  for (let i = 1; i < n; i++) {
    const j = (i + 1) % n;
    const mx = (pts[i].x + pts[j].x) / 2;
    const my = (pts[i].y + pts[j].y) / 2;
    ctx.quadraticCurveTo(pts[i].x, pts[i].y, mx, my);
  }
  ctx.quadraticCurveTo(pts[0].x, pts[0].y, m0x, m0y);
}

// ── Render ────────────────────────────────────────────────────────────────────
function fitView() {
  const pad = 24;
  const s = Math.min(
    (canvas.width  - pad * 2) / terrain.bounds.width,
    (canvas.height - pad * 2) / terrain.bounds.height
  );
  view.scale = s;
  view.x = (canvas.width  - terrain.bounds.width  * s) / 2;
  view.y = (canvas.height - terrain.bounds.height * s) / 2;
}

function render() {
  ctx.clearRect(0, 0, canvas.width, canvas.height);
  ctx.fillStyle = C.bg;
  ctx.fillRect(0, 0, canvas.width, canvas.height);

  ctx.save();
  ctx.setTransform(view.scale, 0, 0, view.scale, view.x, view.y);

  const lakeCellSet = buildLakeSet();

  // Fill cells — smoothed polygon (linear OR bezier through midpoints).
  for (const cell of terrain.cells) {
    const poly = smoothedCells.get(cell.id);
    if (!poly || poly.length < 3) continue;
    ctx.beginPath();
    if (render_.smoothCurves) {
      pathSmoothClosed(poly);
    } else {
      pathPolyline(poly);
      ctx.closePath();
    }
    let rgb;
    if (cell.river) {
      rgb = hexRgb(C.riverCell);
    } else if (cell.terrain === 'land') {
      rgb = landRgb(cell.elevation || 0);
      if (render_.hillshade) {
        // Shade ∈ [-0.15, 0.15] after clamp; ×4 gives brightness ≈ [0.4, 1.6].
        const s = cellShade.get(cell.id) || 0;
        rgb = mulRgb(rgb, 1 + s * 4);
      }
    } else if (lakeCellSet.has(cell.id)) {
      rgb = hexRgb(C.lake);
    } else {
      rgb = waterRgb(cell.elevation || 0);
    }
    const css = rgbToCss(rgb);
    ctx.fillStyle = css;
    ctx.fill();
    // Same-color 1px stroke hides sub-pixel AA seams between adjacent cells.
    // Without this the map shows faint "outlines" even with Show-edges off.
    ctx.strokeStyle = css;
    ctx.lineWidth = 1.2 / view.scale;
    ctx.stroke();
  }

  // Cell edges — batched by type, smoothed. Skip entirely if disabled.
  if (render_.showEdges) {
    ctx.lineWidth = 0.7 / view.scale;

    // land-land (non-river)
    ctx.beginPath();
    ctx.strokeStyle = C.landEdge;
    for (const e of terrain.edges) {
      if (e.coastline || e.type !== 'land-land') continue;
      const ca = cellById.get(e.cells[0]), cb = cellById.get(e.cells[1]);
      if (ca && ca.river || cb && cb.river) continue;
      pathPolyline(smoothedEdges.get(e.id));
    }
    ctx.stroke();

    // river cell borders — draw only river↔river or river↔land.
    // River↔lake / river↔coast-water is skipped so the mouth blends seamlessly
    // into the receiving water body (both are water — no visual fence).
    ctx.beginPath();
    ctx.strokeStyle = C.riverEdge;
    for (const e of terrain.edges) {
      if (e.coastline) continue;
      const ca = cellById.get(e.cells[0]), cb = cellById.get(e.cells[1]);
      if (!ca || !cb) continue;
      const aRiver = ca.river, bRiver = cb.river;
      if (!aRiver && !bRiver) continue;
      // If one side is river and the other is non-river water → skip (blend).
      const aIsOtherWater = !aRiver && ca.terrain === 'water';
      const bIsOtherWater = !bRiver && cb.terrain === 'water';
      if (aIsOtherWater || bIsOtherWater) continue;
      pathPolyline(smoothedEdges.get(e.id));
    }
    ctx.stroke();

    // water-water (skip river cell edges — handled by river batch)
    ctx.beginPath();
    ctx.strokeStyle = C.waterEdge;
    for (const e of terrain.edges) {
      if (e.coastline || e.type !== 'water-water') continue;
      const ca = cellById.get(e.cells[0]), cb = cellById.get(e.cells[1]);
      if (ca && ca.river || cb && cb.river) continue;
      pathPolyline(smoothedEdges.get(e.id));
    }
    ctx.stroke();
  }

  // Contour lines — draw any cell-edge that straddles a 0.1 elevation level.
  // Topo map look; fine-grained detail on mountains and valleys.
  if (render_.contours) {
    ctx.strokeStyle = 'rgba(18, 14, 8, 0.55)';
    ctx.lineWidth = 0.6 / view.scale;
    const levels = [0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9];
    ctx.beginPath();
    for (const edge of terrain.edges) {
      const ca = cellById.get(edge.cells[0]);
      const cb = cellById.get(edge.cells[1]);
      if (!ca || !cb) continue;
      if (ca.terrain !== 'land' || cb.terrain !== 'land') continue;
      const ea = ca.elevation || 0, eb = cb.elevation || 0;
      for (const lv of levels) {
        if ((ea < lv) !== (eb < lv)) {
          const pts = smoothedEdges.get(edge.id);
          if (pts) pathPolyline(pts);
          break;
        }
      }
    }
    ctx.stroke();
  }

  // River border extension — for origin=border rivers, draw a short stroke
  // from the source cell's center to the nearest map edge so the river
  // visibly "enters" the map. Uses the river cell fill color so it blends
  // into the river rather than adding a highlight stripe.
  if (terrain.rivers && terrain.rivers.length > 0) {
    const bw = terrain.bounds.width, bh = terrain.bounds.height;
    const BORDER_M = 40;
    const atMapBorder = p =>
      p.x <= BORDER_M || p.x >= bw - BORDER_M || p.y <= BORDER_M || p.y >= bh - BORDER_M;
    const nearestBorder = p => {
      const opts = [
        { x: 0,  y: p.y, d: p.x },
        { x: bw, y: p.y, d: bw - p.x },
        { x: p.x, y: 0,  d: p.y },
        { x: p.x, y: bh, d: bh - p.y },
      ];
      return opts.reduce((a, b) => a.d < b.d ? a : b);
    };
    ctx.strokeStyle = C.riverCell;
    ctx.lineCap = 'round';
    for (const river of terrain.rivers) {
      const cp = river.cellPath;
      if (!cp || cp.length < 2) continue;
      const src = cellById.get(cp[0]);
      if (!src || !atMapBorder(src.center)) continue;
      const w = { narrow: 5, medium: 7, wide: 10 }[river.width] || 7;
      const entry = nearestBorder(src.center);
      ctx.lineWidth = w;
      ctx.beginPath();
      ctx.moveTo(entry.x, entry.y);
      ctx.lineTo(src.center.x, src.center.y);
      ctx.stroke();
    }
  }

  // Highways — light concrete, overlaid on top of all terrain/water.
  // Border-terminus endpoints extend to the nearest map edge so traffic
  // visibly enters/exits the map. Coastal termini (when the target side
  // is entirely water, the highway ends at the shore) do NOT extend —
  // they stop at the cell center, a natural "port" look.
  if (terrain.highways && terrain.highways.length > 0) {
    const bw = terrain.bounds.width, bh = terrain.bounds.height;
    const MARGIN = 40;
    const atBorder = p =>
      p.x <= MARGIN || p.x >= bw - MARGIN || p.y <= MARGIN || p.y >= bh - MARGIN;
    const toNearestBorder = pt => {
      const opts = [
        { x: 0,  y: pt.y, d: pt.x },
        { x: bw, y: pt.y, d: bw - pt.x },
        { x: pt.x, y: 0,  d: pt.y },
        { x: pt.x, y: bh, d: bh - pt.y },
      ];
      return opts.reduce((a, b) => a.d < b.d ? a : b);
    };
    ctx.lineCap = 'round';
    ctx.lineJoin = 'round';
    // Fixed width — highways are a 4-lane game asset, not a terrain feature
    // with varying size.
    const baseW = 2.6;
    for (const hw of terrain.highways) {
      const cp = hw.cellPath;
      if (!cp || cp.length < 2) continue;
      const first = cellById.get(cp[0]);
      const last  = cellById.get(cp[cp.length - 1]);
      if (!first || !last) continue;
      const entry = atBorder(first.center) ? toNearestBorder(first.center) : null;
      const exit  = atBorder(last.center)  ? toNearestBorder(last.center)  : null;

      const trace = () => {
        ctx.beginPath();
        if (entry) ctx.moveTo(entry.x, entry.y);
        else ctx.moveTo(first.center.x, first.center.y);
        if (entry) ctx.lineTo(first.center.x, first.center.y);
        for (let i = 1; i < cp.length; i++) {
          const c = cellById.get(cp[i]);
          if (c) ctx.lineTo(c.center.x, c.center.y);
        }
        if (exit) ctx.lineTo(exit.x, exit.y);
      };

      ctx.strokeStyle = '#1c1f26';
      ctx.lineWidth = baseW + 1.4;
      trace();
      ctx.stroke();
      ctx.strokeStyle = '#c0c8d4';
      ctx.lineWidth = baseW;
      trace();
      ctx.stroke();
    }
  }

  // Coastlines
  if (terrain.coastline && terrain.coastline.edges.length > 0) {
    ctx.lineWidth = 1.8 / view.scale;
    ctx.strokeStyle = C.coast;
    ctx.lineCap = 'round';
    ctx.lineJoin = 'round';
    ctx.beginPath();
    for (const eid of terrain.coastline.edges) {
      const pts = smoothedEdges.get(eid);
      if (!pts) continue;
      pathPolyline(pts);
    }
    ctx.stroke();
    ctx.lineCap = 'butt';
    ctx.lineJoin = 'miter';
  }

  ctx.restore();
  updateStatus();
}

function buildLakeSet() {
  const s = new Set();
  if (terrain.lakes) for (const l of terrain.lakes) for (const cid of l.cells) s.add(cid);
  return s;
}

function renderEmpty() {
  ctx.clearRect(0, 0, canvas.width, canvas.height);
  ctx.fillStyle = C.bg;
  ctx.fillRect(0, 0, canvas.width, canvas.height);
}

function updateStatus() {
  const m = terrain.meta;
  const rv = (terrain.rivers || []).length;
  const lk = (terrain.lakes  || []).length;
  el('statusbar').textContent =
    `SEED:${m.seed}  CELLS:${terrain.cells.length}  EDGES:${terrain.edges.length}  RIVERS:${rv}  LAKES:${lk}  ZOOM:${view.scale.toFixed(2)}x`;
}

// ── Pan & Zoom ────────────────────────────────────────────────────────────────
canvas.addEventListener('mousedown', e => {
  if (e.button !== 0) return;
  dragging = true;
  dragStart  = { x: e.clientX, y: e.clientY };
  viewAtDrag = { x: view.x,    y: view.y    };
  canvas.classList.add('dragging');
});

window.addEventListener('mousemove', e => {
  if (!dragging) return;
  view.x = viewAtDrag.x + (e.clientX - dragStart.x);
  view.y = viewAtDrag.y + (e.clientY - dragStart.y);
  terrain ? render() : renderEmpty();
});

window.addEventListener('mouseup', () => {
  dragging = false;
  canvas.classList.remove('dragging');
});

canvas.addEventListener('wheel', e => {
  e.preventDefault();
  zoomAt(e.offsetX, e.offsetY, e.deltaY < 0 ? 1.12 : 1 / 1.12);
  terrain ? render() : renderEmpty();
}, { passive: false });

function zoomAt(sx, sy, factor) {
  const wx = (sx - view.x) / view.scale;
  const wy = (sy - view.y) / view.scale;
  view.scale = Math.max(0.04, Math.min(80, view.scale * factor));
  view.x = sx - wx * view.scale;
  view.y = sy - wy * view.scale;
}

// Touch
canvas.addEventListener('touchstart', e => {
  e.preventDefault();
  if (e.touches.length === 1) {
    dragging  = true;
    touchRef  = null;
    dragStart  = { x: e.touches[0].clientX, y: e.touches[0].clientY };
    viewAtDrag = { x: view.x, y: view.y };
  } else if (e.touches.length === 2) {
    dragging = false;
    const dx = e.touches[0].clientX - e.touches[1].clientX;
    const dy = e.touches[0].clientY - e.touches[1].clientY;
    touchRef = {
      dist: Math.hypot(dx, dy),
      cx: (e.touches[0].clientX + e.touches[1].clientX) / 2,
      cy: (e.touches[0].clientY + e.touches[1].clientY) / 2,
      vx: view.x, vy: view.y, vs: view.scale,
    };
  }
}, { passive: false });

canvas.addEventListener('touchmove', e => {
  e.preventDefault();
  if (e.touches.length === 1 && dragging) {
    view.x = viewAtDrag.x + (e.touches[0].clientX - dragStart.x);
    view.y = viewAtDrag.y + (e.touches[0].clientY - dragStart.y);
    terrain ? render() : renderEmpty();
  } else if (e.touches.length === 2 && touchRef) {
    const dx   = e.touches[0].clientX - e.touches[1].clientX;
    const dy   = e.touches[0].clientY - e.touches[1].clientY;
    const dist = Math.hypot(dx, dy);
    const f    = dist / touchRef.dist;
    const wx   = (touchRef.cx - touchRef.vx) / touchRef.vs;
    const wy   = (touchRef.cy - touchRef.vy) / touchRef.vs;
    view.scale = Math.max(0.04, Math.min(80, touchRef.vs * f));
    view.x = touchRef.cx - wx * view.scale;
    view.y = touchRef.cy - wy * view.scale;
    terrain ? render() : renderEmpty();
  }
}, { passive: false });

canvas.addEventListener('touchend', () => { dragging = false; touchRef = null; });

// ── API ───────────────────────────────────────────────────────────────────────
async function generate() {
  setLoading(true);
  try {
    const res  = await fetch('/api/terrain/generate', {
      method:  'POST',
      headers: { 'Content-Type': 'application/json' },
      body:    JSON.stringify(readConfig()),
    });
    const data = await res.json();
    if (!res.ok) { alert('Error: ' + (data.error || res.statusText)); return; }
    loadTerrain(data.terrain);
  } catch (err) {
    alert('Request failed: ' + err.message);
  } finally {
    setLoading(false);
  }
}

function loadTerrain(t) {
  terrain   = t;
  edgeById  = new Map((t.edges || []).map(e => [e.id, e]));
  cellById  = new Map((t.cells || []).map(c => [c.id, c]));
  preprocessSmooth();
  computeHillshade();
  showEmpty(false);
  fitView();
  render();
}

// ── Config ────────────────────────────────────────────────────────────────────
function readConfig() {
  return {
    seed:            intVal('cfg-seed',   42),
    width:           intVal('cfg-width',  1000),
    height:          intVal('cfg-height', 800),
    cellCount:       intVal('cfg-cells',  1000),
    relaxIterations: intVal('cfg-relax',  3),
    terrain: {
      coastEnabled:  el('cfg-coast-en').checked,
      coastSide:     el('cfg-coast-side').value,
      coastNoise:    intVal('cfg-coast-noise',  50) / 100,
      waterRatio:    intVal('cfg-water-ratio',  25) / 100,
      roughness:     intVal('cfg-roughness',   100) / 100,
      riversEnabled:   el('cfg-rivers-en').checked,
      rivers:          readRiverList(),
      lakesEnabled:    el('cfg-lakes-en').checked,
      lakes:           readLakeList(),
      highwaysEnabled: el('cfg-highways-en').checked,
      highways:        readHighwayList(),
    },
  };
}

// ── Dynamic rivers / lakes list ──────────────────────────────────────────────
function readRiverList() {
  const rows = el('rivers-list').querySelectorAll('.dyn-row');
  return Array.from(rows).map(row => {
    const route = row.querySelector('.river-route').value.split('-');
    return {
      width:        row.querySelector('.river-width').value,
      origin:       route[0],
      end:          route[1],
      straightness: parseInt(row.querySelector('.river-straight').value, 10) / 100,
      meander:      parseInt(row.querySelector('.river-meander').value, 10) / 100,
    };
  });
}

// River style presets — set straightness/meander combos for common looks.
const RIVER_PRESETS = {
  natural:     { straight: 0,   meander: 25 },
  channelized: { straight: 70,  meander: 0  },
  concrete:    { straight: 100, meander: 0  },
  ravine:      { straight: 10,  meander: 60 },
};

function readLakeList() {
  const rows = el('lakes-list').querySelectorAll('.dyn-row');
  return Array.from(rows).map(row => ({
    size: row.querySelector('.lake-size').value,
  }));
}

function addRiverRow(width = 'medium', origin = 'border', end = 'coast',
                     straight = 0, meander = 0, preset = 'custom') {
  const tpl = el('river-row-template');
  const row = tpl.content.firstElementChild.cloneNode(true);
  row.querySelector('.river-width').value = width;
  row.querySelector('.river-route').value = `${origin}-${end}`;

  const sl = row.querySelector('.river-straight');
  const sv = row.querySelector('.river-straight-val');
  const ml = row.querySelector('.river-meander');
  const mv = row.querySelector('.river-meander-val');
  const ps = row.querySelector('.river-preset');

  const setVals = (s, m) => {
    sl.value = s; sv.textContent = s + '%';
    ml.value = m; mv.textContent = m + '%';
  };
  setVals(straight, meander);

  sl.addEventListener('input', () => { sv.textContent = sl.value + '%'; ps.value = 'custom'; });
  ml.addEventListener('input', () => { mv.textContent = ml.value + '%'; ps.value = 'custom'; });

  ps.value = preset;
  ps.addEventListener('change', () => {
    const p = RIVER_PRESETS[ps.value];
    if (p) setVals(p.straight, p.meander);
  });

  row.querySelector('.btn-remove').addEventListener('click', () => row.remove());
  el('rivers-list').appendChild(row);

  // Apply initial preset values if not custom.
  if (preset !== 'custom' && RIVER_PRESETS[preset]) {
    const p = RIVER_PRESETS[preset];
    setVals(p.straight, p.meander);
  }
}

function addLakeRow(size = 'medium') {
  const tpl = el('lake-row-template');
  const row = tpl.content.firstElementChild.cloneNode(true);
  row.querySelector('.lake-size').value = size;
  row.querySelector('.btn-remove').addEventListener('click', () => row.remove());
  el('lakes-list').appendChild(row);
}

function readHighwayList() {
  const rows = el('highways-list').querySelectorAll('.dyn-row');
  return Array.from(rows).map(row => ({
    from: row.querySelector('.highway-from').value,
    to:   row.querySelector('.highway-to').value,
  }));
}

function addHighwayRow(from = 'north', to = 'south') {
  const tpl = el('highway-row-template');
  const row = tpl.content.firstElementChild.cloneNode(true);
  row.querySelector('.highway-from').value = from;
  row.querySelector('.highway-to').value = to;
  row.querySelector('.btn-remove').addEventListener('click', () => row.remove());
  el('highways-list').appendChild(row);
}

// ── Import / Export ───────────────────────────────────────────────────────────
function exportJSON() {
  if (!terrain) { alert('No terrain loaded.'); return; }
  const blob = new Blob([JSON.stringify(terrain, null, 2)], { type: 'application/json' });
  const url  = URL.createObjectURL(blob);
  const a    = document.createElement('a');
  a.href     = url;
  a.download = `terrain_${terrain.meta.id || Date.now()}.json`;
  a.click();
  URL.revokeObjectURL(url);
}

el('file-input').addEventListener('change', e => {
  const file = e.target.files[0];
  if (!file) return;
  const reader = new FileReader();
  reader.onload = ev => {
    try { loadTerrain(JSON.parse(ev.target.result)); }
    catch (err) { alert('Invalid JSON: ' + err.message); }
  };
  reader.readAsText(file);
  e.target.value = '';
});

// ── UI helpers ────────────────────────────────────────────────────────────────
function el(id)           { return document.getElementById(id); }
function intVal(id, def)  { return parseInt(el(id).value, 10) || def; }
function setLoading(on)   { el('loading').classList.toggle('visible', on); }
function showEmpty(on)    { el('empty-state').style.display = on ? 'flex' : 'none'; }

function bindRange(id, valId, fmt) {
  const r = el(id), v = el(valId);
  const upd = () => { v.textContent = fmt ? fmt(r.value) : r.value; };
  r.addEventListener('input', upd);
  upd();
}

function bindToggle(cbId, optsId) {
  const cb = el(cbId), opts = el(optsId);
  const upd = () => { opts.style.opacity = cb.checked ? '1' : '0.35'; };
  cb.addEventListener('change', upd);
  upd();
}

// Sidebar collapse
const sidebar = document.getElementById('sidebar');
el('sidebar-toggle').addEventListener('click', () => {
  const c = sidebar.classList.toggle('collapsed');
  el('sidebar-toggle').innerHTML = c ? '&#9654;' : '&#9664;';
  setTimeout(resizeCanvas, 220);
});

el('btn-random').addEventListener('click', () => {
  el('cfg-seed').value = (Math.random() * 0x7FFFFFFF | 0);
});

el('btn-generate').addEventListener('click', generate);
el('btn-export').addEventListener('click', exportJSON);
el('btn-import').addEventListener('click', () => el('file-input').click());

el('btn-add-river').addEventListener('click', () => addRiverRow());
el('btn-add-lake').addEventListener('click', () => addLakeRow());
el('btn-add-highway').addEventListener('click', () => addHighwayRow());

// ── Init ──────────────────────────────────────────────────────────────────────
function init() {
  bindRange('cfg-cells',       'cfg-cells-val');
  bindRange('cfg-relax',       'cfg-relax-val');
  bindRange('cfg-coast-noise', 'cfg-coast-noise-val', v => (v / 100).toFixed(2));
  bindRange('cfg-water-ratio', 'cfg-water-ratio-val', v => v + '%');
  bindRange('cfg-roughness',   'cfg-roughness-val',   v => v + '%');
  bindRange('cfg-subdiv',      'cfg-subdiv-val');
  bindRange('cfg-wiggle',      'cfg-wiggle-val');

  // Render-option live bindings (re-render without regeneration).
  const reproc = () => { if (terrain) { preprocessSmooth(); render(); } };
  el('cfg-subdiv').addEventListener('input', () => {
    render_.subdiv = parseInt(el('cfg-subdiv').value, 10);
    reproc();
  });
  el('cfg-wiggle').addEventListener('input', () => {
    render_.amount = parseInt(el('cfg-wiggle').value, 10) / 100;
    reproc();
  });
  el('cfg-show-edges').addEventListener('change', () => {
    render_.showEdges = el('cfg-show-edges').checked;
    if (terrain) render();
  });
  el('cfg-smooth-curves').addEventListener('change', () => {
    render_.smoothCurves = el('cfg-smooth-curves').checked;
    if (terrain) render();
  });
  el('cfg-hillshade').addEventListener('change', () => {
    render_.hillshade = el('cfg-hillshade').checked;
    if (terrain) render();
  });
  el('cfg-contours').addEventListener('change', () => {
    render_.contours = el('cfg-contours').checked;
    if (terrain) render();
  });

  bindToggle('cfg-coast-en',    'coast-opts');
  bindToggle('cfg-rivers-en',   'rivers-opts');
  bindToggle('cfg-lakes-en',    'lakes-opts');
  bindToggle('cfg-highways-en', 'highways-opts');

  // Default river / lake configuration: 3 rivers, 5 lakes.
  for (let i = 0; i < 3; i++) addRiverRow();
  for (let i = 0; i < 5; i++) addLakeRow();

  showEmpty(true);
  resizeCanvas();
}

init();
