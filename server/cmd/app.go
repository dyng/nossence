package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/dyng/nosdaily/bot"
	"github.com/dyng/nosdaily/database"
	"github.com/dyng/nosdaily/nostr"
	"github.com/dyng/nosdaily/service"
	"github.com/dyng/nosdaily/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/omeid/uconfig"
)

type Application struct {
	config  *types.Config
	neo4j   *database.Neo4jDb
	service *service.Service
	crawler *nostr.Crawler
	bot     *bot.BotApplication
}

type response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
}

func NewApplication() *Application {
	// load config and init logger
	config := loadConfig()
	initLogger(config)
	log.Debug("Loaded configuration", "config", config)

	// inject dependencies
	neo4j := database.NewNeo4jDb(config)
	service := service.NewService(config, neo4j)
	crawler := nostr.NewCrawler(service)
	bot := bot.NewBotApplication(config, service)
	return &Application{
		config:  config,
		neo4j:   neo4j,
		service: service,
		crawler: crawler,
		bot:     bot,
	}
}

func (app *Application) Run() {
	// connect to neo4j
	err := app.neo4j.Connect()
	if err != nil {
		log.Crit("Failed to connect to neo4j", "err", err)
	}
	defer app.neo4j.Close()

	// add relays
	for _, v := range app.config.Crawler.Relays {
		app.crawler.AddRelay(v)
	}

	// start bot app
	go func() {
		app.bot.Run(context.Background())
	}()

	// start http server
	app.listenAndServe()
}

func (app *Application) listenAndServe() {
	mux := http.NewServeMux()
	mux.HandleFunc("/feed", app.handleFeed)

	log.Info("Server started")
	err := http.ListenAndServe(":8080", mux)
	if errors.Is(err, http.ErrServerClosed) {
		log.Info("Server closed")
	} else {
		log.Error("Server error", "err", err)
	}
}

func (app *Application) handleFeed(w http.ResponseWriter, r *http.Request) {
	doResponse(w, true, app.service.GetFeed())
}

func doResponse(w http.ResponseWriter, success bool, body any) {
	resp := response{
		Success: success,
		Data:    body,
	}

	err := json.NewEncoder(w).Encode(resp)
	if err != nil {
		log.Error("Failed to encode response body", "body", body, "err", err)
	}
}

func loadConfig() *types.Config {
	config := &types.Config{}
	files := uconfig.Files{
		{"config.json", json.Unmarshal},
	}
	_, err := uconfig.Classic(&config, files)
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}
	return config
}

func initLogger(config *types.Config) {
	// log to console by default
	file := os.Stdout

	path := config.Log.Path
	if path != "console" {
		var err error
		file, err = os.OpenFile(path, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			fmt.Printf("Cannot create log file at path %s: %v\n", path, err)
			os.Exit(1)
		}
	}

	handler := log.StreamHandler(file, log.TerminalFormat(false))
	level, _ := log.LvlFromString(config.Log.Level)
	handler = log.LvlFilterHandler(level, handler)
	log.Root().SetHandler(handler)
}
