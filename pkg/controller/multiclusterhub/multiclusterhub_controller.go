package multiclusterhub

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	operatorsv1alpha1 "github.com/open-cluster-management/multicloudhub-operator/pkg/apis/operators/v1alpha1"
	"github.com/open-cluster-management/multicloudhub-operator/pkg/deploying"
	"github.com/open-cluster-management/multicloudhub-operator/pkg/rendering"
)

var log = logf.Log.WithName("controller_multiclusterhub")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new MultiClusterHub Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileMultiClusterHub{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("multiclusterhub-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource MultiClusterHub
	err = c.Watch(&source.Kind{Type: &operatorsv1alpha1.MultiClusterHub{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Pods and requeue the owner MultiClusterHub
	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &operatorsv1alpha1.MultiClusterHub{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileMultiClusterHub implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileMultiClusterHub{}

// ReconcileMultiClusterHub reconciles a MultiClusterHub object
type ReconcileMultiClusterHub struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a MultiClusterHub object and makes changes based on the state read
// and what is in the MultiClusterHub.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileMultiClusterHub) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling MultiClusterHub")

	// Fetch the MultiClusterHub instance
	multiClusterHub := &operatorsv1alpha1.MultiClusterHub{}
	err := r.client.Get(context.TODO(), request.NamespacedName, multiClusterHub)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Info("MultiClusterHub resource not found. Ignoring since object must be deleted")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		reqLogger.Error(err, "Failed to get MultiClusterHub CR")
		return reconcile.Result{}, err
	}

	var result *reconcile.Result
	result, err = r.ensureSecret(request, multiClusterHub, r.mongoAuthSecret(multiClusterHub))
	if result != nil {
		return *result, err
	}

	//Render the templates with a specified CR
	renderer := rendering.NewRenderer(multiClusterHub)
	toDeploy, err := renderer.Render()
	if err != nil {
		reqLogger.Error(err, "Failed to render MultiClusterHub templates")
		return reconcile.Result{}, err
	}
	//Deploy the resources
	for _, res := range toDeploy {
		if err := controllerutil.SetControllerReference(multiClusterHub, res, r.scheme); err != nil {
			reqLogger.Error(err, "Failed to set controller reference")
		}
		if err := deploying.Deploy(r.client, res); err != nil {
			reqLogger.Error(err, fmt.Sprintf("Failed to deploy %s %s/%s", res.GetKind(), multiClusterHub.Namespace, res.GetName()))
			return reconcile.Result{}, err
		}
	}

	// Update the CR status
	multiClusterHub.Status.Phase = "Failed"
	ready, deployments, err := deploying.ListDeployments(r.client, multiClusterHub.Namespace)
	if err != nil {
		return reconcile.Result{}, err
	}
	if ready {
		multiClusterHub.Status.Phase = "Running"
	}
	statedDeployments := []operatorsv1alpha1.DeploymentResult{}
	for _, deploy := range deployments {
		statedDeployments = append(statedDeployments, operatorsv1alpha1.DeploymentResult{
			Name:   deploy.Name,
			Status: deploy.Status,
		})
	}
	multiClusterHub.Status.Deployments = statedDeployments
	err = r.client.Status().Update(context.TODO(), multiClusterHub)
	if err != nil {
		reqLogger.Error(err, fmt.Sprintf("Failed to update %s/%s status ", multiClusterHub.Namespace, multiClusterHub.Name))
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileMultiClusterHub) ensureSecret(request reconcile.Request,
	instance *operatorsv1alpha1.MultiClusterHub,
	s *corev1.Secret,
) (*reconcile.Result, error) {
	found := &corev1.Secret{}
	err := r.client.Get(context.TODO(), types.NamespacedName{
		Name:      s.Name,
		Namespace: instance.Namespace,
	}, found)
	if err != nil && errors.IsNotFound(err) {
		// Create the secret
		log.Info("Creating a new secret", "Secret.Namespace", s.Namespace, "Secret.Name", s.Name)
		err = r.client.Create(context.TODO(), s)

		if err != nil {
			// Creation failed
			log.Error(err, "Failed to create new Secret", "Secret.Namespace", s.Namespace, "Secret.Name", s.Name)
			return &reconcile.Result{}, err
		}
		// Creation was successful
		return nil, nil

	} else if err != nil {
		// Error that isn't due to the secret not existing
		log.Error(err, "Failed to get Secret")
		return &reconcile.Result{}, err
	}

	return nil, nil
}

func (r *ReconcileMultiClusterHub) mongoAuthSecret(v *operatorsv1alpha1.MultiClusterHub) *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mongodb-admin",
			Namespace: v.Namespace,
		},
		Type: "Opaque",
		StringData: map[string]string{
			"user":     "some@example.com",
			"password": generatePass(16),
		},
	}

	if err := controllerutil.SetControllerReference(v, secret, r.scheme); err != nil {
		log.Error(err, "Failed to set controller reference", "Secret.Namespace", v.Namespace, "Secret.Name", v.Name)
	}
	return secret
}

func generatePass(length int) string {
	chars := "ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
		"abcdefghijklmnopqrstuvwxyz" +
		"0123456789"

	buf := make([]byte, length)
	for i := 0; i < length; i++ {
		nBig, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		buf[i] = chars[nBig.Int64()]
	}
	return string(buf)
}