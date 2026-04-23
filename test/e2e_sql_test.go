//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/pkg/utils"
	"sigs.k8s.io/e2e-framework/support/kind"
)

var (
	testenv env.Environment

	// docker image name
	dockerImage = "ghcr.io/prashanthjos/kflashback:latest"

	// use the file paths
	operatorPath       = "../config/crd/"
	rbacPath           = "../config/rbac/"
	controllerPath     = "../config/manager/"
	trackingPolicyPath = "../config/samples/sample-policy.yaml"
	sqlitePath = "../config/sample/sample-config.yaml"
	postgresPath = "../config/samples/sample-config-postgres.yaml"

	// namespace name
	namespace = "kflashback-system"
)

// get the db backend
func getFunc() string {
	dbtype := os.Getenv("STORAGE_BACKEND")
	log.Printf("Using storage backend: %s", dbtype)

	return dbtype

	// if dbtype == "sqlite" {
	// 	return setupSQLite
	// } else {
	// 	return setupPostgres
	// }
}

func TestMain(m *testing.M) {

	testenv = env.New()
	kindClusterName := envconf.RandomName("kind-cluster", 10)
	kindCluster := kind.NewCluster(kindClusterName)
	dbtype := getFunc()

	backendDB := getFunc()

	testenv.Setup(
		envfuncs.CreateCluster(kindCluster, kindClusterName),
		envfuncs.CreateNamespace(namespace),

		// installing the CRDs
		func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
			log.Println("Installing the CRDs")
			if p := utils.RunCommand(fmt.Sprintf("kubectl apply -f %s", operatorPath)); p.Err() != nil {
				log.Printf("Failed to deploy kflashback: %s: %s", p.Err(), p.Out())
				return ctx, p.Err()
			}
			return ctx, nil
		},

		// installing RBAC policy
		func(ctx context.Context, c *envconf.Config) (context.Context, error) {
			log.Println("Install RBAC")
			if p := utils.RunCommand(fmt.Sprintf("kubectl apply -f %s", rbacPath)); p.Err() != nil {
				log.Printf("Failed to deploy kflashback RBAC Policy: %s: %s", p.Err(), p.Out())
				return ctx, p.Err()
			}

			return ctx, nil
		},

		// if dbtype == "sqlite"{
			
		// } ,

		func setupSQLite(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
			log.Println("Setup Sqlite Policy RBAC")
			if p := utils.RunCommand(fmt.Sprintf("kubectl apply -f %s", sqlitePath)); p.Err() != nil {
				log.Printf("Failed to deploy SQLite Policy: %s: %s", p.Err(), p.Out())
				return ctx, p.Err()
			}

			return ctx, nil
		},

		// if dbtype == "postregsql"{
		// 	func setupPostgres(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		// 		log.Println("Setup Sqlite Policy RBAC")
		// 		if p := utils.RunCommand(fmt.Sprintf("kubectl create secret generic kflashback-db-credentials --namespace=kflashback-system --from-literal=dsn='postgres://postgres :postgres@localhost:5432/kflashback?sslmode=require' kubectl apply -f %s",
		// 		postgresPath)); p.Err() != nil {
		// 			log.Printf("Failed to deploy Postgres Policy: %s: %s", p.Err(), p.Out())
		// 			return ctx, p.Err()
		// 		}

		// 		return ctx, nil
		// 	}
		// },

		// build Docker Image
		func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
			log.Println("Deploying Docker Image")
			if p := utils.RunCommand(fmt.Sprintf("docker build -t %s .", dockerImage)); p.Err() != nil {
				log.Printf("Failed to docker image: %s: %s", p.Err(), p.Out())
				return ctx, p.Err()
			}

			// loading the docker image to KinD cluster
			log.Printf("Loading the Docker Image to KinD Cluster")
			if err := kindCluster.LoadImage(ctx, dockerImage); err != nil {
				log.Printf("Failed to load image into KinD: %s", err)
				return ctx, err
			}

			// deploying the controller
			log.Println("Deploying kflashback controller")
			if p := utils.RunCommand(fmt.Sprintf("kubectl apply -f %s", controllerPath)); p.Err() != nil {
				log.Printf("Failed to deploy the controller")
				return ctx, p.Err()
			}

			// waiting for the deployment to be complete
			log.Println("Waitting for the Deplyment of the Controller")
			client := cfg.Client()
			if err := wait.For(
				conditions.New(client.Resources()).DeploymentAvailable("kflashback-controller", namespace),
				wait.WithTimeout(3*time.Minute),
				wait.WithInterval(10*time.Second),
			); err != nil {
				log.Printf("Timed out while waiting for the controller deployment: %s", err)
				return ctx, err
			}

			return ctx, nil
		},

		// create a tracking policy
		func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
			log.Println("Deploying the tracking policy")
			if p := utils.RunCommand(fmt.Sprintf("kubectl apply -f %s", trackingPolicyPath)); p.Err() != nil {
				log.Printf("Failed to deploy resources: %s: %s", p.Err(), p.Out())
				return ctx, p.Err()
			}

			return ctx, nil
		},

		// port-forward the api dashboard to access apis
		func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
			log.Println("Port forward the container api port")
			if p := utils.RunCommand("kubectl port-forward -n kflashback-system svc/kflashback-api 9090:9090"); p.Err() != nil {
				time.Sleep(3 * time.Second) // give it a moment to start
				return ctx, p.Err()
			}

			return ctx, nil
		},
	)

	testenv.Finish(
		// Cleaning up the resources
		func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
			log.Println("Cleaning Up teh Resources.....")
			utils.RunCommand(fmt.Sprintf("kubectl delete -f %s", controllerPath))
			utils.RunCommand(fmt.Sprintf("kubectl delete -f %s", rbacPath))
			utils.RunCommand(fmt.Sprintf("kubectl delete -f %s", operatorPath))
			return ctx, nil
		},

		envfuncs.DeleteNamespace(namespace),
		envfuncs.DestroyCluster(kindClusterName),
	)
	os.Exit(testenv.Run(m))
}
