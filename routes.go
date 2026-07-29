package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"main/frontend"
	"main/lib/caches"
	"main/lib/imagecolorsort"
	"main/lib/killstorage"
	"main/lib/levelcoords"
	"maps"
	"math"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/a-h/templ"
	"github.com/fogleman/gg"
	"github.com/rs/zerolog/log"
)

func makeHTTPServeMux() http.HandlerFunc {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", httpLog(handle404))
	mux.HandleFunc("GET /static/", httpLog(http.StripPrefix("/static/", http.FileServer(http.Dir("static"))).ServeHTTP))
	mux.HandleFunc("GET /{$}", httpLog(compRender(serveIndex)))
	mux.HandleFunc("GET /stats", httpLog(compRender(serveStats)))
	// mux.HandleFunc("GET /waitroom/{p...}", httpLog(compRender(seveWaitroom)))

	mux.HandleFunc("GET /minimap/{size}/{k...}", serveCachedMinimaps)
	mux.HandleFunc("GET /heat", httpLog(serveHeat))

	mux.HandleFunc("GET /missions...", httpLog(servePermaRedirect("/")))
	mux.HandleFunc("GET /clans...", httpLog(servePermaRedirect("/")))
	mux.HandleFunc("GET /players...", httpLog(servePermaRedirect("/")))
	mux.HandleFunc("GET /sessions...", httpLog(servePermaRedirect("/")))

	return mux.ServeHTTP
}

// func ensureCached(work http.HandlerFunc, checks ...caches.ValueCacheCommon) http.HandlerFunc {
// 	return func(w http.ResponseWriter, r *http.Request) {
// 		t := time.Now()
// 		for {
// 			ready := true
// 			for _, c := range checks {
// 				if !c.Ready() {
// 					ready = false
// 					break
// 				}
// 			}
// 			if ready {
// 				work(w, r)
// 				return
// 			}
// 			time.Sleep(1 * time.Millisecond)
// 			if time.Since(t) > 500*time.Millisecond {
// 				w.Header().Add("Location", "/waitroom/"+url.PathEscape(r.URL.Path))
// 				w.WriteHeader(http.StatusFound)
// 				return
// 			}
// 		}
// 	}
// }

// func seveWaitroom(w http.ResponseWriter, r *http.Request) templ.Component {
// 	p := r.PathValue("p")
// 	if p == "" || p[0] != '/' {
// 		w.Header().Add("Location", "/")
// 		w.WriteHeader(http.StatusFound)
// 		return nil
// 	}
// 	w.Header().Add("Location", p)
// 	w.Header().Add("Retry-After", "5")
// 	w.WriteHeader(http.StatusFound)
// 	return frontend.Page(frontend.Waitroom())
// }

var (
	levelByColorSorter = imagecolorsort.NewImageColorSort(func(id string) (*image.RGBA, error) {
		ret, err := tankmapsCache.Get(base64.StdEncoding.EncodeToString([]byte(id)))
		if err != nil {
			return nil, err
		}
		im, err := png.Decode(bytes.NewReader(ret))
		imRGBA, ok := im.(*image.RGBA)
		if !ok {
			imNRGBA, ok := im.(*image.NRGBA)
			if !ok {
				return nil, fmt.Errorf("not rgba or nrgba from tankmap cache: %T", im)
			}
			imRGBA = &image.RGBA{
				Pix:    imNRGBA.Pix,
				Stride: imNRGBA.Stride,
				Rect:   imNRGBA.Rect,
			}
		}
		// log.Info().Str("id", id).Msg("colorguessing")
		return imRGBA, nil
	})
	levelAmountsCache = caches.NewValueRefresh(6*time.Hour, func() []killstorage.AmountsByLevelRow {
		ret, err := ks.GetAmountsByLevel(context.Background())
		if err != nil {
			log.Err(err).Msg("get amounts by level")
			ret = []killstorage.AmountsByLevelRow{}
		}
		return ret
	})
	levelStatsSorted = caches.NewValueRefresh(6*time.Hour, getSortedLevelStats)
)

func serveIndex(w http.ResponseWriter, r *http.Request) templ.Component {
	levels := levelStatsSorted.Get()
	vehicles := slices.Collect(maps.Values(ks.GetDictVehicles()))
	slices.Sort(vehicles)
	return frontend.Page(frontend.Index(levels, vehicles))
}

func getSortedLevelStats() []frontend.LevelStat {
	levelAmounts := levelAmountsCache.Get()
	levelNames := make([]string, len(levelAmounts))
	for i, v := range levelAmounts {
		levelNames[i] = v.LevelName
	}
	err := levelByColorSorter.Sort(levelNames)
	if err != nil {
		log.Err(err).Msg("sort")
	}
	levels := make([]frontend.LevelStat, 0, len(levelAmounts))
	for _, levelName := range levelNames {
		levels = append(levels, frontend.LevelStat{
			Level:        levelName,
			LevelDisplay: levelToLocalized(levelName),
			Samples: levelAmounts[slices.IndexFunc(levelAmounts, func(val killstorage.AmountsByLevelRow) bool {
				return val.LevelName == levelName
			})].Count,
		})
	}
	return levels
}

