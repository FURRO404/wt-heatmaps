package levelcoords

import "testing"

// A box the browser drew from pixel 10 to pixel 12 covers the heatmap columns
// 10 and 11. Column c holds the kills whose raw x rounds to c, which is the raw
// x from c-0.5 up to c+0.5. The two columns together are 9.5 up to 11.5.
//
// The rows work the same way with the axis flipped. Rows 20 to 23 hold the
// whole meters 4076 down to 4073, which is the raw z from 4072.5 up to 4076.5.
func TestTankMapAreaToWorldSitsOnThePixelEdges(t *testing.T) {
	c := LevelCoords{TankMap0: [2]float32{0, 0}, TankMap1: [2]float32{4096, 4096}}
	x0, z0, x1, z1 := c.TankMapAreaToWorld([4]float64{10.0 / 4096, 20.0 / 4096, 12.0 / 4096, 24.0 / 4096})
	for _, v := range []struct {
		name string
		got  float64
		want float64
	}{
		{"x0", x0, 9.5},
		{"x1", x1, 11.5},
		{"z0", z0, 4076.5},
		{"z1", z1, 4072.5},
	} {
		if v.got != v.want {
			t.Errorf("%s = %v, want %v", v.name, v.got, v.want)
		}
	}
}

// An offset map keeps the same half meter, measured from its own origin.
func TestTankMapAreaToWorldWithAnOffsetOrigin(t *testing.T) {
	c := LevelCoords{TankMap0: [2]float32{1024, 0}, TankMap1: [2]float32{3072, 2048}}
	x0, _, x1, _ := c.TankMapAreaToWorld([4]float64{0, 0, 1, 1})
	if x0 != 1023.5 || x1 != 3071.5 {
		t.Errorf("x0, x1 = %v, %v, want 1023.5, 3071.5", x0, x1)
	}
}

// A drag that left the map stops at the edge.
func TestTankMapAreaToWorldClampsOutsideTheMap(t *testing.T) {
	c := LevelCoords{TankMap0: [2]float32{0, 0}, TankMap1: [2]float32{2048, 2048}}
	x0, z0, x1, z1 := c.TankMapAreaToWorld([4]float64{-3, -3, 4, 4})
	if x0 != -0.5 || x1 != 2047.5 || z0 != 2048.5 || z1 != 0.5 {
		t.Errorf("got %v %v %v %v, want -0.5 2047.5 2048.5 0.5", x0, z0, x1, z1)
	}
}
