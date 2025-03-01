package main

import (
	"fmt"
	"os"

	"github.com/anchel/mini-seckill/lib/utils"
	"github.com/charmbracelet/log"
	"github.com/joho/godotenv"
)

func main() {
	fmt.Println("hello seckill")
	exe, err := os.Executable()
	if err != nil {
		log.Error("os.Executable error", "message", err)
		os.Exit(-1)
	}
	log.Info("executable path", exe)

	if utils.CheckEnvFile() {
		log.Info("loading .env file")
		err := godotenv.Load()
		if err != nil {
			log.Error("Error loading .env file")
			os.Exit(-1)
		}
		log.Info("load .env successful")
	}
}
