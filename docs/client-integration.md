# Client Integration — Findings from First Babylon Pass

Date: 2026-04-19
Demo: `/babylon.html` (served alongside the 2D editor)

## Status

Our terrain JSON renders in Babylon.js with **zero adapters**. The demo at
`web/babylon-demo.js` fetches `/api/terrain/generate` and produces a
recognizable 3D map using only:
- `cell.vertices` → mesh triangles (fan-triangulated)
- `cell.elevation` → height offset
- `cell.biome` → color palette (9-value enum)
- `river.curve` / `highway.curve` → tube meshes
- `meta.bounds.width/height` → camera framing

No schema extensions needed for a viable first render.

## Architectural Context

The `corpllm-client` already has a map pipeline (`src/map/types.ts` +
`src/screens/map.ts`) that consumes a **`CityGraph`** JSON from
`corpllm-server/engine/cmd/mapgen`. That's urban layout (Sectors, Nodes,
Edges, Buildings) — a different abstraction than our Voronoi terrain.

**Our mapgen sits beneath the city layer.** Two integration paths:

1. **Terrain as backdrop** — our Voronoi map is the landscape the `CityGraph`
   sits on. Both coexist: client loads both, renders terrain underneath
   then overlays sectors/buildings. The `CityGraph` just needs its
   `global_x/y` normalized coordinates mapped into the Voronoi map's
   coordinate system.

2. **Terrain replaces CityGraph** — we extend our output to include sectors
   and buildings (much bigger scope). Not recommended.

Path 1 is clean. Our job: provide a good terrain layer the existing client
pipeline can render underneath its city data.

## What Worked

| Feature              | Observation |
|----------------------|-------------|
| Cell geometry        | `vertices` + fan-triangulation → clean mesh, no issues |
| Biome colors         | 9-value enum maps 1:1 to texture/asset categories |
| Elevation            | Visible relief; mountains vs plains obvious |
| Rivers               | Catmull-Rom curves → `B.MeshBuilder.CreateTube` renders perfectly smooth |
| Highways             | Same tube pattern, different material — reads clearly |
| Water cells          | Rendered with slight alpha + higher specular; looks natural |
| Bathymetry           | Negative elevation for water cells gives visible depth |
| JSON Schema          | All fields documented; client can `ajv`-validate |

## Gaps Identified

### Priority 1 — Render quality

1. **Cells are flat-topped, visibly stepped.** Each Voronoi cell is a
   prism at `elevation × SCALE`. Adjacent cells at different elevations
   show hard vertical walls. **Fix**: add per-vertex elevation to the
   response (or compute client-side by averaging cell elevations at each
   shared vertex).

2. **Rivers/highway tubes start at cell centers.** Visible floating start
   when the source cell is an interior cell. **Fix**: extend the curve
   array itself server-side with the border extension point for
   `origin=border` rivers and border-to-border highways. Currently the
   frontend does this at render time; document or move into the API.

3. **No lake surface mesh.** Lake cells are filled at `elevation=0` but
   a proper water surface with alpha + animated normal map would sell it.
   **Fix (client-side)**: no schema change; client creates a single flat
   plane per lake at Y=0 with water shader, driven by `Lake.cells`.

### Priority 2 — Placement hints

4. **No city-placement metadata.** For each land cell, the client-side
   urban generator (CityGraph) needs to know where to place sectors. Our
   terrain could provide a **`suitability`** score per cell:
   - flat + near water + not in mountain → good for residential/commercial
   - along highway → good for industrial/commercial
   - high elevation + isolated → good for government/restricted
   - Could ship as `cell.cityScore: number` ∈ [0,1] or a `cell.cityHint:
     'settlement' | 'industrial' | 'restricted' | 'wilderness'`.

5. **No vegetation density.** Forests aren't in our biome list. A
   `cell.vegetation: number` ∈ [0,1] driven by Perlin + biome would
   let the client drop tree assets.

### Priority 3 — Stability

6. **Cell IDs are stable per seed**, but there's no **version** field
   in meta. If we later change the Voronoi algorithm, clients can't
   detect backward-incompat output. **Fix**: add `meta.schemaVersion`
   (start at `"1.0.0"`) to the Meta struct.

7. **Regeneration churns all IDs.** User changes config → everything
   renumbers. The client needs stable references for saves/sessions.
   Not a blocker if regeneration is editor-time only (baked output).
   **Fix (if live-regen becomes a use case)**: content-addressed IDs
   from cell center + seed so stable across generations.

### Priority 4 — Nice to have

8. **Coastline stitching.** Our coastline is a list of edges; the client
   has to trace them into continuous polylines. A pre-stitched
   `coastline.polylines: Point[][]` would save work.

9. **Biome palette.** We declare the enum but not the "official"
   colors/textures. A sibling `schema/biome-palette.json` with
   recommended RGB + asset-category names would let multiple consumers
   stay visually consistent.

## Recommended Next Steps

1. **`meta.schemaVersion`** — cheap, adds forward-compat. Do it now.
2. **Per-vertex elevation** — either expose in schema or document the
   averaging client-side recipe.
3. **`cell.suitability`** or `cell.cityHint` — biggest gameplay unlock;
   lets the CityGraph generator sit on top.
4. **Border-extended curves server-side** — tidies up the API contract
   and removes client-side special-casing.

## Demo Controls

- `/` — the 2D editor (main dev UI)
- `/babylon.html` — 3D preview (this integration test)
- Mouse drag — orbit
- Scroll — zoom
- `Regenerate` button — new random seed
