package controllers

import (
	"net/http"
	"text/template"

	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/config"
	"github.com/nitrous-io/nitrous/db"
)

type Helper interface {
	Root() string
	Logger() *logger.Logger
	DB() *db.Database
	Env() *config.Env

	RenderTemplate(w http.ResponseWriter, status int, tmpl *template.Template, data interface{})
	RenderHTML(w http.ResponseWriter, status int, html []byte)
	Respond(w http.ResponseWriter, status int, data []byte)
	RespondEncodeJSON(w http.ResponseWriter, status int, data map[string]interface{})
}
