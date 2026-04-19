package terrain

// Point is a 2D coordinate.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Cell is a Voronoi polygon.
// Elevation is [-1,1]: water cells ≤ 0, land cells > 0.
// Biome is a coarse classification the client uses to pick textures and
// asset categories (see biome.go for the full enum).
type Cell struct {
	ID        int     `json:"id"`
	Center    Point   `json:"center"`
	Vertices  []Point `json:"vertices"`
	Terrain   string  `json:"terrain"`
	Elevation float64 `json:"elevation"`
	Biome     string  `json:"biome"`
	Neighbors []int   `json:"neighbors"`
	River     bool    `json:"river"`
	Lake      bool    `json:"lake"`
	Coastline bool    `json:"coastline"`
}

// Edge is a shared border between two Voronoi cells.
type Edge struct {
	ID        int      `json:"id"`
	Cells     [2]int   `json:"cells"`
	Vertices  [2]Point `json:"vertices"`
	Type      string   `json:"type"`
	River     bool     `json:"river"`
	Coastline bool     `json:"coastline"`
}

// River is a path through cell-edges from source to mouth.
// CellPath is the matching sequence of cell IDs (len = len(Path)+1),
// useful for flow-accumulation rendering (width varies along the path).
type River struct {
	ID       int    `json:"id"`
	Path     []int  `json:"path"`
	CellPath []int  `json:"cellPath"`
	Source   Point  `json:"source"`
	Mouth    Point  `json:"mouth"`
	Width    string `json:"width"`
}

// Lake is a BFS cluster of water cells.
type Lake struct {
	ID    int   `json:"id"`
	Cells []int `json:"cells"`
	Area  int   `json:"area"`
}

// Coastline holds all coastline edge IDs.
type Coastline struct {
	Edges  []int `json:"edges"`
	Length int   `json:"length"`
}

// Highway is a cell path connecting two map borders.
// A* routed with penalties for elevation change and water crossings.
// No width — all highways are the same physical size (4-lane street
// asset in the game); they're a logical transport layer, not terrain.
type Highway struct {
	ID       int   `json:"id"`
	CellPath []int `json:"cellPath"`
	From     Point `json:"from"`
	To       Point `json:"to"`
}

// Bounds is the map rectangle.
type Bounds struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Meta contains generation metadata.
// WorldScale is meters per map unit — lets the game engine scale the
// abstract 1000×800 map coordinates into its world size (default 1 m/unit).
type Meta struct {
	ID              string  `json:"id"`
	Seed            int64   `json:"seed"`
	GeneratedAt     string  `json:"generatedAt"`
	CellCount       int     `json:"cellCount"`
	RelaxIterations int     `json:"relaxIterations"`
	WorldScale      float64 `json:"worldScale"`
	Config          *Config `json:"config"`
}

// Terrain is the full output.
type Terrain struct {
	Meta      Meta      `json:"meta"`
	Bounds    Bounds    `json:"bounds"`
	Cells     []Cell    `json:"cells"`
	Edges     []Edge    `json:"edges"`
	Rivers    []River   `json:"rivers"`
	Lakes     []Lake    `json:"lakes"`
	Highways  []Highway `json:"highways"`
	Coastline Coastline `json:"coastline"`
}
