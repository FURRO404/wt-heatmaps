package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"iter"
	"main/lib/killstorage-duckdb"
	"main/lib/levelcoords"
	"math"
	"net/http"
	"net/url"
	"time"

	"github.com/fogleman/gg"
	"github.com/rs/zerolog/log"
)

// Auto-exposure walks out from the peak of a pixel histogram. It stops where a
// rolling average over autoExposureWindow values falls by less than
// autoExposureSlope (natural log) for each step, because the tail of peak
// pixels starts there. On a map with a lot of data, this gives limits near 8,
// which is the same look as intensity 32.
const (
	autoExposureWindow = 3
	autoExposureSlope  = 0.4
)

// histogramTailStart finds the peak of h from the index from in the direction
// dir, then walks from the peak in the direction dir. It returns the index
// where the tail of h starts.
func histogramTailStart(h []int, from, dir int) int {
	avg := func(i int) float64 {
		sum := 0
		for j := i; j >= 0 && j < len(h) && j != i+dir*autoExposureWindow; j += dir {
			sum += h[j]
		}
		return float64(sum) / autoExposureWindow
	}
	i := from
	for j := from; j >= 0 && j < len(h); j += dir {
		if h[j] > h[i] {
			i = j
		}
	}
	for ; i+dir >= 0 && i+dir < len(h); i += dir {
		a, b := avg(i), avg(i+dir)
		if b == 0 || math.Log(b/a) > -autoExposureSlope {
			break
		}
	}
	return i
}

func serveHeat(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	level := q.Get("level")
	if level == "" {
		w.WriteHeader(204)
		return
	}

	levelOffsets, err := getLevelOffsets(level)
	if err != nil {
		log.Err(err).Msg("get level offsets")
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
		return
	}

	var out []byte

	perf := time.Now()
	if isDefaultQuery(q) {
		err = cachedHeat.Access(level, func(a *cachedHeatAggregate) (*cachedHeatAggregate, error) {
			if a == nil {
				perf := time.Now()
				a, err = cachedHeatInitAggregate(level)
				log.Info().Msgf("heat perf cache miss took %s", time.Since(perf).String())
			} else {
				log.Info().Msg("heat perf cache hit")
			}
			out = renderTally(levelOffsets, a.Loop(), q)
			return a, err
		})
	} else {
		kq := buildKillQuery(q, level)
		if kq == nil {
			w.WriteHeader(204)
			return
		}
		// log.Info().Msg(kq.Dump())
		a, err := ks.GetKillCountsByCoord(r.Context(), kq)
		if err != nil {
			log.Err(err).Msg("get kills")
			w.WriteHeader(500)
			w.Write([]byte(err.Error()))
			return
		}
		out = renderTally(levelOffsets, func(yield func(*killstorage.KillTally) bool) {
			for i := range a {
				if !yield(&a[i]) {
					return
				}
			}
		}, q)
	}
	log.Info().Msgf("heat perf total %s", time.Since(perf).String())
	if err != nil {
		return
	}

	w.Write(out)
}

