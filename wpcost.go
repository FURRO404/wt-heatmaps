package main

import (
	"fmt"
	"io"
	"main/lib/wt"
	"net/http"
	"os"
	"path/filepath"
)

var (
	battleRatingGetter wt.BattleRatingGetter
)

func wpcostLoad() error {
	cacheFilePath := "cache/wpcost.json"
	f, err := os.Open(cacheFilePath)
	if err == nil {
		tmp, err := wt.NewBattleRatingGetter(f)
		f.Close()
		if err != nil {
			return err
		}
		battleRatingGetter = *tmp
		return nil
	}
	err = os.MkdirAll(filepath.Dir(cacheFilePath), 0766)
	if err != nil {
		return err
	}
	f, err = os.OpenFile(cacheFilePath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	rsp, err := http.Get("https://raw.githubusercontent.com/gszabi99/War-Thunder-Datamine/refs/heads/master/char.vromfs.bin_u/config/wpcost.blkx")
	if err != nil {
		return err
	}
	if rsp.StatusCode != 200 {
		return fmt.Errorf("Fetching wpcost code %s", rsp.Status)
	}
	defer rsp.Body.Close()
	r := io.TeeReader(rsp.Body, f)
	tmp, err := wt.NewBattleRatingGetter(r)
	if err != nil {
		return err
	}
	battleRatingGetter = *tmp
	return nil
}
