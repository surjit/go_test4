package main

import (
	"baljeet/controllers"
	"github.com/gofiber/fiber/v2"
	"log"
)

func main() {
	// Create a Fiber app
	app := fiber.New(fiber.Config{
		Prefork:               true,
		CaseSensitive:         true,
		StrictRouting:         true,
		DisableStartupMessage: true,
		ServerHeader:          "Fibre v2",
	})

	app.Static("/", "./public")

	app.Get("/terminal-ws/:id", controllers.TerminalHandler())

	log.Fatal(app.Listen("0.0.0.0:3000"))
}
