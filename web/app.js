'use strict';

// ── State ─────────────────────────────────────────────────────────────────────
let terrain = null;
let edgeById = new Map();
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
  bg:        '#050a14',
  land:      '#1b2e1b',
  landEdge:  '#203525',
  water:     '#0a1828',
  waterEdge: '#0c1e2c',
  lake:      '#0e2238',
  lakeEdge:  '#102640',
  coast:     '#3a9ab8',
  river:     '#1e6a9e',
};

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

  // Fill cells
  for (const cell of terrain.cells) {
    const v = cell.vertices;
    if (!v || v.length < 3) continue;
    ctx.beginPath();
    ctx.moveTo(v[0].x, v[0].y);
    for (let i = 1; i < v.length; i++) ctx.lineTo(v[i].x, v[i].y);
    ctx.closePath();
    ctx.fillStyle = cell.terrain === 'land'
      ? C.land
      : (lakeCellSet.has(cell.id) ? C.lake : C.water);
    ctx.fill();
  }

  // Cell edges — batched by type
  const lw = 0.7 / view.scale;
  ctx.lineWidth = lw;

  ctx.beginPath();
  ctx.strokeStyle = C.landEdge;
  for (const e of terrain.edges) {
    if (e.coastline || e.type !== 'land-land') continue;
    ctx.moveTo(e.vertices[0].x, e.vertices[0].y);
    ctx.lineTo(e.vertices[1].x, e.vertices[1].y);
  }
  ctx.stroke();

  ctx.beginPath();
  ctx.strokeStyle = C.waterEdge;
  for (const e of terrain.edges) {
    if (e.coastline || e.type !== 'water-water') continue;
    ctx.moveTo(e.vertices[0].x, e.vertices[0].y);
    ctx.lineTo(e.vertices[1].x, e.vertices[1].y);
  }
  ctx.stroke();

  // Coastlines
  if (terrain.coastline && terrain.coastline.edges.length > 0) {
    ctx.lineWidth = 1.8 / view.scale;
    ctx.strokeStyle = C.coast;
    ctx.beginPath();
    for (const eid of terrain.coastline.edges) {
      const e = edgeById.get(eid);
      if (!e) continue;
      ctx.moveTo(e.vertices[0].x, e.vertices[0].y);
      ctx.lineTo(e.vertices[1].x, e.vertices[1].y);
    }
    ctx.stroke();
  }

  // Rivers
  if (terrain.rivers && terrain.rivers.length > 0) {
    ctx.strokeStyle = C.river;
    ctx.lineCap = 'round';
    ctx.lineJoin = 'round';
    for (const river of terrain.rivers) {
      if (!river.path || river.path.length === 0) continue;
      const rw = { wide: 4, medium: 2.5, narrow: 1.5 }[river.width] || 2.5;
      ctx.lineWidth = Math.max(0.8 / view.scale, rw * 0.45);
      ctx.beginPath();
      let lastPt = null;
      for (const eid of river.path) {
        const e = edgeById.get(eid);
        if (!e) continue;
        const [a, b] = e.vertices;
        if (!lastPt) {
          ctx.moveTo(a.x, a.y);
          ctx.lineTo(b.x, b.y);
          lastPt = b;
        } else {
          const da = Math.hypot(a.x - lastPt.x, a.y - lastPt.y);
          if (da < 1.0) {
            ctx.lineTo(b.x, b.y);
            lastPt = b;
          } else {
            ctx.lineTo(a.x, a.y);
            lastPt = a;
          }
        }
      }
      ctx.stroke();
    }
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
  showEmpty(false);
  fitView();
  render();
}

// ── Config ────────────────────────────────────────────────────────────────────
function readConfig() {
  const coastEn  = el('cfg-coast-en').checked;
  const riversEn = el('cfg-rivers-en').checked;
  const lakesEn  = el('cfg-lakes-en').checked;
  return {
    seed:            intVal('cfg-seed',   42),
    width:           intVal('cfg-width',  1000),
    height:          intVal('cfg-height', 800),
    cellCount:       intVal('cfg-cells',  500),
    relaxIterations: intVal('cfg-relax',  3),
    terrain: {
      coastEnabled:  coastEn,
      coastSide:     el('cfg-coast-side').value,
      coastNoise:    intVal('cfg-coast-noise',  50) / 100,
      waterRatio:    intVal('cfg-water-ratio',  25) / 100,
      riversEnabled: riversEn,
      riverCount:    intVal('cfg-river-count',  3),
      riverWidth:    el('cfg-river-width').value,
      lakesEnabled:  lakesEn,
      lakeCount:     intVal('cfg-lake-count',   5),
      lakeSize:      el('cfg-lake-size').value,
    },
  };
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

// ── Init ──────────────────────────────────────────────────────────────────────
function init() {
  bindRange('cfg-cells',       'cfg-cells-val');
  bindRange('cfg-relax',       'cfg-relax-val');
  bindRange('cfg-coast-noise', 'cfg-coast-noise-val', v => (v / 100).toFixed(2));
  bindRange('cfg-water-ratio', 'cfg-water-ratio-val', v => v + '%');
  bindRange('cfg-river-count', 'cfg-river-count-val');
  bindRange('cfg-lake-count',  'cfg-lake-count-val');

  bindToggle('cfg-coast-en',  'coast-opts');
  bindToggle('cfg-rivers-en', 'rivers-opts');
  bindToggle('cfg-lakes-en',  'lakes-opts');

  showEmpty(true);
  resizeCanvas();
}

init();
