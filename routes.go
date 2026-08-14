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
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/fogleman/gg"
	"github.com/rs/zerolog/log"
)

func makeHTTPServeMux() http.HandlerFunc {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", httpLog(handle404))
	mux.HandleFunc("GET /static/", httpLog(http.StripPrefix("/static/", http.FileServer(http.Dir("static"))).ServeHTTP))
	mux.HandleFunc("GET /{$}", httpLog(compRenderFn(serveIndex)))
	mux.HandleFunc("GET /stats", httpLog(compRenderFn(serveStats)))
	mux.HandleFunc("GET /about", httpLog(compRender(frontend.Page(frontend.About()))))
	// mux.HandleFunc("GET /waitroom/{p...}", httpLog(compRender(seveWaitroom)))

	mux.HandleFunc("GET /minimap/{size}/{k...}", serveCachedMinimaps)
	mux.HandleFunc("GET /heat", httpLog(serveHeat))
	mux.HandleFunc("GET /areastats", httpLog(compRenderFn(serveAreaStats)))

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
	return frontend.Page(frontend.Index(levels, battleRatingGetter.GetRankMax()))
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

	kq, ok := buildKillQuery(q, level)
	if !ok {
		w.WriteHeader(204)
		return
	}

	// log.Info().Msg(kq.Dump())

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

	scoreIntensity := urlValueIntOr(q, "scoreIntensity", 32)
	countIntensity := urlValueIntOr(q, "countIntensity", 32)

	// log.Info().Msgf("scale %f %f area %f %f offsets %#v", scaleW, scaleH, areaW, areaH, levelOffsets)
	totalN := 0
	for _, v := range tally {
		tx := math.Round(float64(float64((float32(v.X)-areaOffsetX)/areaW) * float64(outputW)))
		tz := math.Round(float64(float64(1-(float32(v.Z)-areaOffsetZ)/areaH) * float64(outputH)))

		out.SetRGBA(int(tx), int(tz), color.RGBA{
			R: uint8(max(min(v.Score*scoreIntensity, 255), 0)),
			G: uint8(max(min(v.Score*v.Score, 255), 0) / 2),
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

func buildKillQuery(q url.Values, level string) (*killstorage.QueryConditions, bool) {
	kq := &killstorage.QueryConditions{}
	if !ks.QueryWithLevel(kq, level) {
		return nil, false
	}
	if val := urlValueInt(q, "killerTeam"); val != nil {
		kq.QueryWithKillerTeam(*val)
	}
	if val := urlValueInt(q, "killTimeMin"); val != nil {
		kq.QueryWithKillTimeMin(time.Duration(*val) * time.Second)
	}
	if val := urlValueInt(q, "killTimeMax"); val != nil {
		kq.QueryWithKillTimeMax(time.Duration(*val) * time.Second)
	}
	if val := battleRatingGetter.GetAllInRange(urlValueInt(q, "killerBattleRatingMin"), urlValueInt(q, "killerBattleRatingMax")); val != nil {
		for i := range val {
			val[i] = "tankmodels/" + val[i]
		}
		ks.QueryWithKillerVehicles(kq, val)
	}
	if val := battleRatingGetter.GetAllInRange(urlValueInt(q, "victimBattleRatingMin"), urlValueInt(q, "victimBattleRatingMax")); val != nil {
		for i := range val {
			val[i] = "tankmodels/" + val[i]
		}
		ks.QueryWithVictimVehicles(kq, val)
	}
	return kq, true
}

const areaStatsLimit = 50

func serveAreaStats(w http.ResponseWriter, r *http.Request) templ.Component {
	q := r.URL.Query()
	level := q.Get("level")
	box, ok := areaBoxFromQuery(q)
	if level == "" || !ok {
		w.WriteHeader(400)
		return nil
	}

	levelOffsets, err := levelcoords.GetLevelCoordsCached(cfg.GetDString("cache/offsets.json", "cacheOffsets"), level)
	if err != nil {
		log.Err(err).Msg("get level offsets")
		w.WriteHeader(500)
		return nil
	}
	x0, z0, x1, z1 := levelOffsets.TankMapAreaToWorld(box)

	kq, ok := buildKillQuery(q, level)
	if !ok {
		w.WriteHeader(204)
		return nil
	}
	kq.QueryWithArea(x0, z0, x1, z1)

	stats, err := ks.GetVehicleStatsByArea(r.Context(), kq, areaStatsLimit)
	if err != nil {
		log.Err(err).Msg("get vehicle stats by area")
		w.WriteHeader(500)
		return nil
	}

	rows := make([]frontend.AreaVehicleStat, len(stats))
	total := 0
	for i, v := range stats {
		rows[i] = frontend.AreaVehicleStat{
			Vehicle: strings.TrimPrefix(v.Vehicle, "tankmodels/"),
			Kills:   v.Kills,
			Deaths:  v.Deaths,
		}
		total += v.Kills + v.Deaths
	}
	return frontend.AreaStats(rows, int(math.Round(math.Abs(x1-x0))), int(math.Round(math.Abs(z1-z0))), total, areaStatsLimit)
}

func areaBoxFromQuery(vals url.Values) (box [4]float64, ok bool) {
	for i, name := range []string{"u0", "v0", "u1", "v1"} {
		v, err := strconv.ParseFloat(vals.Get(name), 64)
		if err != nil || math.IsNaN(v) {
			return box, false
		}
		box[i] = v
	}
	return box, box[0] != box[2] && box[1] != box[3]
}

func urlValueInt(vals url.Values, name string) *int {
	if !vals.Has(name) {
		return nil
	}
	ret, err := strconv.Atoi(vals.Get(name))
	if err != nil {
		return nil
	}
	return &ret
}

func urlValueIntOr(vals url.Values, name string, or int) int {
	if !vals.Has(name) {
		return or
	}
	ret, err := strconv.Atoi(vals.Get(name))
	if err != nil {
		return or
	}
	return ret
}

func compRenderFn(f func(w http.ResponseWriter, r *http.Request) templ.Component) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		c := f(w, r)
		if c != nil {
			c.Render(r.Context(), w)
		}
	}
}

func compRender(c templ.Component) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		c.Render(r.Context(), w)
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
