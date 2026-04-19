package noise

import "math"

// Perlin is a seeded 2D Perlin noise generator.
type Perlin struct {
	perm [512]int
}

// New creates a Perlin generator from seed.
func New(seed int64) *Perlin {
	p := &Perlin{}
	// Build permutation table with LCG shuffle.
	var table [256]int
	for i := range table {
		table[i] = i
	}
	s := uint64(seed)
	for i := 255; i > 0; i-- {
		s = s*6364136223846793005 + 1442695040888963407
		j := int(s>>33) % (i + 1)
		table[i], table[j] = table[j], table[i]
	}
	for i := 0; i < 256; i++ {
		p.perm[i] = table[i]
		p.perm[i+256] = table[i]
	}
	return p
}

func fade(t float64) float64 {
	return t * t * t * (t*(t*6-15) + 10)
}

func lerp(a, b, t float64) float64 {
	return a + t*(b-a)
}

func grad(hash int, x, y float64) float64 {
	switch hash & 3 {
	case 0:
		return x + y
	case 1:
		return -x + y
	case 2:
		return x - y
	default:
		return -x - y
	}
}

// Eval returns Perlin noise in [-1,1] for (x,y).
func (p *Perlin) Eval(x, y float64) float64 {
	xi := int(math.Floor(x)) & 255
	yi := int(math.Floor(y)) & 255
	xf := x - math.Floor(x)
	yf := y - math.Floor(y)
	u := fade(xf)
	v := fade(yf)
	aa := p.perm[p.perm[xi]+yi]
	ab := p.perm[p.perm[xi]+yi+1]
	ba := p.perm[p.perm[xi+1]+yi]
	bb := p.perm[p.perm[xi+1]+yi+1]
	return lerp(
		lerp(grad(aa, xf, yf), grad(ba, xf-1, yf), u),
		lerp(grad(ab, xf, yf-1), grad(bb, xf-1, yf-1), u),
		v,
	)
}

// Eval01 returns noise normalised to [0,1].
func (p *Perlin) Eval01(x, y float64) float64 {
	return (p.Eval(x, y) + 1) * 0.5
}
