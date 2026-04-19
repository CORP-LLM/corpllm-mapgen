package terrain

import (
	"math"
	"testing"
)

func TestCatmullRomEndpointsMatch(t *testing.T) {
	pts := []Point{{0, 0}, {10, 5}, {20, 0}, {30, 10}}
	curve := catmullRom(pts, 4)
	if curve[0] != pts[0] {
		t.Errorf("start: want %v, got %v", pts[0], curve[0])
	}
	if curve[len(curve)-1] != pts[len(pts)-1] {
		t.Errorf("end: want %v, got %v", pts[len(pts)-1], curve[len(curve)-1])
	}
}

func TestCatmullRomSampleCount(t *testing.T) {
	// n controls with samplesPerSegment=k → 1 + (n-1)*k output points.
	pts := []Point{{0, 0}, {10, 5}, {20, 0}, {30, 10}, {40, 5}}
	curve := catmullRom(pts, 4)
	want := 1 + (len(pts)-1)*4
	if len(curve) != want {
		t.Errorf("sample count: want %d, got %d", want, len(curve))
	}
}

func TestCatmullRomShortInput(t *testing.T) {
	// Single point or two points: no interpolation should break.
	if got := catmullRom(nil, 4); len(got) != 0 {
		t.Errorf("nil input: want empty, got %v", got)
	}
	if got := catmullRom([]Point{{1, 1}}, 4); len(got) != 1 {
		t.Errorf("single point: want 1, got %d", len(got))
	}
	if got := catmullRom([]Point{{0, 0}, {10, 0}}, 0); len(got) != 2 {
		t.Errorf("zero samples: expected original copy, got %d", len(got))
	}
}

// TestCatmullRomSmoothness: consecutive sample points should have bounded
// distance between them — no wild discontinuities in the generated spline.
func TestCatmullRomSmoothness(t *testing.T) {
	pts := []Point{{0, 0}, {50, 30}, {100, 0}, {150, 20}, {200, 10}}
	curve := catmullRom(pts, 8)
	// Average segment length between controls = ~55. With 8 samples per
	// segment, the average sample-to-sample distance should be ~55/8 ≈ 7.
	// Cap the worst-case at 3× that to catch bugs that emit outliers.
	for i := 1; i < len(curve); i++ {
		d := math.Hypot(curve[i].X-curve[i-1].X, curve[i].Y-curve[i-1].Y)
		if d > 20 {
			t.Errorf("huge jump %.2f between sample %d and %d (%v → %v)",
				d, i-1, i, curve[i-1], curve[i])
		}
	}
}
