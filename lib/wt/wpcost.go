package wt

import (
	"encoding/json"
	"errors"
	"io"
	"slices"
)

type wpcostVehicle struct {
	EconomicRankHistorical int `json:"economicRankHistorical"`
}

func (val *wpcostVehicle) UnmarshalJSON(b []byte) error {
	if len(b) == 0 {
		return nil
	}
	switch b[0] {
	case '{':
		type tmp struct {
			EconomicRankHistorical int `json:"economicRankHistorical"`
		}
		var tmpval tmp
		err := json.Unmarshal(b, &tmpval)
		if err != nil {
			return err
		}
		val.EconomicRankHistorical = tmpval.EconomicRankHistorical
	default:
		var tmpval int
		err := json.Unmarshal(b, &tmpval)
		if err != nil {
			return err
		}
		val.EconomicRankHistorical = tmpval
	}
	return nil
}

type BattleRatingGetter struct {
	byBattleRating [][]string
	rankMax        int
}

func NewBattleRatingGetter(wpcostJsonReader io.Reader) (*BattleRatingGetter, error) {
	var wpcost map[string]wpcostVehicle
	err := json.NewDecoder(wpcostJsonReader).Decode(&wpcost)
	if err != nil {
		return nil, err
	}
	rankMax, ok := wpcost["economicRankMax"]
	if !ok {
		return nil, errors.New("economicRankMax not found in json")
	}
	byBattleRating := make([][]string, rankMax.EconomicRankHistorical+1)
	for k, v := range wpcost {
		if k == "economicRankMax" {
			continue
		}
		if v.EconomicRankHistorical < 0 || v.EconomicRankHistorical >= len(byBattleRating) {
			continue
		}
		byBattleRating[v.EconomicRankHistorical] = append(byBattleRating[v.EconomicRankHistorical], k)
	}
	for i := range byBattleRating {
		if byBattleRating[i] == nil {
			byBattleRating[i] = []string{}
		}
		slices.Sort(byBattleRating[i])
	}
	ret := &BattleRatingGetter{
		byBattleRating: byBattleRating,
		rankMax:        rankMax.EconomicRankHistorical,
	}
	return ret, nil
}

func (brg *BattleRatingGetter) GetAllByRank(rank int) []string {
	if rank < 0 {
		return nil
	}
	if rank >= len(brg.byBattleRating) {
		return nil
	}
	return brg.byBattleRating[rank]
}

func (brg *BattleRatingGetter) GetRankMax() int {
	return brg.rankMax
}

func (brg *BattleRatingGetter) GetAllInRange(brminp, brmaxp *int) []string {
	if brminp == nil && brmaxp == nil {
		return nil
	}
	brmin := 0
	brmax := brg.rankMax
	if brminp != nil && *brminp <= brg.rankMax && *brminp >= 0 {
		brmin = *brminp
	}
	if brmaxp != nil && *brmaxp <= brg.rankMax && *brmaxp >= 0 {
		brmax = *brmaxp
	}
	if brmin > brmax {
		brmin, brmax = brmax, brmin
	}
	ret := []string{}
	for i := brmin; i <= brmax; i++ {
		ret = append(ret, brg.GetAllByRank(i)...)
	}
	return ret
}
