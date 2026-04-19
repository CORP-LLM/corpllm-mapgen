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

// Interpolate a color from a list of elevation stops.
function stopColor(t, stops) {
  t = Math.max(stops[0].t, Math.min(stops[stops.length - 1].t, t));
  for (let i = 0; i < stops.length - 1; i++) {
    const a = stops[i], b = stops[i + 1];
    if (t >= a.t && t <= b.t) {
      const k = (t - a.t) / (b.t - a.t);
      const r  = Math.round(a.r + (b.r - a.r) * k);
      const g  = Math.round(a.g + (b.g - a.g) * k);
      const bl = Math.round(a.b + (b.b - a.b) * k);
      return `rgb(${r},${g},${bl})`;
    }
  }
  return `rgb(${stops[0].r},${stops[0].g},${stops[0].b})`;
}

// Land: dark green (low) → olive → khaki → warm peaks.
function landColor(elev) {
  return stopColor(elev, [
    { t: 0.00, r:  22, g:  48, b:  22 },
    { t: 0.45, r:  58, g:  72, b:  38 },
    { t: 0.80, r:  96, g:  92, b:  62 },
    { t: 1.00, r: 150, g: 138, b: 110 },
  ]);
}

// Water depth: shallow (near land) is a brighter teal, deep ocean is near-black.
// elev is negative for water: 0 = at the coast, -1 = deepest.
function waterColor(elev) {
  const d = Math.max(0, Math.min(1, -elev));
  return stopColor(d, [
    { t: 0.00, r: 32, g: 74, b: 96 },   // shallow teal
    { t: 0.40, r: 16, g: 42, b: 66 },   // mid blue
    { t: 1.00, r:  5, g: 14, b: 28 },   // deep almost-black
  ]);
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
  // Clamp endpoints to map bounds — Voronoi clipping may leave float slop.
  const bw = terrain ? terrain.bounds.width : Infinity;
  const bh = terrain ? terrain.bounds.height : Infinity;
  const clampPt = p => ({
    x: Math.max(0, Math.min(bw, p.x)),
    y: Math.max(0, Math.min(bh, p.y)),
  });
  a = clampPt(a);
  b = clampPt(b);

  // Canonical order → identical result regardless of traversal direction.
  const reversed = a.x > b.x || (a.x === b.x && a.y > b.y);
  const v1 = reversed ? b : a;
  const v2 = reversed ? a : b;
  const dx = v2.x - v1.x, dy = v2.y - v1.y;
  const len = Math.hypot(dx, dy);
  if (len < 0.5 || render_.subdiv < 2) return reversed ? [b, a] : [a, b];

  // Any endpoint on the map border → keep edge straight. Wiggling a
  // border-touching edge either pushes it past the map rectangle or (after
  // clamp) creates a ragged zigzag along the border. Clean rectangle wins.
  const m = 1.5;
  const atBorderA = v1.x <= m || v1.x >= bw - m || v1.y <= m || v1.y >= bh - m;
  const atBorderB = v2.x <= m || v2.x >= bw - m || v2.y <= m || v2.y >= bh - m;
  if (atBorderA || atBorderB) return reversed ? [b, a] : [a, b];

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
    if (cell.river) {
      ctx.fillStyle = C.riverCell;
    } else if (cell.terrain === 'land') {
      ctx.fillStyle = landColor(cell.elevation || 0);
    } else if (lakeCellSet.has(cell.id)) {
      ctx.fillStyle = C.lake;
    } else {
      ctx.fillStyle = waterColor(cell.elevation || 0);
    }
    ctx.fill();
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
      riversEnabled: el('cfg-rivers-en').checked,
      rivers:        readRiverList(),
      lakesEnabled:  el('cfg-lakes-en').checked,
      lakes:         readLakeList(),
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

// ── Init ──────────────────────────────────────────────────────────────────────
function init() {
  bindRange('cfg-cells',       'cfg-cells-val');
  bindRange('cfg-relax',       'cfg-relax-val');
  bindRange('cfg-coast-noise', 'cfg-coast-noise-val', v => (v / 100).toFixed(2));
  bindRange('cfg-water-ratio', 'cfg-water-ratio-val', v => v + '%');
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

  bindToggle('cfg-coast-en',  'coast-opts');
  bindToggle('cfg-rivers-en', 'rivers-opts');
  bindToggle('cfg-lakes-en',  'lakes-opts');

  // Default river / lake configuration: 3 rivers, 5 lakes.
  for (let i = 0; i < 3; i++) addRiverRow();
  for (let i = 0; i < 5; i++) addLakeRow();

  showEmpty(true);
  resizeCanvas();
}

init();
