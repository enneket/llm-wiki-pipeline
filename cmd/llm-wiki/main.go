package main

import (
	"context"
	"fmt"
	"log"
	"os/signal"
	"syscall"

	"llm-wiki/cmd/llm-wiki/commands"
)

var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := commands.Execute(ctx); err != nil {
		log.Fatal(err)
	}
	fmt.Println("llm-wiki exited cleanly")
}
