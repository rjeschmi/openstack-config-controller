package controllers

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

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
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// OpenStackBlockStorageReconciler reconciles a OpenStackBlockStorage object
type OpenStackBlockStorageReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// ReconcileInterval is the default interval between reconciles when no
	// events are received. Can be overridden by the OPENSTACK_RECONCILE_INTERVAL
	// environment variable which accepts a Go duration string (e.g. "30s", "5m").
	ReconcileInterval time.Duration
	intervalMu        sync.RWMutex
}

//+kubebuilder:rbac:groups=openstack.ayr.ca,resources=openstackblockstorages,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=openstack.ayr.ca,resources=openstackblockstorages/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=openstack.ayr.ca,resources=openstackvolumes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=openstack.ayr.ca,resources=openstackvolumes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *OpenStackBlockStorageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	var inst openstackv1.OpenStackBlockStorage
	if err := r.Get(ctx, req.NamespacedName, &inst); err != nil {
		logger.Error(err, "unable to fetch OpenStackBlockStorage")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var provider *gophercloud.ProviderClient
	var err error

	// If a cloud config secret is specified, use it for authentication.
	if inst.Spec.Cloud != "" && inst.Spec.CloudConfig.Name != "" {
		var secret corev1.Secret
		secretKey := client.ObjectKey{Namespace: inst.Namespace, Name: inst.Spec.CloudConfig.Name}
		if err := r.Get(ctx, secretKey, &secret); err != nil {
			logger.Error(err, "unable to fetch cloud config secret", "secret", secretKey)
			return ctrl.Result{}, err
		}

		cloudsYAML, ok := secret.Data[inst.Spec.CloudConfig.Key]
		if !ok {
			err := fmt.Errorf("cloud config secret %q does not contain key %q", inst.Spec.CloudConfig.Name, inst.Spec.CloudConfig.Key)
			logger.Error(err, "invalid cloud config secret")
			return ctrl.Result{}, err
		}

		// The gophercloud.AuthOptionsFromEnv() still reads some variables, so
		// we must unset them to avoid conflicts.
		os.Unsetenv("OS_AUTH_URL")
		os.Unsetenv("OS_USERNAME")
		os.Unsetenv("OS_PASSWORD")
		os.Unsetenv("OS_PROJECT_NAME")
		os.Unsetenv("OS_PROJECT_ID")
		os.Unsetenv("OS_DOMAIN_NAME")
		os.Unsetenv("OS_DOMAIN_ID")

		authOpts, err := openstack.AuthOptionsFromYAML(cloudsYAML, inst.Spec.Cloud)
		if err != nil {
			logger.Error(err, "failed to parse auth options from clouds.yaml")
			return ctrl.Result{}, err
		}

		provider, err = openstack.AuthenticatedClient(authOpts)
		if err != nil {
			logger.Error(err, "failed to authenticate to OpenStack using clouds.yaml")
			return ctrl.Result{}, err
		}
		logger.Info("authenticated using clouds.yaml from secret", "cloud", inst.Spec.Cloud)
	} else {
		// Fallback to environment variables
		ao, err := openstack.AuthOptionsFromEnv()
		if err != nil {
			logger.Error(err, "auth options from env failed")
			return ctrl.Result{}, err
		}
		provider, err = openstack.AuthenticatedClient(ao)
		if err != nil {
			logger.Error(err, "failed to authenticate to OpenStack")
			return ctrl.Result{}, err
		}
		logger.Info("authenticated using environment variables")
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
	var allVolumes []volumesv3.Volume
	err = pager.EachPage(func(page pagination.Page) (bool, error) {
		vols, err := volumesv3.ExtractVolumes(page)
		if err != nil {
			return false, err
		}
		allVolumes = append(allVolumes, vols...)
		return true, nil
	})
	if err != nil {
		logger.Error(err, "error listing volumes")
		return ctrl.Result{}, fmt.Errorf("error listing volumes: %w", err)
	}

	logger.Info("volumes parsed by gophercloud", "count", len(allVolumes))

	var found []openstackv1.VolumeStatus
	for _, v := range allVolumes {
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

	// Reconcile OpenStackVolume objects
	desired := map[string]struct{}{}
	for _, v := range allVolumes {
		name := fmt.Sprintf("%s-%s", inst.Name, v.ID)
		desired[name] = struct{}{}

		var osv openstackv1.OpenStackVolume
		err := r.Get(ctx, client.ObjectKey{Namespace: inst.Namespace, Name: name}, &osv)
		if err != nil {
			if apierrors.IsNotFound(err) {
				osv = openstackv1.OpenStackVolume{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: inst.Namespace,
						Labels: map[string]string{
							"openstack.ayr.ca/owner": inst.Name,
						},
					},
					Spec: openstackv1.OpenStackVolumeSpec{
						ID:               v.ID,
						Name:             v.Name,
						Status:           v.Status,
						Size:             v.Size,
						AvailabilityZone: v.AvailabilityZone,
						CreatedAt:        metav1.NewTime(v.CreatedAt),
						VolumeType:       v.VolumeType,
						Bootable:         v.Bootable,
						Encrypted:        v.Encrypted,
					},
				}
				if err := controllerutil.SetControllerReference(&inst, &osv, r.Scheme); err != nil {
					logger.Error(err, "failed to set owner reference on OpenStackVolume")
					return ctrl.Result{}, err
				}
				if err := r.Create(ctx, &osv); err != nil {
					logger.Error(err, "failed to create OpenStackVolume", "name", name)
					return ctrl.Result{}, err
				}
				logger.Info("created OpenStackVolume", "name", name)
				continue
			}
			logger.Error(err, "failed to get OpenStackVolume")
			return ctrl.Result{}, err
		}

		// Update existing OpenStackVolume if data changed
		desiredSpec := openstackv1.OpenStackVolumeSpec{
			ID:               v.ID,
			Name:             v.Name,
			Status:           v.Status,
			Size:             v.Size,
			AvailabilityZone: v.AvailabilityZone,
			CreatedAt:        metav1.NewTime(v.CreatedAt),
			VolumeType:       v.VolumeType,
			Bootable:         v.Bootable,
			Encrypted:        v.Encrypted,
		}

		if osv.Spec.ID != desiredSpec.ID ||
			osv.Spec.Name != desiredSpec.Name ||
			osv.Spec.Status != desiredSpec.Status ||
			osv.Spec.Size != desiredSpec.Size ||
			osv.Spec.AvailabilityZone != desiredSpec.AvailabilityZone ||
			!osv.Spec.CreatedAt.Equal(&desiredSpec.CreatedAt) ||
			osv.Spec.VolumeType != desiredSpec.VolumeType ||
			osv.Spec.Bootable != desiredSpec.Bootable ||
			osv.Spec.Encrypted != desiredSpec.Encrypted {

			osv.Spec = desiredSpec
			if err := r.Update(ctx, &osv); err != nil {
				logger.Error(err, "failed to update OpenStackVolume", "name", name)
				return ctrl.Result{}, err
			}
			logger.Info("updated OpenStackVolume", "name", name)
		}
	}

	// Clean up OpenStackVolumes that no longer correspond to volumes
	var osvList openstackv1.OpenStackVolumeList
	if err := r.List(ctx, &osvList, client.InNamespace(inst.Namespace), client.MatchingLabels{"openstack.ayr.ca/owner": inst.Name}); err != nil {
		logger.Error(err, "failed to list OpenStackVolumes for cleanup")
		return ctrl.Result{}, err
	}
	for _, item := range osvList.Items {
		if _, ok := desired[item.Name]; !ok {
			if err := r.Delete(ctx, &item); err != nil {
				logger.Error(err, "failed to delete stale OpenStackVolume", "name", item.Name)
				return ctrl.Result{}, err
			}
			logger.Info("deleted stale OpenStackVolume", "name", item.Name)
		}
	}

	inst.Status.Volumes = found
	if err := r.Status().Update(ctx, &inst); err != nil {
		logger.Error(err, "failed to update status")
		return ctrl.Result{}, err
	}

	// Compute requeue interval (may be overridden by env var). Read the
	// reconciler's configured interval under a read lock so runtime updates
	// from the ConfigMap watch are safe.
	r.intervalMu.RLock()
	current := r.ReconcileInterval
	r.intervalMu.RUnlock()
	requeueAfter, perr := ComputeRequeueAfter(current)
	if perr != nil {
		logger.Info("invalid OPENSTACK_RECONCILE_INTERVAL, using default", "err", perr.Error())
	}
	logger.Info("scheduling next reconcile", "after", requeueAfter.String())
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// ComputeRequeueAfter returns the duration to use for requeueing reconciles.
// If the `OPENSTACK_RECONCILE_INTERVAL` environment variable is set and is a
// valid duration, that value is returned. Otherwise the provided
// `defaultInterval` is returned; if that is zero, a 5-minute default is used.
func ComputeRequeueAfter(defaultInterval time.Duration) (time.Duration, error) {
	if s := os.Getenv("OPENSTACK_RECONCILE_INTERVAL"); s != "" {
		d, err := time.ParseDuration(s)
		if err != nil {
			if defaultInterval == 0 {
				return 5 * time.Minute, err
			}
			return defaultInterval, err
		}
		return d, nil
	}
	if defaultInterval == 0 {
		return 5 * time.Minute, nil
	}
	return defaultInterval, nil
}

func (r *OpenStackBlockStorageReconciler) SetupWithManager(mgr ctrl.Manager) error {
	builder := ctrl.NewControllerManagedBy(mgr).
		For(&openstackv1.OpenStackBlockStorage{}).
		Owns(&openstackv1.OpenStackVolume{})

	// Optionally watch a ConfigMap for dynamic reconcile interval updates.
	// If `OPENSTACK_RECONCILE_CONFIGMAP_NAME` is set, the controller will
	// watch ConfigMaps and update its `ReconcileInterval` when the key
	// `reconcile-interval` is present. Optionally restrict namespace via
	// `OPENSTACK_RECONCILE_CONFIGMAP_NAMESPACE` (empty => watch all namespaces).
	if cmName := os.Getenv("OPENSTACK_RECONCILE_CONFIGMAP_NAME"); cmName != "" {
		cmNs := os.Getenv("OPENSTACK_RECONCILE_CONFIGMAP_NAMESPACE")
		builder = builder.Watches(&source.Kind{Type: &corev1.ConfigMap{}}, handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
			cm, ok := obj.(*corev1.ConfigMap)
			if !ok {
				return nil
			}
			if cm.Name != cmName {
				return nil
			}
			if cmNs != "" && cm.Namespace != cmNs {
				return nil
			}

			// If the ConfigMap contains a `reconcile-interval` entry, try to parse
			// and update the reconciler's interval.
			if s, ok := cm.Data["reconcile-interval"]; ok {
				if d, err := time.ParseDuration(s); err == nil {
					r.intervalMu.Lock()
					r.ReconcileInterval = d
					r.intervalMu.Unlock()
				}
			}

			// Enqueue all OpenStackBlockStorage resources for reconciliation so
			// they pick up the new interval promptly.
			var list openstackv1.OpenStackBlockStorageList
			if err := r.List(context.Background(), &list); err != nil {
				return nil
			}
			reqs := make([]reconcile.Request, 0, len(list.Items))
			for _, item := range list.Items {
				reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKey{Namespace: item.Namespace, Name: item.Name}})
			}
			return reqs
		}))
	}

	return builder.Complete(r)
}
