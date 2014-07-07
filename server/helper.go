package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"text/template"

	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/config"
	"github.com/nitrous-io/nitrous/db"
)

func (s *Server) Root() string {
	return s.root
}

func (s *Server) Logger() *logger.Logger {
	return s.logger
}

func (s *Server) DB() *db.Database {
	return s.db
}

func (s *Server) Env() *config.Env {
	return s.env
}

func (s *Server) RenderTemplate(w http.ResponseWriter, status int, tmpl *template.Template, data interface{}) {
	buf := &bytes.Buffer{}
	err := tmpl.Execute(buf, data)
	if err != nil {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf("Error: %v", err)))
		s.logger.Error(err)
		return
	}

	s.RenderHTML(w, status, buf.Bytes())
}

func (s *Server) RenderHTML(w http.ResponseWriter, status int, html []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	w.Write(html)
	s.logger.Infof("Response: %v", status)
}

func (s *Server) Respond(w http.ResponseWriter, status int, data []byte) {
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	}
	w.WriteHeader(status)
	w.Write(data)
	s.logger.Infof("Response: %v %v", status, string(data))
}

func (s *Server) RespondEncodeJSON(w http.ResponseWriter, status int, data map[string]interface{}) {
	j, err := json.Marshal(data)
	if err != nil {
		s.handleError(w, err)
		return
	}
	s.Respond(w, status, j)
}
