package response

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
)

type Responder struct {
	ErrorLog *log.Logger
}

func (r Responder) WriteJSON(w http.ResponseWriter, status int, data any, headers http.Header) error {
	js, err := json.MarshalIndent(data, "", "\t")
	if err != nil {
		return err
	}
	js = append(js, '\n')
	for key, value := range headers {
		w.Header()[key] = value
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, err = w.Write(js)
	if err != nil {
		return err
	}

	return nil
}

func (r Responder) ServerError(w http.ResponseWriter, err error) {
	trace := fmt.Sprintf("%s\n%s", err.Error(), debug.Stack())
	if r.ErrorLog != nil {
		r.ErrorLog.Println(trace)
	}

	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

func (r Responder) ClientError(w http.ResponseWriter, status int) {
	http.Error(w, http.StatusText(status), status)
}

func (r Responder) BadRequest(w http.ResponseWriter) {
	r.ClientError(w, http.StatusBadRequest)
}

func (r Responder) NotFound(w http.ResponseWriter) {
	r.ClientError(w, http.StatusNotFound)
}
