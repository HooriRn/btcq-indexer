package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/julienschmidt/httprouter"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/fetch/sync"
	"gitlab.com/thorchain/midgard/internal/util/miderr"
)

type DebugBlockResponse struct {
	Timestamp db.Nano `json:"timestamp"`
	Height    int64   `json:"blockHeight"`
	// As long as we use `debug` endpoint for debugging only
	// types for block results are not needed to be defined in detail and can be "generic"
	Results interface{} `json:"blockResults"`
}

func debugBlock(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	idStr := ps[0].Value
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		fmt.Fprintf(w, "Provide an integer height or timestamp (%s): %v ", idStr, err)
		return
	}

	// Convert seconds timestamp to nano timestamp
	dl := len(strconv.FormatInt(id, 10))
	if dl > 9 && dl < 17 {
		id = id * 1e9
	}
	height, timestamp, err := TimestampAndHeight(r.Context(), id)
	if err != nil {
		fmt.Fprintf(w, "Height and timestamp lookup error: %v", err)
		return
	}

	var results *coretypes.ResultBlockResults
	results, err = sync.GlobalSync.FetchSingle(height)
	if err != nil {
		fmt.Fprint(w, "Failed to fetch block: ", err)
	}

	buf, _ := json.Marshal(results)
	var any interface{}
	err = json.Unmarshal(buf, &any)
	if err != nil {
		fmt.Fprint(w, "Failed to convert block to interface{}: ", err)
	}

	unwrapBase64Fields(any)
	recSquashAttributes(any)

	resp := DebugBlockResponse{
		Timestamp: timestamp,
		Height:    height,
		Results:   any,
	}

	respJSON(w, resp)
}

func TimestampAndHeight(ctx context.Context, id int64) (
	height int64, timestamp db.Nano, err error) {
	q := `
		SELECT height, timestamp
		FROM block_log
		WHERE height=$1 OR timestamp<=$1
		ORDER BY TIMESTAMP DESC
		LIMIT 1
	`
	rows, err := db.Query(ctx, q, id)
	if err != nil {
		return
	}
	defer rows.Close()

	if !rows.Next() {
		err = miderr.BadRequestF("No such height or timestamp: %d", id)
		return
	}
	err = rows.Scan(&height, &timestamp)
	return
}

var fieldsToUnwrap = map[string]bool{"key": true, "value": true, "data": true}

func unwrapBase64Fields(any interface{}) {
	msgMap, ok := any.(map[string]interface{})
	if ok {
		for k, v := range msgMap {
			if fieldsToUnwrap[k] {
				s, ok := v.(string)
				if ok {
					msgMap[k] = s
				}
			}
			unwrapBase64Fields(v)
		}
	}
	msgSlice, ok := any.([]interface{})
	if ok {
		for i := range msgSlice {
			unwrapBase64Fields(msgSlice[i])
		}
	}
}

// Replace these:
// "attributes": [
//
//		  {
//	     "index": true,
//	     "key": "firstAttr",
//	     "value": "42"
//		  },
//		  {
//		    "index": true,
//		    "key": "secondAttr",
//		    "value": "textvalue"
//		  }
//	 ]
//
// With this:
//
//	"attributes": {
//	  "firstAttr": "42",
//	  "secondAttr": "textvalue"
//	}
//
// If attributes doesn't look like this, keeps original.
func recSquashAttributes(any interface{}) {
	msgMap, ok := any.(map[string]interface{})
	if ok {
		for k, v := range msgMap {
			if k == "attributes" {
				msgMap["attributes"] = squashAttributes(v)
			} else {
				recSquashAttributes(v)
			}
		}
	}
	msgSlice, ok := any.([]interface{})
	if ok {
		for i := range msgSlice {
			recSquashAttributes(msgSlice[i])
		}
	}
}

func squashAttributes(orig interface{}) interface{} {
	vec, ok := orig.([]interface{})
	if !ok {
		return orig
	}

	ret := map[string]interface{}{}

	for _, x := range vec {
		attr, ok := x.(map[string]interface{})
		if !ok {
			return orig
		}
		keyAny, ok := attr["key"]
		if !ok {
			return orig
		}
		key, ok := keyAny.(string)
		if !ok {
			return orig
		}
		value, ok := attr["value"]
		if !ok {
			return orig
		}
		_, alreadyExists := ret[key]
		if alreadyExists {
			return orig
		}

		expectedSize := 2
		_, hasIndexField := attr["index"]
		if hasIndexField {
			expectedSize++
		}

		if len(attr) != expectedSize {
			return orig
		}

		ret[key] = value
	}

	return ret
}
