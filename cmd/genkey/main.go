package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/harshalvk/cage/internal/auth"
	"github.com/harshalvk/cage/internal/config"
	"github.com/harshalvk/cage/internal/store"
)

func main() {
	name := flag.String("name", "default", "a label for this key, e.g. 'local-dev' or 'ci'")
	flag.Parse()

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	st, err := store.NewStore(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}

	rawKey, keyHash, err := auth.GenerateAPIKey()
	if err != nil {
		log.Fatal(err)
	}

	if err := st.CreateAPIKey(ctx, *name, keyHash); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("API key created for '%s':\n%s\n\nSave this now — it will not be shown again.\n", *name, rawKey)
}
