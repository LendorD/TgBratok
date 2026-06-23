package main

import (
	"log/slog"
	"os"

	"bratok/internal/app"
)

// main — точка входа: запускает приложение и завершает процесс при ошибке.
func main() {
	if err := app.Run(); err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}
