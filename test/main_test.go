package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func int32Ptr(i int32) *int32 { return &i }

// deployments used for testing
var testDeployment = &appsv1.Deployment{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "test-app",
		Namespace: "default",
	},
	Spec: appsv1.DeploymentSpec{
		Replicas: int32Ptr(3),
		Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"app": "test-app"},
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{"app": "test-app"},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "app",
						Image: "nginx:1.0",
					},
				},
			},
		},
	},
}

// expectedrevisions-to be checked everytime with the reconstructed version
var expectedRevisions = map[int]struct {
	image    string
	replicas int32
}{
	1: {image: "nginx:1.0", replicas: 3},
	2: {image: "nginx:2.0", replicas: 3},
	3: {image: "nginx:2.0", replicas: 5},
}

type HistoryResponse struct {
	Revisions []Revision `json:"revisions"`
}

type Revision struct {
	Revision  int    `json:"revision"`
	EventType string `json:"eventType"`
}

type ReconstructResponse struct {
	Spec struct {
		Replicas int32 `json:"replicas"`
		Template struct {
			Spec struct {
				Containers []struct {
					Image string `json:"image"`
				} `json:"containers"`
			} `json:"spec"`
		} `json:"template"`
	} `json:"spec"`
}

func TestDeploymentTracking(t *testing.T) {
	// creating the deployment uuid to refer to in assessments
	var deploymentUID string

	feature := features.New("Deployment Tracking")

	// create the deployment

	feature.Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		client := cfg.Client()

		appsv1.AddToScheme(client.Resources().GetScheme())

		if err := client.Resources().Create(ctx, testDeployment); err != nil {
			t.Fatalf("failed to create deployment: %s", err)
		}

		// wait for deployment to be available
		if err := wait.For(
			conditions.New(client.Resources()).DeploymentAvailable("test-app", "default"),
			wait.WithTimeout(2*time.Minute),
			wait.WithInterval(10*time.Second),
		); err != nil {
			t.Fatalf("deployment never became available: %s", err)
		}

		var created appsv1.Deployment
		if err := client.Resources().Get(ctx, "test-app", "default", &created); err != nil {
			t.Fatalf("failed to get deployment uid: %s", err)
		}
		deploymentUID = string(created.UID)
		t.Logf("deployment created with UID: %s", deploymentUID)

		return ctx
	})

	// check revision 1 was recorded
	feature.Assess("revision 1 recorded on creation", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {

		time.Sleep(5 * time.Second)

		history := getHistory(t, deploymentUID)
		if len(history.Revisions) < 1 {
			t.Fatalf("expected at least 1 revision, got %d", len(history.Revisions))
		}

		if history.Revisions[0].EventType != "CREATED" {
			t.Fatalf("expected first revision to be CREATED, got %s", history.Revisions[0].EventType)
		}

		t.Logf("revision 1 recorded correctly ")
		return ctx
	})

	// update image to check revision 2
	feature.Assess("revision 2 recorded on image update", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		client := cfg.Client()

		var deployment appsv1.Deployment
		if err := client.Resources().Get(ctx, "test-app", "default", &deployment); err != nil {
			t.Fatalf("failed to get deployment: %s", err)
		}

		// update the image
		deployment.Spec.Template.Spec.Containers[0].Image = "nginx:2.0"
		if err := client.Resources().Update(ctx, &deployment); err != nil {
			t.Fatalf("failed to update deployment image: %s", err)
		}

		time.Sleep(5 * time.Second)

		history := getHistory(t, deploymentUID)
		if len(history.Revisions) < 2 {
			t.Fatalf("expected at least 2 revisions, got %d", len(history.Revisions))
		}

		t.Logf("revision 2 recorded correctly ")
		return ctx
	})

	// increase number replicas to check revision 3

	feature.Assess("revision 3 recorded on scaling", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		client := cfg.Client()

		var deployment appsv1.Deployment
		if err := client.Resources().Get(ctx, "test-app", "default", &deployment); err != nil {
			t.Fatalf("failed to get deployment: %s", err)
		}

		deployment.Spec.Replicas = int32Ptr(5)
		if err := client.Resources().Update(ctx, &deployment); err != nil {
			t.Fatalf("failed to scale deployment: %s", err)
		}

		time.Sleep(5 * time.Second)

		history := getHistory(t, deploymentUID)
		if len(history.Revisions) < 3 {
			t.Fatalf("expected at least 3 revisions, got %d", len(history.Revisions))
		}

		t.Logf("revision 3 recorded correctly")
		return ctx
	})

	// reconstruct revision 1

	feature.Assess("reconstruct revision 1 matches expected state", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		reconstructed := reconstruct(t, deploymentUID, 1)

		expected := expectedRevisions[1]

		// verify the reconstructed spec
		if reconstructed.Spec.Template.Spec.Containers[0].Image != expected.image {
			t.Fatalf("revision 1 image: expected %s got %s",
				expected.image,
				reconstructed.Spec.Template.Spec.Containers[0].Image,
			)
		}
		if reconstructed.Spec.Replicas != expected.replicas {
			t.Fatalf("revision 1 replicas: expected %d got %d",
				expected.replicas,
				reconstructed.Spec.Replicas,
			)
		}

		t.Logf("revision 1 reconstruction correct")
		return ctx
	})

	// reconstruct revision 2

	feature.Assess("reconstruct revision 2 matches expected state", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		reconstructed := reconstruct(t, deploymentUID, 2)

		expected := expectedRevisions[2]

		// verify 2nd revision
		if reconstructed.Spec.Template.Spec.Containers[0].Image != expected.image {
			t.Fatalf("revision 2 image: expected %s got %s",
				expected.image,
				reconstructed.Spec.Template.Spec.Containers[0].Image,
			)
		}

		t.Logf("revision 2 reconstruction correct ")
		return ctx
	})

	// cleanup the resources
	feature.Teardown(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		client := cfg.Client()
		if err := client.Resources().Delete(ctx, testDeployment); err != nil {
			t.Logf("warning: failed to delete deployment: %s", err)
		}
		return ctx
	})

	testenv.Test(t, feature.Feature())
}

// call the= history API for a resource
func getHistory(t *testing.T, uid string) HistoryResponse {
	t.Helper()
	url := fmt.Sprintf("http://localhost:9090/api/v1/resources/%s/history", uid)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("failed to call history API: %s", err)
	}
	defer resp.Body.Close()

	var history HistoryResponse
	if err := json.NewDecoder(resp.Body).Decode(&history); err != nil {
		t.Fatalf("failed to decode history response: %s", err)
	}
	return history
}

// call the reconstruct API for a specific revision
func reconstruct(t *testing.T, uid string, revision int) ReconstructResponse {
	t.Helper()
	url := fmt.Sprintf("http://localhost:9090/api/v1/resources/%s/reconstruct/%d", uid, revision)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("failed to call reconstruct API: %s", err)
	}
	defer resp.Body.Close()

	var result ReconstructResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode reconstruct response: %s", err)
	}
	return result
}
