package controllers

import (
	"context"
	"fmt"
	"io"

	"os"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	volumesv3 "github.com/gophercloud/gophercloud/openstack/blockstorage/v3/volumes"
	openstackutils "github.com/gophercloud/gophercloud/openstack/utils"
	"github.com/gophercloud/gophercloud/pagination"
	openstackv1 "github.com/rjeschmi/openstack-config-controller/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// OpenStackBlockStorageReconciler reconciles a OpenStackBlockStorage object
type OpenStackBlockStorageReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=openstack.ayr.ca,resources=openstackblockstorages,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=openstack.ayr.ca,resources=openstackblockstorages/status,verbs=get;update;patch

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

	region := os.Getenv("OS_REGION_NAME")
	availability := os.Getenv("OS_INTERFACE")
	logger.Info("OpenStack endpoint selection", "region", region, "interface", availability)

	var c *gophercloud.ServiceClient
	// If a specific endpoint is provided via env, use it directly (debug/override)
	if ep := os.Getenv("OS_BLOCKSTORAGE_ENDPOINT"); ep != "" {
		logger.Info("using OS_BLOCKSTORAGE_ENDPOINT override", "endpoint", ep)
		c = &gophercloud.ServiceClient{ProviderClient: provider, Endpoint: ep}
	}

	// Try creating the block-storage client with several fallbacks while logging errors
	var lastErr error

	try := func(opts gophercloud.EndpointOpts) error {
		sc, err := openstack.NewBlockStorageV3(provider, opts)
		if err != nil {
			return err
		}
		c = sc
		return nil
	}

	// If an override endpoint was provided we already set `c` above; only run fallbacks when not set
	if c == nil {
		lastErr = try(gophercloud.EndpointOpts{Region: region, Availability: gophercloud.Availability(availability)})
		if lastErr != nil {
			logger.Info("primary endpoint selection failed, trying region-only", "err", lastErr.Error())
			// Fallback 1: region only
			lastErr = try(gophercloud.EndpointOpts{Region: region})
		}
		if lastErr != nil {
			logger.Info("region-only selection failed, trying default endpoint resolution", "err", lastErr.Error())
			// Fallback 2: no endpoint opts (use provider/catalog defaults)
			lastErr = try(gophercloud.EndpointOpts{})
		}

		// If gophercloud couldn't find the endpoint (some clouds use "block-storage"
		// instead of gophercloud's expected "volumev3" service type), try locating
		// the endpoint explicitly from the provider's catalog using the alternative
		// service type and construct a ServiceClient manually.
		if lastErr != nil {
			// Try alternate service type used by some deployments
			altOpts := gophercloud.EndpointOpts{Type: "block-storage", Region: region, Availability: gophercloud.Availability(availability)}
			if ep, err := provider.EndpointLocator(altOpts); err == nil {
				// Ensure base and v3 suffix match gophercloud expectations
				base, berr := openstackutils.BaseEndpoint(ep)
				if berr != nil {
					logger.Info("alternate endpoint found but base extraction failed", "endpoint", ep, "err", berr.Error())
				} else {
					endpoint := gophercloud.NormalizeURL(base) + "v3/"
					logger.Info("using alternate block-storage endpoint from catalog", "endpoint", endpoint)
					c = &gophercloud.ServiceClient{ProviderClient: provider, Endpoint: endpoint, Type: "volumev3"}
					lastErr = nil
				}
			} else {
				logger.Info("alternate service-type lookup failed", "err", err.Error())
			}
		}

		if lastErr != nil {
			logger.Error(lastErr, "failed to create block storage client after fallbacks")
			return ctrl.Result{}, lastErr
		}
	}

	listOpts := volumesv3.ListOpts{}
	// Debug: log constructed service client details before listing volumes
	if c != nil {
		logger.Info("blockstorage client info", "Endpoint", c.Endpoint, "ServiceURL_volumes_detail", c.ServiceURL("volumes", "detail"), "Type", c.Type)

		// Log provider token length (do not log token contents)
		tok := provider.Token()
		logger.Info("provider token", "length", len(tok))

		// Raw HTTP GET to the computed service URL to capture status/body for debugging
		rawURL := c.ServiceURL("volumes", "detail")
		resp, rerr := c.Get(rawURL, nil, &gophercloud.RequestOpts{KeepResponseBody: true})
		if rerr != nil {
			logger.Info("raw GET to volumes detail failed", "url", rawURL, "err", rerr.Error())
		} else {
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			logger.Info("raw GET response", "url", rawURL, "status", resp.StatusCode, "contentLength", resp.ContentLength, "headers", fmt.Sprintf("%v", resp.Header), "bodyLen", len(body), "body", string(body))
		}
	} else {
		logger.Info("blockstorage client is nil before listing volumes")
	}
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

	logger.Info("volumes parsed by gophercloud", "count", len(found))

	// Reconcile ConfigMaps representing each OpenStack volume
	desired := map[string]struct{}{}
	for _, v := range found {
		name := fmt.Sprintf("%s-%s", inst.Name, v.ID)
		desired[name] = struct{}{}

		var cm corev1.ConfigMap
		err := r.Get(ctx, client.ObjectKey{Namespace: inst.Namespace, Name: name}, &cm)
		if err != nil {
			if apierrors.IsNotFound(err) {
				cm = corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: inst.Namespace,
						Labels: map[string]string{
							"openstack.example.com/owner": inst.Name,
						},
					},
					Data: map[string]string{},
				}
				cm.Data["id"] = v.ID
				cm.Data["name"] = v.Name
				cm.Data["status"] = v.Status
				cm.Data["sizeGB"] = fmt.Sprintf("%d", v.SizeGB)
				if err := controllerutil.SetControllerReference(&inst, &cm, r.Scheme); err != nil {
					logger.Error(err, "failed to set owner reference on ConfigMap")
					return ctrl.Result{}, err
				}
				if err := r.Create(ctx, &cm); err != nil {
					logger.Error(err, "failed to create ConfigMap for volume", "name", name)
					return ctrl.Result{}, err
				}
				logger.Info("created ConfigMap for volume", "name", name)
				continue
			}
			logger.Error(err, "failed to get ConfigMap")
			return ctrl.Result{}, err
		}

		// Update existing ConfigMap if data changed
		changed := false
		if cm.Data == nil {
			cm.Data = map[string]string{}
			changed = true
		}
		if cm.Data["id"] != v.ID {
			cm.Data["id"] = v.ID
			changed = true
		}
		if cm.Data["name"] != v.Name {
			cm.Data["name"] = v.Name
			changed = true
		}
		if cm.Data["status"] != v.Status {
			cm.Data["status"] = v.Status
			changed = true
		}
		sizeStr := fmt.Sprintf("%d", v.SizeGB)
		if cm.Data["sizeGB"] != sizeStr {
			cm.Data["sizeGB"] = sizeStr
			changed = true
		}
		if changed {
			if err := r.Update(ctx, &cm); err != nil {
				logger.Error(err, "failed to update ConfigMap for volume", "name", name)
				return ctrl.Result{}, err
			}
			logger.Info("updated ConfigMap for volume", "name", name)
		}
	}

	// Clean up ConfigMaps that no longer correspond to volumes
	var cmList corev1.ConfigMapList
	if err := r.List(ctx, &cmList, client.InNamespace(inst.Namespace), client.MatchingLabels{"openstack.example.com/owner": inst.Name}); err != nil {
		logger.Error(err, "failed to list ConfigMaps for cleanup")
		return ctrl.Result{}, err
	}
	for _, cm := range cmList.Items {
		if _, ok := desired[cm.Name]; !ok {
			if err := r.Delete(ctx, &cm); err != nil {
				logger.Error(err, "failed to delete stale ConfigMap", "name", cm.Name)
				return ctrl.Result{}, err
			}
			logger.Info("deleted stale ConfigMap", "name", cm.Name)
		}
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
