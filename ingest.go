package main

import (
	"context"
	"encoding/json"
	"main/lib/killstorage"
	"main/lib/lux"
	"main/lib/ratetrack"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
)

var (
	ingestStatSessionRate5m = &atomic.Int64{}
	ingestStatKillsRate5m   = &atomic.Int64{}
)

func ingestRoutine(exitChan <-chan struct{}) {
	luxToken, haveLuxToken := cfg.GetString("lux", "token")
	if !haveLuxToken {
		return
	}

	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	carvesChan := make(chan *lux.LuxCarve, 16)
	preferencesChan := make(chan lux.FetchPreferences, 16)
	reconnectExitChan := make(chan struct{})
	updatePreferencesExitChan := make(chan struct{})

	wg.Go(func() {
		prefs, err := getPreferences()
		preferencesChan <- prefs
		log.Err(err).Str("maps", prefs.String()).Msg("getting initial preferences")
		for {
			select {
			case <-updatePreferencesExitChan:
				return
			case <-time.After(20 * time.Minute):
				prefs, err := getPreferences()
				log.Err(err).Str("maps", prefs.String()).Msg("getting hourly preferences")
				select {
				case preferencesChan <- prefs:
					log.Info().Msg("ingest preferences updated")
				default:
					log.Warn().Msg("ingest preferences not updated")
				}
			}
		}
	})
	wg.Go(func() {
		rate := ratetrack.NewRateTracker(5*time.Minute, ingestStatSessionRate5m, ingestStatKillsRate5m)
		for carve := range carvesChan {
			kills, err := killstorage.LuxCarveToKills(carve)
			if err != nil {
				log.Err(err).Msg("carve to kills fail")
				b, _ := json.Marshal(carve)
				os.WriteFile("dump.json", b, 0644)
				continue
			}
			err = ks.StoreKills(kills)
			if err != nil {
				log.Err(err).Int("n", len(kills)).Msg("storing kills")
			}
			rate.Measure(len(kills))
		}
		log.Info().Msg("carve loop exited")
	})
	wg.Go(func() {
		defer log.Info().Msg("lux loop exited")
		defer close(carvesChan)
		for {
			select {
			case <-reconnectExitChan:
				return
			case <-time.After(5 * time.Second):
			}
			err := lux.FetchFromLux(log.Logger, ctx.Done(), carvesChan, preferencesChan, luxToken)
			log.Err(err).Msg("lux fetch exited")
		}
	})

	<-exitChan

	cancel()
	close(reconnectExitChan)
	close(updatePreferencesExitChan)

	wg.Wait()
}

func getPreferences() (ret lux.FetchPreferences, err error) {
	amounts, err := ks.GetAmountsByLevel(context.Background())
	if err != nil {
		return
	}
	ret = lux.FetchPreferences{
		Maps:   []string{},
		UIDs:   []string{},
		Groups: []string{"tank"},
	}
	i := 0
	for _, v := range slices.Backward(amounts) {
		if i >= cfg.GetDInt(20, "lux", "fetchMapsCount") {
			break
		}
		if v.LevelName == "levels/avg_nuclear_incident.bin" {
			continue
		}
		ret.Maps = append(ret.Maps, levelToLocalized(v.LevelName))
		i++
	}
	return
}
