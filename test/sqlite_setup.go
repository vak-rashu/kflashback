package e2e

import (
	"context"
	"fmt"
	"log"

	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/utils"
)

var (
	sqlitePath = "../config/sample/sample-config.yaml"
)

func setupSQLite(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
	log.Println("Setup Sqlite Policy RBAC")
	if p := utils.RunCommand(fmt.Sprintf("kubectl apply -f %s", sqlitePath)); p.Err() != nil {
		log.Printf("Failed to deploy SQLite Policy: %s: %s", p.Err(), p.Out())
		return ctx, p.Err()
	}

	return ctx, nil
}
