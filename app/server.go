package app

import (
	"fmt"
	"log"
	"net/http"

	"github.com/odpf/siren/api"
	"github.com/odpf/siren/provider"
	"github.com/odpf/siren/store"
)

// RunServer runs the application server
func RunServer(c *Config) error {
	db, err := store.New(&store.Config{
		Host:     c.DBHost,
		User:     c.DBUser,
		Password: c.DBPassword,
		Name:     c.DBName,
		Port:     c.DBPort,
		SslMode:  c.DBSslMode,
	})
	if err != nil {
		return err
	}

	models := []interface{}{
		&provider.Model{},
	}
	store.Migrate(db, models...)

	r := api.New()

	log.Printf("running server on port %d\n", c.Port)
	return http.ListenAndServe(fmt.Sprintf(":%d", c.Port), r)
}