func renderTally(levelOffsets levelcoords.LevelCoords, tally iter.Seq[*killstorage.KillTally], q url.Values) []byte {
	perf := time.Now()

	areaW := float32(math.Abs(float64(levelOffsets.TankMap0[0] - levelOffsets.TankMap1[0])))
	areaH := float32(math.Abs(float64(levelOffsets.TankMap0[1] - levelOffsets.TankMap1[1])))
	areaOffsetX := levelOffsets.TankMap0[0]
	areaOffsetZ := levelOffsets.TankMap0[1]
	outputW := int(areaW)
	outputH := int(areaH)
	out := image.NewRGBA(image.Rect(0, 0, outputW, outputH))

	scoreIntensity := float64(urlValueIntOr(q, "scoreIntensity", 0))
	countIntensity := float64(urlValueIntOr(q, "countIntensity", 0))

	// log.Info().Msgf("scale %f %f area %f %f offsets %#v", scaleW, scaleH, areaW, areaH, levelOffsets)
	totalN := uint64(0)
	maxCount, minDelta, maxDelta := int64(0), int64(0), int64(0)
	blockKills := make([]int64, outputH*outputW)
	blockDeaths := make([]int64, outputH*outputW)

	for v := range tally {
		tx := math.Round(float64(float64((float32(v.X)-areaOffsetX)/areaW) * float64(outputW)))
		tz := math.Round(float64(float64(1-(float32(v.Z)-areaOffsetZ)/areaH) * float64(outputH)))
		if tx < 0 || tx >= float64(outputW) || tz < 0 || tz >= float64(outputH) {
			continue
		}
		vk, vd := int64(v.Kills), int64(v.Deaths)
		blockKills[int(tx+tz*float64(outputW))] = vk
		blockDeaths[int(tx+tz*float64(outputW))] = vd
		totalN += uint64(v.Kills + v.Deaths)
		maxCount = max(maxCount, vk+vd)
		minDelta = min(minDelta, vk-vd)
		maxDelta = max(maxDelta, vk-vd)
	}

	// Auto-exposure: a pixel at or past a limit gets the full color, so the few
	// peak pixels in the tails of the histograms do not make the rest dark.
	if (scoreIntensity == 0 || countIntensity == 0) && maxCount > 0 {
		countHist := make([]int, maxCount+1)
		deltaHist := make([]int, maxDelta-minDelta+1)
		for i, vk := range blockKills {
			vd := blockDeaths[i]
			if vk+vd == 0 {
				continue
			}
			countHist[vk+vd]++
			deltaHist[vk-vd-minDelta]++
		}
		countUpper := int64(histogramTailStart(countHist, 1, 1))
		deltaLower, deltaUpper := int64(0), int64(0)
		if minDelta < 0 {
			deltaLower = int64(histogramTailStart(deltaHist, int(-1-minDelta), -1)) + minDelta
		}
		if maxDelta > 0 {
			deltaUpper = int64(histogramTailStart(deltaHist, int(1-minDelta), 1)) + minDelta
		}
		// One scale for red and blue, so the same |delta| gives the same color.
		if scoreIntensity == 0 {
			scoreIntensity = 255 / float64(max(-deltaLower, deltaUpper, 1))
		}
		if countIntensity == 0 {
			countIntensity = 255 / float64(max(countUpper, 1))
		}
		log.Info().Int64("deltaLower", deltaLower).Int64("deltaUpper", deltaUpper).Int64("countUpper", countUpper).
			Float64("scoreIntensity", scoreIntensity).Float64("countIntensity", countIntensity).Msg("heat auto-exposure")
	}

	for i, vk := range blockKills {
		o := i * 4
		vd := blockDeaths[i]
		vdelta := vk - vd
		out.Pix[o+0] = uint8(max(min(float64(vdelta)*scoreIntensity, 255), 0))
		out.Pix[o+1] = uint8(max(min(vdelta*vdelta, 255), 0) / 2)
		out.Pix[o+2] = uint8(max(min(float64(-vdelta)*scoreIntensity, 255), 0))
		out.Pix[o+3] = uint8(max(min(float64(vk+vd)*countIntensity, 255), 0))
	}

	ret := ggStrings(gg.NewContextForImage(out), 20, 20, color.RGBA{R: 255, G: 255, B: 255, A: 128}, color.RGBA{R: 0, G: 0, B: 0, A: 255},
		"Rendered by FlexCoral at thunder.nanachi.party",
		"Data provided by Lux",
		fmt.Sprintf("Data points: %d", totalN),
		time.Now().Round(0).String(),
	)

	log.Info().Msgf("heat perf render took %s", time.Since(perf).String())

	buf := &bytes.Buffer{}
	ret.EncodePNG(buf)
	return buf.Bytes()
}
