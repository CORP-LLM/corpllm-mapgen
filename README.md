# mapgen

Procedural terrain generator for CORP//LLM. Voronoi cells, coast with
noise, downhill-routed rivers, BFS lakes, A*-routed highways, per-cell
biomes, per-vertex elevation for smooth mesh rendering, and a city
placement suitability score consumed by the CityGraph layer.

Output is a JSON document validated against
[`schema/terrain.schema.json`](schema/terrain.schema.json) (JSON Schema
2020-12). The `meta.schemaVersion` field is semver-tracked so clients
can refuse unexpected versions.

## Quick start

```bash
# Browser editor at http://localhost:8080/
# 3D preview at http://localhost:8080/babylon.html
go run ./cmd/mapgen

# Headless generation — reads config on stdin, emits terrain on stdout
echo '{"seed":42}' | go run ./cmd/mapgen generate > map.json

# Copy the schema locally
go run ./cmd/mapgen schema > terrain.schema.json
```

## HTTP API

| Endpoint                                | Method | Purpose                                              |
|-----------------------------------------|--------|------------------------------------------------------|
| `/api/terrain/generate`                 | POST   | Generate a map. Body = `Config` JSON. Returns `{id, terrain}`. |
| `/api/terrain/{id}`                     | GET    | Fetch a previously-generated map by ID.              |
| `/api/terrain/{id}/regenerate`          | POST   | Regenerate with a new config, keeping the same ID.   |
| `/api/schema/terrain`                   | GET    | JSON Schema for the response payload.                |

All endpoints return JSON; errors come back as `{"error": "<message>"}` with
an appropriate HTTP status. CORS is wide-open for local development.

## Data model (high-level)

```
Terrain
├── meta          id, seed, schemaVersion, worldScale (m/unit), config echo
├── bounds        width, height (integer world units)
├── cells []      Voronoi cells — id, center, vertices, vertexElevations[],
│                 terrain ("land"|"water"), elevation ∈ [-1,1], biome,
│                 suitability ∈ [0,1], neighbors, river, lake, coastline
├── edges []      shared borders — id, cells, vertices, type, river, coastline
├── rivers []     id, path (edge ids), cellPath (cell ids),
│                 curve (densified Catmull-Rom spline through cell centers),
│                 source, mouth, width
├── lakes []      id, cells, area
├── highways []   id, cellPath, curve (smooth spline), from, to
└── coastline     edges[] (ocean shorelines only; lake/river borders excluded)
```

### Biomes (`cell.biome`)

One of nine values, used by the client to pick textures/assets:

- Water: `ocean` · `coast` · `lake` · `river`
- Land:  `beach` · `grassland` · `hills` · `mountain` · `peak`

### Elevation

- `cell.elevation` ∈ `[-1, 1]` — height at cell center. Land > 0, water ≤ 0
  (depth increases toward -1).
- `cell.vertexElevations[]` — parallel to `vertices`. Each entry is the
  average elevation across all cells sharing that Voronoi vertex. Use
  these for smooth mesh rendering (no stepped flat-top cells).

### Suitability

`cell.suitability` ∈ `[0, 1]` — CityGraph generator uses this to decide
sector placement. 0 = unbuildable (water, peaks). 1 = prime city land
(flat grassland near water and a highway). Derived from biome +
neighbor-water + neighbor-highway + slope.

### Rivers & highways

Both structures carry `cellPath []int` (cell IDs in order) and `curve
[]Point` (densified Catmull-Rom samples, 4 per segment). The curve
already extends to the map border at endpoints that conceptually leave
the map (`river.origin="border"`, all highway ends adjacent to a
border). Render the curve directly as a tube/mesh.

Rivers have `width ∈ {narrow, medium, wide}`; highways have no width
(all the same 4-lane street asset in-game).

## CLI

```
mapgen                 start HTTP server + browser editor (default)
mapgen serve           same as above
mapgen generate        read config JSON on stdin, emit terrain JSON on stdout
mapgen schema          emit the JSON Schema for the terrain response
mapgen help            print usage
```

Environment:
- `PORT` — HTTP port for serve mode (default `8080`)

## Integration with corpllm-client

See [`docs/client-integration.md`](docs/client-integration.md) for the
first-pass Babylon.js integration test and findings. The 3D preview at
`/babylon.html` shows how to consume the JSON with no adapters — per-vertex
elevations drive the mesh, spline curves drive rivers/highways as tubes.

The existing `corpllm-client` map pipeline consumes a `CityGraph`
(urban layout) from `corpllm-server/engine/cmd/mapgen` — our terrain
sits BENEATH that. The two layers coexist; the client's CityGraph
generator uses our `cell.suitability` to decide sector placement.

## Development

```bash
# All tests (coverage + validation + invariants)
go test ./...

# Stress (10× run catches flaky invariant checks)
go test ./... -count=10

# Coverage
go test ./... -cover
```

Test inventory includes:
- **Invariants** — cell bounds, edge types, neighbor symmetry, river
  connectivity, lake isolation, coastline correctness
- **Randomized** — 20 random seeds verifying every invariant holds
- **Hydrology** — pit-fill guarantees downhill paths, rivers terminate
  at water or map border
- **Schema drift** — every generated terrain validates against
  `schema/terrain.schema.json`

## License

See repository. CORP//LLM internal.
