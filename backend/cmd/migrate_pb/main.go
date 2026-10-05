package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/edhases/kadrbox-server/config"
	"github.com/edhases/kadrbox-server/internal/auth"
	"github.com/edhases/kadrbox-server/internal/repository/postgres"
)

type PBUserRecord struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
	Bio      string `json:"bio"`
}

type PBListResponse[T any] struct {
	Items []T `json:"items"`
}

func main() {
	pbURL := os.Getenv("POCKETBASE_URL")
	if pbURL == "" {
		pbURL = "https://oxide.skystreamua.space"
	}
	pbAdminToken := os.Getenv("POCKETBASE_ADMIN_TOKEN")

	log.Printf("[Migrator] Connecting to PocketBase at %s...", pbURL)

	cfg := config.Load()
	ctx := context.Background()
	dbPool, err := postgres.InitDB(ctx, cfg.PostgresDSN())
	if err != nil {
		log.Fatalf("failed to connect to postgres: %v", err)
	}
	defer dbPool.Close()

	log.Println("[Migrator] Connected to PostgreSQL. Fetching collections from PocketBase...")

	// 1. Експорт користувачів
	users, err := fetchPBRecords[PBUserRecord](pbURL, "users", pbAdminToken)
	if err != nil {
		log.Printf("[Migrator] Warning: could not fetch users (token may be required): %v", err)
	} else {
		log.Printf("[Migrator] Found %d users in PocketBase. Importing...", len(users))
		userRepo := postgres.NewUserRepository(dbPool)
		for _, u := range users {
			dummyHash, _ := auth.HashPassword("DefaultMigratedPassword2026!")
			_, err := userRepo.CreateUser(ctx, u.Email, dummyHash, u.Username)
			if err != nil {
				log.Printf("user %s import notice: %v", u.Email, err)
			}
		}
	}

	log.Println("[Migrator] Migration process finished successfully")
}

func fetchPBRecords[T any](baseURL, collection, token string) ([]T, error) {
	reqURL := fmt.Sprintf("%s/api/collections/%s/records?perPage=500", baseURL, collection)
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(b))
	}

	var list PBListResponse[T]
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, err
	}

	return list.Items, nil
}