func serveHeat(w http.ResponseWriter, r *http.Request) {
	perf := time.Now()
	q := r.URL.Query()
	level := q.Get("level")
	if level == "" {
		w.WriteHeader(204)
		return
	}

	levelOffsets, err := levelcoords.GetLevelCoordsCached(cfg.GetDString("cache/offsets.json", "cacheOffsets"), level)
	if err != nil {
		log.Err(err).Msg("get level offsets")
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
		return
	}

	kq := &killstorage.QueryConditions{} //(time.Now().Add(-7*24*time.Hour), time.Now())
	ks.QueryWithLevel(kq, level)
	killerTeamStr := q.Get("killerTeam")
	if killerTeamStr != "" {
		killerTeam, err := strconv.Atoi(killerTeamStr)
		if err == nil {
			kq.QueryWithKillerTeam(killerTeam)
		}
	}
	killTimeMinStr := q.Get("killTimeMin")
	if killTimeMinStr != "" {
		killTimeMin, err := strconv.Atoi(killTimeMinStr)
		if err == nil {
			kq.QueryWithKillTimeMin(time.Duration(killTimeMin) * time.Second)
		}
	}
	killTimeMaxStr := q.Get("killTimeMax")
	if killTimeMaxStr != "" {
		killTimeMax, err := strconv.Atoi(killTimeMaxStr)
		if err == nil {
			kq.QueryWithKillTimeMax(time.Duration(killTimeMax) * time.Second)
		}
	}
	killerVehicle := q.Get("killerVehicle")
	if killerVehicle != "" {
		ks.QueryWithKillerVehicle(kq, killerVehicle)
	}
	tally, err := ks.GetKillCountsByCoord(r.Context(), kq)
	if err != nil {
		log.Err(err).Msg("get kills")
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
		return
	}

	areaW := float32(math.Abs(float64(levelOffsets.TankMap0[0] - levelOffsets.TankMap1[0])))
	areaH := float32(math.Abs(float64(levelOffsets.TankMap0[1] - levelOffsets.TankMap1[1])))
	areaOffsetX := levelOffsets.TankMap0[0]
	areaOffsetZ := levelOffsets.TankMap0[1]
	outputW := int(areaW)
	outputH := int(areaH)
	out := image.NewRGBA(image.Rect(0, 0, outputW, outputH))

	scoreIntensityStr := q.Get("scoreIntensity")
	scoreIntensity, err := strconv.Atoi(scoreIntensityStr)
	if err != nil {
		scoreIntensity = 32
	}
	countIntensityStr := q.Get("countIntensity")
	countIntensity, err := strconv.Atoi(countIntensityStr)
	if err != nil {
		countIntensity = 32
	}

	// log.Info().Msgf("scale %f %f area %f %f offsets %#v", scaleW, scaleH, areaW, areaH, levelOffsets)
	totalN := 0
	for _, v := range tally {
		tx := math.Round(float64(float64((float32(v.X)-areaOffsetX)/areaW) * float64(outputW)))
		tz := math.Round(float64(float64(1-(float32(v.Z)-areaOffsetZ)/areaH) * float64(outputH)))
		out.SetRGBA(int(tx), int(tz), color.RGBA{
			R: uint8(max(min(v.Score*scoreIntensity, 255), 0)),
			G: 0,
			B: uint8(max(min(-v.Score*scoreIntensity, 255), 0)),
			A: uint8(min(v.Count*countIntensity, 255)),
		})
		totalN += v.Count
	}
	ggStrings(gg.NewContextForImage(out), 20, 20, color.RGBA{R: 255, G: 255, B: 255, A: 128}, color.RGBA{R: 0, G: 0, B: 0, A: 255},
		"Rendered by FlexCoral at thunder.nanachi.party",
		"Data provided by Lux",
		fmt.Sprintf("Data points: %d", totalN),
		time.Now().Round(0).String(),
	).EncodePNG(w)
	log.Info().Dur("perf", time.Since(perf)).Int("nPix", len(tally)).Int("nDp", totalN).Msg("heat")
}

func compRender(f func(w http.ResponseWriter, r *http.Request) templ.Component) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		c := f(w, r)
		if c != nil {
			c.Render(r.Context(), w)
		}
	}
}

func ggStrings(ctx *gg.Context, ox, oy float64, colBg, colFg color.RGBA, vals ...string) *gg.Context {
	for _, v := range vals {
		sw, sh := ctx.MeasureString(v)
		ctx.SetRGBA(float64(colBg.R)/255, float64(colBg.G)/255, float64(colBg.B)/255, float64(colBg.A)/255)
		ctx.DrawRectangle(ox-1, oy+3, sw+1, sh)
		ctx.Fill()
		ctx.SetRGBA(float64(colFg.R)/255, float64(colFg.G)/255, float64(colFg.B)/255, float64(colFg.A)/255)
		oy += sh
		ctx.DrawString(v, ox, oy)
		oy += 2
	}
	return ctx
}

func servePermaRedirect(location string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Location", location)
		w.WriteHeader(http.StatusMovedPermanently)
	}
}
