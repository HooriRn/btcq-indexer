package api

import (
	"net/http"

	"github.com/julienschmidt/httprouter"

	"github.com/btcq/btcq-indexer/openapi/generated"
)

func jsonDoc(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(generated.DocHTML)
}
