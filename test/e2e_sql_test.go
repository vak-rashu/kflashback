package e2e

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
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
	// dockerImage = "ghcr.io/prashanthjos/kflashback:latest"
	dockerImage = "kflashback-controller:e2e"

	// file paths
	crdPath  = "../config/crd/"
	rbacPath = "../config/rbac/"
	// controllerPath   = "../config/manager/"
	controllerPath   = "./test_data/test-deployment.yaml"
	trackPolicyPath  = "./test_data/test-tracking-policy.yaml"
	sqliteConfigPath = "../config/samples/sample-config.yaml"
	dockerFile       = "../Dockerfile"
	//postgresPath       = "../config/samples/sample-config-postgres.yaml"

	// namespace name value
	namespace = "kflashback-system"
)

func TestMain(m *testing.M) {

	testenv = env.New()
	kindClusterName := envconf.RandomName("kind-cluster", 10)
	kindCluster := kind.NewCluster(kindClusterName)

	testenv.Setup(
		envfuncs.CreateCluster(kindCluster, kindClusterName),
		envfuncs.CreateNamespace(namespace),

		// Applying CRDs
		func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
			log.Println("Applying CRDs")

			if p := utils.RunCommand(fmt.Sprintf("kubectl apply -f %s", crdPath)); p.Err() != nil {
				log.Printf("Failed to deploy kflashback: %s: %s", p.Err(), p.Out())
				return ctx, p.Err()
			}
			return ctx, nil
		},

		// Applying RBAC Policy
		func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
			log.Println("Applying RBAC Policy")

			if p := utils.RunCommand(fmt.Sprintf("kubectl apply -f %s", rbacPath)); p.Err() != nil {
				log.Printf("Failed to deploy kflashback RBAC Policy: %s: %s", p.Err(), p.Out())
				return ctx, p.Err()
			}

			return ctx, nil
		},

		// Apply SQLite kflashback Config
		func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
			log.Println("Apply SQLite kflashback Config")

			if p := utils.RunCommand(fmt.Sprintf("kubectl apply -f %s", sqliteConfigPath)); p.Err() != nil {
				log.Printf("Failed to deploy SQLite Policy: %s: %s", p.Err(), p.Out())
				return ctx, p.Err()
			}

			return ctx, nil
		},

		// Build the Docker Image
		func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
			log.Println("Building the Docker Image...")

			if p := utils.RunCommand(fmt.Sprintf("docker build -t %s .. -f %s", dockerImage, dockerFile)); p.Err() != nil {
				log.Printf("Failed to docker image: %s: %s", p.Err(), p.Out())
				return ctx, p.Err()
			}

			// Loading the Docker Image to KinD Cluster
			log.Printf("Loading the Docker Image to KinD Cluster")

			if err := kindCluster.LoadImage(ctx, dockerImage); err != nil {
				log.Printf("Failed to load image into KinD: %s", err)
				return ctx, err
			}

			// Deploying kflashback controller
			log.Println("Deploying kflashback controller")

			if p := utils.RunCommand(fmt.Sprintf("kubectl apply -f %s", controllerPath)); p.Err() != nil {
				log.Printf("Failed to deploy the controller")
				return ctx, p.Err()
			}

			// waiting for the deployment to be complete
			log.Println("Waiting for the Deployment to be complete")

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

		// Applying tracking policy
		func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
			log.Println("Applying tracking policy")

			if p := utils.RunCommand(fmt.Sprintf("kubectl apply -f %s", trackPolicyPath)); p.Err() != nil {
				log.Printf("Failed to deploy resources: %s: %s", p.Err(), p.Out())
				return ctx, p.Err()
			}

			return ctx, nil
		},

		// port-forward to access api endpoints
		func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
			log.Println("Port forwarding to access api endpoints")

			cmd := exec.Command("kubectl", "port-forward", "-n", "kflashback-system", "svc/kflashback-api", "9090:9090")
			if err := cmd.Start(); err != nil {
				return ctx, fmt.Errorf("failed to start port-forward: %w", err)
			}
			time.Sleep(5 * time.Second) // wait for it to be ready

			return ctx, nil
		},
	)

	testenv.Finish(
		// Cleaning up the resources
		func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
			log.Println("Cleaning up the resources.....")
			utils.RunCommand(fmt.Sprintf("kubectl delete -f %s", controllerPath))
			utils.RunCommand(fmt.Sprintf("kubectl delete -f %s", rbacPath))
			utils.RunCommand(fmt.Sprintf("kubectl delete -f %s", crdPath))
			return ctx, nil
		},

		envfuncs.DeleteNamespace(namespace),
		envfuncs.DestroyCluster(kindClusterName),
	)
	os.Exit(testenv.Run(m))
}
