'use strict';

// Minimal Babylon.js consumer for the mapgen /api/terrain/generate response.
// Goal: prove the API shape is sufficient for a 3D renderer and highlight gaps.
// NO asset-loading, buildings, or post-processing — just geometry from the
// JSON, styled by biome + elevation + spline curves.

const B = BABYLON;
const canvas = document.getElementById('canvas');
const engine = new B.Engine(canvas, true, { preserveDrawingBuffer: true });
const scene  = new B.Scene(engine);
scene.clearColor = new B.Color4(0.02, 0.03, 0.05, 1);
scene.fogMode    = B.Scene.FOGMODE_EXP2;
scene.fogDensity = 0.0008;
scene.fogColor   = new B.Color3(0.03, 0.04, 0.07);

// Camera: orbit above the terrain. Radius & target set dynamically after load.
const camera = new B.ArcRotateCamera(
  'cam', -Math.PI / 3, Math.PI / 3.2, 1200,
  new B.Vector3(500, 0, 400), scene,
);
camera.attachControl(canvas, true);
camera.lowerBetaLimit  = 0.1;
camera.upperBetaLimit  = Math.PI / 2.05;
camera.wheelDeltaPercentage = 0.02;
camera.panningSensibility   = 80;

// Lighting: hemispheric ambient + directional sun from NW (matches our
// frontend hillshade so mountains look consistent across both renderers).
new B.HemisphericLight('hemi', new B.Vector3(0, 1, 0.2), scene).intensity = 0.55;
const sun = new B.DirectionalLight('sun', new B.Vector3(0.7, -1, 0.7), scene);
sun.intensity = 0.95;
sun.diffuse   = new B.Color3(1.0, 0.96, 0.88);

// ── Biome → color map ────────────────────────────────────────────────────────
// Mirrors the 9-value enum in schema/terrain.schema.json. Client integrations
// would swap these out for textures/GLBs per biome; we use flat colors here.
const BIOME_COLORS = {
  ocean:     [0.02, 0.05, 0.11],
  coast:     [0.12, 0.29, 0.37],
  lake:      [0.05, 0.13, 0.22],
  river:     [0.10, 0.30, 0.45],
  beach:     [0.80, 0.72, 0.45],
  grassland: [0.22, 0.36, 0.18],
  hills:     [0.33, 0.38, 0.19],
  mountain:  [0.42, 0.40, 0.28],
  peak:      [0.72, 0.69, 0.58],
};

// Vertical scale: map elevation [0..1] to Babylon height in world units.
// Our map is 1000×800; 100 feels proportional (10% of map width).
const HEIGHT_SCALE = 100;
// Negative elevation (water depth) gets compressed — visual hint only.
const DEPTH_SCALE  = 20;

// ── Mesh builders ────────────────────────────────────────────────────────────

// One mesh per biome — all cells of the same biome merged into a single
// indexed mesh for performance. Each cell becomes an extruded prism with
// top at Y = elevation × HEIGHT_SCALE and bottom at -5 (a floor so water
// doesn't z-fight with deep cells).

function buildCellMeshes(terrain) {
  const byBiome = new Map();
  for (const cell of terrain.cells) {
    if (!cell.vertices || cell.vertices.length < 3) continue;
    const arr = byBiome.get(cell.biome) || [];
    arr.push(cell);
    byBiome.set(cell.biome, arr);
  }
  const meshes = [];
  for (const [biome, cells] of byBiome) {
    const mesh = buildBiomeMesh(biome, cells);
    if (mesh) meshes.push(mesh);
  }
  return meshes;
}

function buildBiomeMesh(biome, cells) {
  const positions = [];
  const indices   = [];
  const normals   = [];
  const colors    = [];
  const color = BIOME_COLORS[biome] || [0.4, 0.4, 0.4];
  const r = color[0], g = color[1], b = color[2];

  for (const cell of cells) {
    const cellElev = cell.elevation || 0;
    const base = positions.length / 3;
    const verts = cell.vertices;
    const vElevs = cell.vertexElevations || [];
    // Per-vertex elevation gives smooth meshes that match across cell
    // boundaries (no stepped flat tops). Fall back to center elevation
    // if the field is missing on older servers.
    for (let i = 0; i < verts.length; i++) {
      const v = verts[i];
      const e = vElevs[i] != null ? vElevs[i] : cellElev;
      const y = e >= 0 ? e * HEIGHT_SCALE : e * DEPTH_SCALE;
      positions.push(v.x, y, v.y);
      normals.push(0, 1, 0);
      colors.push(r, g, b, 1);
    }
    for (let i = 1; i < verts.length - 1; i++) {
      indices.push(base, base + i, base + i + 1);
    }
  }

  if (positions.length === 0) return null;

  const mesh = new B.Mesh(`biome_${biome}`, scene);
  const vd = new B.VertexData();
  vd.positions = positions;
  vd.indices   = indices;
  vd.normals   = normals;
  vd.colors    = colors;
  vd.applyToMesh(mesh);

  const mat = new B.StandardMaterial(`mat_${biome}`, scene);
  mat.diffuseColor  = new B.Color3(r, g, b);
  mat.specularColor = new B.Color3(0.05, 0.05, 0.08);
  mat.backFaceCulling = false;
  // Water biomes get a hint of specularity; land stays matte.
  if (biome === 'ocean' || biome === 'lake' || biome === 'river' || biome === 'coast') {
    mat.specularColor = new B.Color3(0.25, 0.25, 0.35);
    mat.alpha = 0.92;
  }
  mesh.material = mat;
  return mesh;
}

