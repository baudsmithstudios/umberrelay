package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

func decodeJSON(r *http.Request, dst interface{}) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return unsupportedMediaTypeError{message: "Content-Type must be application/json"}
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return requestEntityTooLargeError{}
		}
		return fmt.Errorf("invalid JSON request body")
	}
	if decoder.More() {
		return fmt.Errorf("request body must contain a single JSON object")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func decodeAPIJSON(w http.ResponseWriter, r *http.Request, dst interface{}) bool {
	if err := decodeJSON(r, dst); err != nil {
		var mediaErr unsupportedMediaTypeError
		var tooLargeErr requestEntityTooLargeError
		if errors.As(err, &mediaErr) {
			writeJSONError(w, http.StatusUnsupportedMediaType, mediaErr.Error())
			return false
		}
		if errors.As(err, &tooLargeErr) {
			writeJSONError(w, http.StatusRequestEntityTooLarge, tooLargeErr.Error())
			return false
		}
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return false
	}
	return true
}

type unsupportedMediaTypeError struct {
	message string
}

func (e unsupportedMediaTypeError) Error() string {
	return e.message
}

type requestEntityTooLargeError struct{}

func (e requestEntityTooLargeError) Error() string {
	return "request body too large"
}
