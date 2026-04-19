package terrain

import "errors"

// TerrainConfig holds feature-flag parameters.
type TerrainConfig struct {
	CoastEnabled  bool    `json:"coastEnabled"`
	CoastSide     string  `json:"coastSide"`
	CoastNoise    float64 `json:"coastNoise"`
	WaterRatio    float64 `json:"waterRatio"`
	RiversEnabled bool    `json:"riversEnabled"`
	RiverCount    int     `json:"riverCount"`
	RiverWidth    string  `json:"riverWidth"`
	LakesEnabled  bool    `json:"lakesEnabled"`
	LakeCount     int     `json:"lakeCount"`
	LakeSize      string  `json:"lakeSize"`
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
		c.CellCount = 500
	}
	if c.Terrain.CoastSide == "" {
		c.Terrain.CoastSide = "south"
	}
	if c.Terrain.RiverWidth == "" {
		c.Terrain.RiverWidth = "medium"
	}
	if c.Terrain.LakeSize == "" {
		c.Terrain.LakeSize = "medium"
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
	if t.RiverCount < 0 || t.RiverCount > 20 {
		return errors.New("riverCount must be 0–20")
	}
	validWidths := map[string]bool{"narrow": true, "medium": true, "wide": true}
	if t.RiverWidth != "" && !validWidths[t.RiverWidth] {
		return errors.New("riverWidth must be narrow/medium/wide")
	}
	if t.LakeCount < 0 || t.LakeCount > 20 {
		return errors.New("lakeCount must be 0–20")
	}
	validSizes := map[string]bool{"small": true, "medium": true, "large": true}
	if t.LakeSize != "" && !validSizes[t.LakeSize] {
		return errors.New("lakeSize must be small/medium/large")
	}
	return nil
}
