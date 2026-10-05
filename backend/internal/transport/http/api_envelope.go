package http

// Unified response envelopes for /api/v1.
//
// Before this file every endpoint in this package invented its own shape:
// bare arrays, a bare catalogue object, {"status":"success"}, {"is_favorite":…},
// {"success":true}. A client call site therefore hand-rolled its own
// unwrapping, and a server-side rename silently flipped behaviour — when
// `is_favorite` disappeared, toggleFavorite read null, reported "removed",
// and the server had in fact added the row.
//
// Wire format (BREAKING — the Flutter client must be updated in lockstep):
//
//	success, list    {"data":[ … ],"meta":{"limit":200,"offset":0,"count":2,"total":5,"has_more":true}}
//	success, object  {"data":{ … }}
//	error            {"error":"…"}     (byte-identical to jsonError in auth_handler.go)
//
// `meta` is present only on offset/page-paginated lists. `total` is omitted
// when the repository cannot count cheaply, in which case `has_more` is
// derived from a full page and the client must probe one page further.
//
// Errors deliberately carry no extra fields: the auth handlers already emit
// {"error": msg} via jsonError, and the client only ever reads data['error'].
// Keeping one error shape across the whole API means a client-side unwrapper
// written once works everywhere.
//
// http.Error must not be used in this package: it forces
// `Content-Type: text/plain; charset=utf-8`, which Dio hands the Flutter
// client as a String, and the String branch of the error handler then renders
// the raw body instead of the message.

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
)

func writeAPIError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	// A broken error body is not worth a second attempt: the status is already
	// on the wire, and the client falls back to a generic message.
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// writeJSON is the only success writer in this package. It always sets
// Content-Type explicitly rather than letting net/http sniff the first bytes:
// a body-less or non-JSON body is sniffed as text/plain, which is how
// SaveProgress used to answer 200 with a header-less body.
func writeJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("[http] encode response payload: %v", err)
	}
}

type listEnvelope[T any] struct {
	Data []T       `json:"data"`
	Meta *PageMeta `json:"meta,omitempty"`
}

type objectEnvelope[T any] struct {
	Data T `json:"data"`
}

// PageMeta is the single pagination descriptor. Offset-based endpoints fill
// Limit/Offset; page-based endpoints (the provider catalogues) fill Page.
// Count is always the number of elements actually returned.
type PageMeta struct {
	Limit   *int `json:"limit,omitempty"`
	Offset  *int `json:"offset,omitempty"`
	Page    *int `json:"page,omitempty"`
	Count   int  `json:"count"`
	Total   *int `json:"total,omitempty"`
	HasMore bool `json:"has_more"`
}

func intp(v int) *int { return &v }

// writeList emits the list envelope. A nil slice is normalised to `[]`: the
// client must never have to distinguish "no results" from "null".
func writeList[T any](w http.ResponseWriter, r *http.Request, data []T, meta *PageMeta) {
	if data == nil {
		data = []T{}
	}
	if meta != nil {
		meta.Count = len(data)
		setNextLink(w, r, meta)
	}
	writeJSON(w, http.StatusOK, listEnvelope[T]{Data: data, Meta: meta})
}

func writeObject[T any](w http.ResponseWriter, payload T) {
	writeJSON(w, http.StatusOK, objectEnvelope[T]{Data: payload})
}

// setNextLink emits the RFC 8288 Link header the CORS config already exposes.
// The target is a server-relative reference, which is valid and keeps the
// handler free of scheme/host reconstruction.
func setNextLink(w http.ResponseWriter, r *http.Request, meta *PageMeta) {
	if !meta.HasMore || meta.Count == 0 {
		return
	}
	q := url.Values{}
	for k, vs := range r.URL.Query() {
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	if meta.Page != nil {
		q.Set("page", strconv.Itoa(*meta.Page+1))
	} else {
		next := meta.Count
		if meta.Offset != nil {
			next += *meta.Offset
		}
		q.Set("offset", strconv.Itoa(next))
	}
	next := *r.URL
	next.RawQuery = q.Encode()
	w.Header().Set("Link", fmt.Sprintf("<%s>; rel=\"next\"", next.String()))
}
