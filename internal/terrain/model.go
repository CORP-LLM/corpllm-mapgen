package terrain

// Point is a 2D coordinate.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Cell is a Voronoi polygon.
// Elevation is [0,1], 1 = highest. Water cells are clamped to 0.
// Rivers route strictly downhill by elevation (physical gravity model).
type Cell struct {
	ID        int     `json:"id"`
	Center    Point   `json:"center"`
	Vertices  []Point `json:"vertices"`
	Terrain   string  `json:"terrain"`
	Elevation float64 `json:"elevation"`
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

// Bounds is the map rectangle.
type Bounds struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Meta contains generation metadata.
type Meta struct {
	ID              string  `json:"id"`
	Seed            int64   `json:"seed"`
	GeneratedAt     string  `json:"generatedAt"`
	CellCount       int     `json:"cellCount"`
	RelaxIterations int     `json:"relaxIterations"`
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
	Coastline Coastline `json:"coastline"`
}
