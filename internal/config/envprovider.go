package config

import (
	"os"
)

type envProvider interface {
	env() map[string]string
}

var envProviders []envProvider

func SetupEnvProviders() error {
	for _, envProvider := range envProviders {
		env := envProvider.env()
		for k, v := range env {
			if err := os.Setenv(k, v); err != nil {
				return err
			}
		}
	}
	return nil
}
