package e2e

import (
	"context"
	"fmt"
	"log"

	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/utils"
)

var (
	postgresPath = "../config/samples/sample-config-postgres.yaml"
)

func setupPostgres(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
	log.Println("Setup Sqlite Policy RBAC")
	if p := utils.RunCommand(fmt.Sprintf("kubectl create secret generic kflashback-db-credentials --namespace=kflashback-system --from-literal=dsn='postgres://postgres :postgres@localhost:5432/kflashback?sslmode=require' kubectl apply -f %s",
		postgresPath)); p.Err() != nil {
		log.Printf("Failed to deploy Postgres Policy: %s: %s", p.Err(), p.Out())
		return ctx, p.Err()
	}

	return ctx, nil
}
