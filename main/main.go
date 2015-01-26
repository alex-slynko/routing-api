package main

import (
	"flag"
	"net/http"
	"os"
	"strconv"

	"github.com/pivotal-cf-experimental/routing-api/authentication"
	"github.com/pivotal-cf-experimental/routing-api/db"
	"github.com/pivotal-cf-experimental/routing-api/handlers"
	"github.com/pivotal-golang/lager"

	cf_lager "github.com/cloudfoundry-incubator/cf-lager"
	"github.com/tedsuo/rata"
)

var Routes = rata.Routes{
	{Path: "/v1/routes", Method: "POST", Name: "Routes"},
	{Path: "/v1/routes", Method: "DELETE", Name: "Delete"},
}

var maxTTL = flag.Int("maxTTL", 120, "Maximum TTL on the route")
var port = flag.Int("port", 8080, "Port to run rounting-api server on")
var uaaPublicKey = flag.String("uaa-public-key", "", "Public jwt key for uaa")

func route(f func(w http.ResponseWriter, r *http.Request)) http.Handler {
	return http.HandlerFunc(f)
}

func main() {
	logger := cf_lager.New("routing-api")

	flag.Parse()
	if *uaaPublicKey == "" {
		os.Exit(1)
	}

	logger.Info("database", lager.Data{"etcd-addresses": flag.Args()})
	database := db.NewETCD(flag.Args())

	token := authentication.NewAccessToken(*uaaPublicKey, logger)

	validator := handlers.NewValidator()

	routesHandler := handlers.NewRoutesHandler(token, *maxTTL, validator, database, logger)

	actions := rata.Handlers{
		"Routes": route(routesHandler.Routes),
		"Delete": route(routesHandler.Delete),
	}

	handler, err := rata.NewRouter(Routes, actions)
	if err != nil {
		panic("unable to create router: " + err.Error())
	}

	handler = handlers.LogWrap(handler, logger)

	logger.Info("starting", lager.Data{"port": *port})
	err = http.ListenAndServe(":"+strconv.Itoa(*port), handler)
	if err != nil {
		panic(err)
	}
}