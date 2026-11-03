package main

import (
	"log"

	"github.com/EthanShen10086/earnings-sync/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
