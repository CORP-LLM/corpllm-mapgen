package terrain

// catmullRom returns a densified curve through the given control points
// using uniform Catmull-Rom interpolation. samplesPerSegment controls how
// many interpolated points are emitted between each pair of originals.
//
// Endpoints are duplicated (clamp mode) so the curve starts exactly at
// points[0] and ends at points[len-1], which means the client can draw
// the returned slice as a direct polyline/spline without boundary fudging.
func catmullRom(points []Point, samplesPerSegment int) []Point {
	n := len(points)
	if n < 2 || samplesPerSegment < 1 {
		out := make([]Point, n)
		copy(out, points)
		return out
	}
	out := make([]Point, 0, (n-1)*samplesPerSegment+1)
	out = append(out, points[0])
	for i := 0; i < n-1; i++ {
		p1 := points[i]
		p2 := points[i+1]
		p0 := p1
		if i > 0 {
			p0 = points[i-1]
		}
		p3 := p2
		if i+2 < n {
			p3 = points[i+2]
		}
		for k := 1; k <= samplesPerSegment; k++ {
			t := float64(k) / float64(samplesPerSegment)
			out = append(out, catmullRomPoint(p0, p1, p2, p3, t))
		}
	}
	// Fix float drift at the very last sample: Catmull-Rom at t=1 should
	// land exactly on the final control point but picks up 1–2 ULP of
	// rounding. Clients rely on curve[end] == last control exactly.
	out[len(out)-1] = points[n-1]
	return out
}

func catmullRomPoint(p0, p1, p2, p3 Point, t float64) Point {
	t2 := t * t
	t3 := t2 * t
	// Uniform Catmull-Rom basis:
	// 0.5 * [2P1 + (-P0+P2)t + (2P0-5P1+4P2-P3)t² + (-P0+3P1-3P2+P3)t³]
	return Point{
		X: 0.5 * (2*p1.X +
			(-p0.X+p2.X)*t +
			(2*p0.X-5*p1.X+4*p2.X-p3.X)*t2 +
			(-p0.X+3*p1.X-3*p2.X+p3.X)*t3),
		Y: 0.5 * (2*p1.Y +
			(-p0.Y+p2.Y)*t +
			(2*p0.Y-5*p1.Y+4*p2.Y-p3.Y)*t2 +
			(-p0.Y+3*p1.Y-3*p2.Y+p3.Y)*t3),
	}
}

// cellCenters gathers the center points of the given cell IDs in order.
func cellCenters(cells []Cell, ids []int) []Point {
	out := make([]Point, len(ids))
	for i, id := range ids {
		out[i] = cells[id].Center
	}
	return out
}
