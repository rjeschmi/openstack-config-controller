package controllers

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	volumesv3 "github.com/gophercloud/gophercloud/openstack/blockstorage/v3/volumes"
	"github.com/gophercloud/gophercloud/pagination"
	openstackv1 "github.com/rjeschmi/openstack-config-controller/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// OpenStackBlockStorageReconciler reconciles a OpenStackBlockStorage object
type OpenStackBlockStorageReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=openstack.example.com,resources=openstackblockstorages,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=openstack.example.com,resources=openstackblockstorages/status,verbs=get;update;patch

func (r *OpenStackBlockStorageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	var inst openstackv1.OpenStackBlockStorage
	if err := r.Get(ctx, req.NamespacedName, &inst); err != nil {
		logger.Error(err, "unable to fetch OpenStackBlockStorage")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Authenticate to OpenStack using env vars (OS_AUTH_URL, OS_USERNAME, etc.)
	ao, err := openstack.AuthOptionsFromEnv()
	if err != nil {
		logger.Error(err, "auth options from env failed")
		return ctrl.Result{}, err
	}
	provider, err := openstack.AuthenticatedClient(ao)
	if err != nil {
		logger.Error(err, "failed to authenticate to OpenStack")
		return ctrl.Result{}, err
	}

	c, err := openstack.NewBlockStorageV3(provider, gophercloud.EndpointOpts{})
	if err != nil {
		logger.Error(err, "failed to create block storage client")
		return ctrl.Result{}, err
	}

	listOpts := volumesv3.ListOpts{}
	pager := volumesv3.List(c, listOpts)
	var found []openstackv1.VolumeStatus
	err = pager.EachPage(func(page pagination.Page) (bool, error) {
		vols, err := volumesv3.ExtractVolumes(page)
		if err != nil {
			return false, err
		}
		for _, v := range vols {
			vsEntry := openstackv1.VolumeStatus{
				ID:     v.ID,
				Name:   v.Name,
				Status: v.Status,
			}
			if v.Size != 0 {
				vsEntry.SizeGB = v.Size
			}
			found = append(found, vsEntry)
		}
		return true, nil
	})
	if err != nil {
		logger.Error(err, "error listing volumes")
		return ctrl.Result{}, fmt.Errorf("error listing volumes: %w", err)
	}

	inst.Status.Volumes = found
	if err := r.Status().Update(ctx, &inst); err != nil {
		logger.Error(err, "failed to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *OpenStackBlockStorageReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&openstackv1.OpenStackBlockStorage{}).
		Complete(r)
}
