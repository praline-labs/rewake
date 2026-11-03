package modhost

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// The bodies of the protocol: a request read within its bound as the one
// value the schema names, and an answer or a refusal written as JSON.

// decode reads a body within limit bytes into v, refusing a field the schema
// does not name and anything after the one value.
func decode(w http.ResponseWriter, r *http.Request, v any, limit int64) (int, error) {
	body := http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(v); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return http.StatusRequestEntityTooLarge, fmt.Errorf("a body is at most %d bytes", limit)
		}
		return http.StatusBadRequest, fmt.Errorf("the body is not the request this endpoint takes: %w", err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return http.StatusRequestEntityTooLarge, fmt.Errorf("a body is at most %d bytes", limit)
		}
		return http.StatusBadRequest, errors.New("the body carries more than one value")
	}
	return 0, nil
}

func refuse(w http.ResponseWriter, status int, why string) {
	reply(w, status, Refusal{Error: why})
}

func reply(w http.ResponseWriter, status int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		status, data = http.StatusInternalServerError, []byte(`{"error":"the answer could not be encoded"}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(append(data, '\n'))
}
