package terrain

import "errors"

// RiverSpec defines one river.
// Rivers must terminate hydrologically: into coast, into a lake, or off
// the map edge — they don't just stop in the middle of land.
type RiverSpec struct {
	Width        string  `json:"width"`        // "narrow", "medium", "wide"
	Origin       string  `json:"origin"`       // "border" or "inland"
	End          string  `json:"end"`          // "coast", "lake", or "offmap"
	Straightness float64 `json:"straightness"` // 0.0–1.0 — directional bias
	Meander      float64 `json:"meander"`      // 0.0–1.0 — random path perturbation
}

// LakeSpec defines one lake's size.
type LakeSpec struct {
	Size string `json:"size"` // "small", "medium", "large"
}

// HighwaySpec defines one highway: which borders it enters / exits.
// No width — highways are a game-mechanic transport layer (4-lane
// street asset), not part of the terrain topology.
type HighwaySpec struct {
	From string `json:"from"` // "north", "south", "east", "west"
	To   string `json:"to"`   // same enum; must differ from From
}

// TerrainConfig holds feature-flag parameters.
type TerrainConfig struct {
	CoastEnabled bool    `json:"coastEnabled"`
	CoastSide    string  `json:"coastSide"`
	CoastNoise   float64 `json:"coastNoise"`
	WaterRatio   float64 `json:"waterRatio"`
	// Roughness 0.0–1.0 scales Perlin noise on top of the coast-distance
	// gradient. 0 = flat (smooth ramp only, no hills). 1 = full noise.
	Roughness       float64       `json:"roughness"`
	RiversEnabled   bool          `json:"riversEnabled"`
	Rivers          []RiverSpec   `json:"rivers"`
	LakesEnabled    bool          `json:"lakesEnabled"`
	Lakes           []LakeSpec    `json:"lakes"`
	HighwaysEnabled bool          `json:"highwaysEnabled"`
	Highways        []HighwaySpec `json:"highways"`
}

// Config is the full generation config (input JSON).
// WorldScale is meters per map unit. The map's internal coordinates (0..Width
// × 0..Height) are abstract — the game engine multiplies by this scale to
// place cells in its world. Default 1.0 (1 map unit = 1 meter).
type Config struct {
	Seed            int64         `json:"seed"`
	Width           int           `json:"width"`
	Height          int           `json:"height"`
	CellCount       int           `json:"cellCount"`
	RelaxIterations int           `json:"relaxIterations"`
	WorldScale      float64       `json:"worldScale"`
	Terrain         TerrainConfig `json:"terrain"`
}

// Defaults fills zero-value fields with sensible defaults.
func (c *Config) Defaults() {
	if c.Width == 0 {
		c.Width = 1000
	}
	if c.Height == 0 {
		c.Height = 800
	}
	if c.CellCount == 0 {
		c.CellCount = 1000
	}
	if c.WorldScale <= 0 {
		c.WorldScale = 1.0
	}
	if c.Terrain.CoastSide == "" {
		c.Terrain.CoastSide = "south"
	}
	if c.Terrain.Roughness <= 0 {
		c.Terrain.Roughness = 1.0 // missing → default hilly
	}
	for i := range c.Terrain.Rivers {
		r := &c.Terrain.Rivers[i]
		if r.Width == "" {
			r.Width = "medium"
		}
		if r.Origin == "" {
			r.Origin = "border"
		}
		if r.End == "" {
			r.End = "coast"
		}
	}
	for i := range c.Terrain.Lakes {
		if c.Terrain.Lakes[i].Size == "" {
			c.Terrain.Lakes[i].Size = "medium"
		}
	}
	for i := range c.Terrain.Highways {
		h := &c.Terrain.Highways[i]
		if h.From == "" {
			h.From = "north"
		}
		if h.To == "" {
			h.To = "south"
		}
	}
}

// Validate checks parameter ranges.
func (c *Config) Validate() error {
	if c.Width < 100 || c.Width > 10000 {
		return errors.New("width must be 100–10000")
	}
	if c.Height < 100 || c.Height > 10000 {
		return errors.New("height must be 100–10000")
	}
	if c.CellCount < 50 || c.CellCount > 5000 {
		return errors.New("cellCount must be 50–5000")
	}
	if c.RelaxIterations < 0 || c.RelaxIterations > 10 {
		return errors.New("relaxIterations must be 0–10")
	}
	if c.WorldScale < 0.001 || c.WorldScale > 10000 {
		return errors.New("worldScale must be 0.001–10000 (meters per map unit)")
	}
	t := c.Terrain
	validSides := map[string]bool{"north": true, "south": true, "east": true, "west": true, "none": true}
	if !validSides[t.CoastSide] {
		return errors.New("coastSide must be north/south/east/west/none")
	}
	if t.CoastNoise < 0 || t.CoastNoise > 1 {
		return errors.New("coastNoise must be 0.0–1.0")
	}
	if t.WaterRatio < 0 || t.WaterRatio > 1 {
		return errors.New("waterRatio must be 0.0–1.0")
	}
	if t.Roughness < 0 || t.Roughness > 1 {
		return errors.New("roughness must be 0.0–1.0")
	}
	if len(t.Rivers) > 20 {
		return errors.New("at most 20 rivers allowed")
	}
	validWidths := map[string]bool{"narrow": true, "medium": true, "wide": true}
	validOrigins := map[string]bool{"border": true, "inland": true}
	validEnds := map[string]bool{"coast": true, "lake": true, "offmap": true}
	for _, r := range t.Rivers {
		if r.Width != "" && !validWidths[r.Width] {
			return errors.New("river width must be narrow/medium/wide")
		}
		if r.Origin != "" && !validOrigins[r.Origin] {
			return errors.New("river origin must be border/inland")
		}
		if r.End != "" && !validEnds[r.End] {
			return errors.New("river end must be coast/lake/offmap")
		}
		if r.Straightness < 0 || r.Straightness > 1 {
			return errors.New("river straightness must be 0.0–1.0")
		}
		if r.Meander < 0 || r.Meander > 1 {
			return errors.New("river meander must be 0.0–1.0")
		}
	}
	if len(t.Lakes) > 20 {
		return errors.New("at most 20 lakes allowed")
	}
	validSizes := map[string]bool{"small": true, "medium": true, "large": true}
	for _, l := range t.Lakes {
		if l.Size != "" && !validSizes[l.Size] {
			return errors.New("lake size must be small/medium/large")
		}
	}
	if len(t.Highways) > 15 {
		return errors.New("at most 15 highways allowed")
	}
	validSides4 := map[string]bool{"north": true, "south": true, "east": true, "west": true}
	for _, hw := range t.Highways {
		if hw.From != "" && !validSides4[hw.From] {
			return errors.New("highway from must be north/south/east/west")
		}
		if hw.To != "" && !validSides4[hw.To] {
			return errors.New("highway to must be north/south/east/west")
		}
		if hw.From != "" && hw.To != "" && hw.From == hw.To {
			return errors.New("highway from and to must differ")
		}
	}
	return nil
}
