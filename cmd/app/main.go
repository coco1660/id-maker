package main

import (
	"github.com/ilyakaznacheev/cleanenv"
	"log"

	"id-maker/config"
	"id-maker/internal/app"
)

func main() {
	// Configuration
	var cfg config.Config

	err := cleanenv.ReadConfig("./config/config.yml", &cfg)
	if err != nil {
		log.Fatalf("Config error: %s", err)
	}

	// Run
	app.Run(&cfg)
}