// ── Rivers & highways as tubes along their spline curves ─────────────────────

function buildRivers(terrain) {
  const tubes = [];
  for (const river of terrain.rivers || []) {
    const curve = (river.curve || []).map(p => new B.Vector3(p.x, 3, p.y));
    if (curve.length < 2) continue;
    const radius = { narrow: 2, medium: 3.5, wide: 6 }[river.width] || 3.5;
    const tube = B.MeshBuilder.CreateTube(`river_${river.id}`, {
      path: curve, radius, cap: B.Mesh.CAP_ALL, updatable: false,
    }, scene);
    const mat = new B.StandardMaterial(`river_mat_${river.id}`, scene);
    mat.diffuseColor  = new B.Color3(0.18, 0.44, 0.62);
    mat.specularColor = new B.Color3(0.3, 0.3, 0.4);
    tube.material = mat;
    tubes.push(tube);
  }
  return tubes;
}

function buildHighways(terrain) {
  const tubes = [];
  for (const hw of terrain.highways || []) {
    const curve = (hw.curve || []).map(p => new B.Vector3(p.x, 2.5, p.y));
    if (curve.length < 2) continue;
    const tube = B.MeshBuilder.CreateTube(`hw_${hw.id}`, {
      path: curve, radius: 4, cap: B.Mesh.CAP_ALL,
    }, scene);
    const mat = new B.StandardMaterial(`hw_mat_${hw.id}`, scene);
    mat.diffuseColor  = new B.Color3(0.75, 0.78, 0.82);
    mat.specularColor = new B.Color3(0.15, 0.15, 0.18);
    tube.material = mat;
    tubes.push(tube);
  }
  return tubes;
}

// ── Clear + load cycle ───────────────────────────────────────────────────────

let currentMeshes = [];

function clearScene() {
  for (const m of currentMeshes) m.dispose();
  currentMeshes = [];
}

function framCamera(terrain) {
  const w = terrain.bounds.width, h = terrain.bounds.height;
  camera.target = new B.Vector3(w / 2, 0, h / 2);
  camera.radius = Math.max(w, h) * 1.1;
  camera.lowerRadiusLimit = 50;
  camera.upperRadiusLimit = Math.max(w, h) * 3;
}

async function loadTerrain() {
  const status = document.getElementById('status');
  status.textContent = 'Fetching /api/terrain/generate…';
  status.className = 'stat';
  const body = {
    seed: Date.now() & 0x7fffffff,
    width: 1000, height: 800, cellCount: 1500, relaxIterations: 3,
    worldScale: 1.0,
    terrain: {
      coastEnabled: true, coastSide: 'south', coastNoise: 0.5,
      waterRatio: 0.25, roughness: 1.0,
      riversEnabled: true,
      rivers: [
        { width: 'wide',   origin: 'border', end: 'coast', straightness: 0,   meander: 0.3 },
        { width: 'medium', origin: 'border', end: 'coast', straightness: 0.2, meander: 0.2 },
      ],
      lakesEnabled: true,
      lakes: [{ size: 'medium' }, { size: 'small' }],
      highwaysEnabled: true,
      highways: [
        { from: 'north', to: 'south' },
        { from: 'west',  to: 'east'  },
      ],
    },
  };
  let terrain;
  try {
    const res = await fetch('/api/terrain/generate', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    const json = await res.json();
    if (!res.ok) throw new Error(json.error || res.statusText);
    terrain = json.terrain;
  } catch (e) {
    status.textContent = 'Load failed: ' + e.message;
    status.className = 'warn';
    return;
  }

  clearScene();
  const cellMeshes = buildCellMeshes(terrain);
  const rivers     = buildRivers(terrain);
  const highways   = buildHighways(terrain);
  currentMeshes = [...cellMeshes, ...rivers, ...highways];
  framCamera(terrain);

  const biomeCounts = {};
  for (const c of terrain.cells) biomeCounts[c.biome] = (biomeCounts[c.biome] || 0) + 1;
  const biomeLine = Object.entries(biomeCounts)
    .sort((a, b) => b[1] - a[1])
    .map(([k, v]) => `${k}=${v}`).join(' · ');

  status.innerHTML =
    `<span class="ok">Loaded seed ${terrain.meta.seed}</span><br>` +
    `cells: ${terrain.cells.length} · rivers: ${(terrain.rivers||[]).length} · ` +
    `lakes: ${(terrain.lakes||[]).length} · highways: ${(terrain.highways||[]).length}<br>` +
    `worldScale: ${terrain.meta.worldScale} m/unit<br>` +
    `biomes: ${biomeLine}`;
}

document.getElementById('btn-regen').addEventListener('click', loadTerrain);
document.getElementById('btn-editor').addEventListener('click', () => {
  window.location.href = '/';
});

engine.runRenderLoop(() => scene.render());
window.addEventListener('resize', () => engine.resize());

loadTerrain();
