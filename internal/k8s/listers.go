package k8s

import (
	apiextlisters "k8s.io/apiextensions-apiserver/pkg/client/listers/apiextensions/v1"
	appslisters "k8s.io/client-go/listers/apps/v1"
	autoscalinglisters "k8s.io/client-go/listers/autoscaling/v2"
	batchlisters "k8s.io/client-go/listers/batch/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	netlisters "k8s.io/client-go/listers/networking/v1"
	policylisters "k8s.io/client-go/listers/policy/v1"
	rbaclisters "k8s.io/client-go/listers/rbac/v1"
	storagelisters "k8s.io/client-go/listers/storage/v1"
)

// Lazy lister accessors. Each one starts its kind's informer on first use
// and then reads from the shared cache — so k10s only ever watches the
// resource kinds you actually look at, instead of listing the whole cluster
// (every secret, every event) at startup.
//
// These never block: if the cache hasn't synced yet the lister simply
// returns nothing, and the UI repaints when it fills in.

func (s *Store) podLister(ns ...string) corelisters.PodLister {
	return s.factoryFor(kPods, ns...).Core().V1().Pods().Lister()
}

func (s *Store) deployLister(ns ...string) appslisters.DeploymentLister {
	return s.factoryFor(kDeployments, ns...).Apps().V1().Deployments().Lister()
}

func (s *Store) stsLister(ns ...string) appslisters.StatefulSetLister {
	return s.factoryFor(kStatefulSet, ns...).Apps().V1().StatefulSets().Lister()
}

func (s *Store) dsLister(ns ...string) appslisters.DaemonSetLister {
	return s.factoryFor(kDaemonSets, ns...).Apps().V1().DaemonSets().Lister()
}

func (s *Store) jobLister(ns ...string) batchlisters.JobLister {
	return s.factoryFor(kJobs, ns...).Batch().V1().Jobs().Lister()
}

func (s *Store) cronLister(ns ...string) batchlisters.CronJobLister {
	return s.factoryFor(kCronJobs, ns...).Batch().V1().CronJobs().Lister()
}

func (s *Store) svcLister(ns ...string) corelisters.ServiceLister {
	return s.factoryFor(kServices, ns...).Core().V1().Services().Lister()
}

func (s *Store) ingLister(ns ...string) netlisters.IngressLister {
	return s.factoryFor(kIngresses, ns...).Networking().V1().Ingresses().Lister()
}

func (s *Store) cmLister(ns ...string) corelisters.ConfigMapLister {
	return s.factoryFor(kConfigMaps, ns...).Core().V1().ConfigMaps().Lister()
}

func (s *Store) secretLister(ns ...string) corelisters.SecretLister {
	return s.factoryFor(kSecrets, ns...).Core().V1().Secrets().Lister()
}

func (s *Store) pvcLister(ns ...string) corelisters.PersistentVolumeClaimLister {
	return s.factoryFor(kPVCs, ns...).Core().V1().PersistentVolumeClaims().Lister()
}

func (s *Store) nodeLister() corelisters.NodeLister {
	s.ensure(kNodes)
	return s.factory.Core().V1().Nodes().Lister()
}

func (s *Store) nsLister() corelisters.NamespaceLister {
	s.ensure(kNamespaces)
	return s.factory.Core().V1().Namespaces().Lister()
}

func (s *Store) eventLister(ns ...string) corelisters.EventLister {
	return s.factoryFor(kEvents, ns...).Core().V1().Events().Lister()
}

func (s *Store) crdLister() apiextlisters.CustomResourceDefinitionLister {
	s.ensure(kCRDs)
	return s.apiextFactory.Apiextensions().V1().CustomResourceDefinitions().Lister()
}

// ---- the kinds k9s reaches by :rs, :hpa, :sa, :pv … ------------------------

func (s *Store) rsLister(ns ...string) appslisters.ReplicaSetLister {
	return s.factoryFor(kReplicaSets, ns...).Apps().V1().ReplicaSets().Lister()
}

func (s *Store) hpaLister(ns ...string) autoscalinglisters.HorizontalPodAutoscalerLister {
	return s.factoryFor(kHPAs, ns...).Autoscaling().V2().HorizontalPodAutoscalers().Lister()
}

func (s *Store) endpointsLister(ns ...string) corelisters.EndpointsLister {
	return s.factoryFor(kEndpoints, ns...).Core().V1().Endpoints().Lister()
}

func (s *Store) netPolLister(ns ...string) netlisters.NetworkPolicyLister {
	return s.factoryFor(kNetPols, ns...).Networking().V1().NetworkPolicies().Lister()
}

func (s *Store) quotaLister(ns ...string) corelisters.ResourceQuotaLister {
	return s.factoryFor(kQuotas, ns...).Core().V1().ResourceQuotas().Lister()
}

func (s *Store) limitRangeLister(ns ...string) corelisters.LimitRangeLister {
	return s.factoryFor(kLimitRanges, ns...).Core().V1().LimitRanges().Lister()
}

func (s *Store) pdbLister(ns ...string) policylisters.PodDisruptionBudgetLister {
	return s.factoryFor(kPDBs, ns...).Policy().V1().PodDisruptionBudgets().Lister()
}

func (s *Store) pvLister() corelisters.PersistentVolumeLister {
	s.ensure(kPVs)
	return s.factory.Core().V1().PersistentVolumes().Lister()
}

func (s *Store) storageClassLister() storagelisters.StorageClassLister {
	s.ensure(kStorageCls)
	return s.factory.Storage().V1().StorageClasses().Lister()
}

func (s *Store) saLister(ns ...string) corelisters.ServiceAccountLister {
	return s.factoryFor(kSAs, ns...).Core().V1().ServiceAccounts().Lister()
}

func (s *Store) roleLister(ns ...string) rbaclisters.RoleLister {
	return s.factoryFor(kRoles, ns...).Rbac().V1().Roles().Lister()
}

func (s *Store) roleBindingLister(ns ...string) rbaclisters.RoleBindingLister {
	return s.factoryFor(kRoleBinds, ns...).Rbac().V1().RoleBindings().Lister()
}

func (s *Store) clusterRoleLister() rbaclisters.ClusterRoleLister {
	s.ensure(kClusterRole)
	return s.factory.Rbac().V1().ClusterRoles().Lister()
}

func (s *Store) clusterRoleBindingLister() rbaclisters.ClusterRoleBindingLister {
	s.ensure(kClusterBind)
	return s.factory.Rbac().V1().ClusterRoleBindings().Lister()
}
