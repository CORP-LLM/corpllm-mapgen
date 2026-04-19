package terrain

import "errors"

// RiverSpec defines one river: its visual width and routing endpoints.
type RiverSpec struct {
	Width  string `json:"width"`  // "narrow", "medium", "wide"
	Origin string `json:"origin"` // "border" or "inland"
	End    string `json:"end"`    // "coast" or "inland"
}

// LakeSpec defines one lake's size.
type LakeSpec struct {
	Size string `json:"size"` // "small", "medium", "large"
}

// TerrainConfig holds feature-flag parameters.
type TerrainConfig struct {
	CoastEnabled  bool        `json:"coastEnabled"`
	CoastSide     string      `json:"coastSide"`
	CoastNoise    float64     `json:"coastNoise"`
	WaterRatio    float64     `json:"waterRatio"`
	RiversEnabled bool        `json:"riversEnabled"`
	Rivers        []RiverSpec `json:"rivers"`
	LakesEnabled  bool        `json:"lakesEnabled"`
	Lakes         []LakeSpec  `json:"lakes"`
}

// Config is the full generation config (input JSON).
type Config struct {
	Seed            int64         `json:"seed"`
	Width           int           `json:"width"`
	Height          int           `json:"height"`
	CellCount       int           `json:"cellCount"`
	RelaxIterations int           `json:"relaxIterations"`
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
	if c.Terrain.CoastSide == "" {
		c.Terrain.CoastSide = "south"
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
	if len(t.Rivers) > 20 {
		return errors.New("at most 20 rivers allowed")
	}
	validWidths := map[string]bool{"narrow": true, "medium": true, "wide": true}
	validOrigins := map[string]bool{"border": true, "inland": true}
	validEnds := map[string]bool{"coast": true, "inland": true}
	for i, r := range t.Rivers {
		if r.Width != "" && !validWidths[r.Width] {
			return errors.New("river width must be narrow/medium/wide")
		}
		if r.Origin != "" && !validOrigins[r.Origin] {
			return errors.New("river origin must be border/inland")
		}
		if r.End != "" && !validEnds[r.End] {
			return errors.New("river end must be coast/inland")
		}
		_ = i
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
	return nil
}
