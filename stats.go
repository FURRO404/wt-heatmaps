package main

import (
	"context"
	"main/frontend"
	"main/lib/caches"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/a-h/templ"
	"github.com/rs/zerolog/log"
)

func collectStatsTables() []frontend.StatsTable {
	ret := []frontend.StatsTable{}
	byLevel, err := ks.GetAmountsByLevel(context.Background())
	if err != nil {
		log.Err(err).Msg("cache update amounts by level")
	}
	byLevelMax := 0
	for _, v := range byLevel {
		byLevelMax = max(byLevelMax, v.Count)
	}
	byLevelRows := [][]templ.Component{}
	for _, k := range byLevel {
		byLevelRows = append(byLevelRows, []templ.Component{
			frontend.TextNode(k.LevelName),
			frontend.TextNode(strconv.Itoa(k.Count)),
			frontend.StatElementFixedPercentBar(float64(k.Count) / float64(byLevelMax)),
		})
	}
	ret = append(ret, frontend.StatsTable{
		Caption:      "Records by level",
		ColumnLabels: []string{"Level", "Count"},
		Rows:         byLevelRows,
	})

	byDay, err := ks.GetAmountsByDay(context.Background())
	if err != nil {
		log.Err(err).Msg("cache update amounts by day")
	}
	byDayMax := 0
	for _, v := range byDay {
		byDayMax = max(byDayMax, v)
	}
	byDayRows := [][]templ.Component{}
	for _, k := range slices.SortedFunc(maps.Keys(byDay), func(a, b time.Time) int {
		return a.Compare(b)
	}) {
		byDayRows = append(byDayRows, []templ.Component{
			frontend.TextNode(k.Format(time.DateOnly)),
			frontend.TextNode(strconv.Itoa(byDay[k])),
			frontend.StatElementFixedPercentBar(float64(byDay[k]) / float64(byDayMax)),
		})
	}
	ret = append(ret, frontend.StatsTable{
		Caption:      "Records by date",
		ColumnLabels: []string{"Time (UTC)", "Count", ""},
		Rows:         byDayRows,
	})
	return ret
}

var (
	statsCache = caches.NewValueRefresh(6*time.Hour, collectStatsTables)
)

func serveStats(_ http.ResponseWriter, _ *http.Request) templ.Component {
	return frontend.Page(frontend.Stats(statsCache.Get()))
}
