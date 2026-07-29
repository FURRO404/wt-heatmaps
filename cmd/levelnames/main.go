package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	locBytes := noerr(os.ReadFile("/home/max/p/War-Thunder-Datamine/lang.vromfs.bin_u/lang/missions_locations.csv"))
	locReader := csv.NewReader(bytes.NewReader(locBytes))
	locReader.Comma = ';'
	locRaw := noerr(locReader.ReadAll())
	out := map[string]string{}
	for i := 1; i < len(locRaw)-1; i++ {
		row := locRaw[i]
		if len(row) < 2 {
			continue
		}
		out[row[0]] = row[1]
	}
	fmt.Println(string(noerr(json.MarshalIndent(out, "", "\t"))))
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func noerr[T any](ret T, err error) T {
	must(err)
	return ret
}
