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

func (s *Store) podLister() corelisters.PodLister {
	s.ensure(kPods)
	return s.factory.Core().V1().Pods().Lister()
}

func (s *Store) deployLister() appslisters.DeploymentLister {
	s.ensure(kDeployments)
	return s.factory.Apps().V1().Deployments().Lister()
}

func (s *Store) stsLister() appslisters.StatefulSetLister {
	s.ensure(kStatefulSet)
	return s.factory.Apps().V1().StatefulSets().Lister()
}

func (s *Store) dsLister() appslisters.DaemonSetLister {
	s.ensure(kDaemonSets)
	return s.factory.Apps().V1().DaemonSets().Lister()
}

func (s *Store) jobLister() batchlisters.JobLister {
	s.ensure(kJobs)
	return s.factory.Batch().V1().Jobs().Lister()
}

func (s *Store) cronLister() batchlisters.CronJobLister {
	s.ensure(kCronJobs)
	return s.factory.Batch().V1().CronJobs().Lister()
}

func (s *Store) svcLister() corelisters.ServiceLister {
	s.ensure(kServices)
	return s.factory.Core().V1().Services().Lister()
}

func (s *Store) ingLister() netlisters.IngressLister {
	s.ensure(kIngresses)
	return s.factory.Networking().V1().Ingresses().Lister()
}

func (s *Store) cmLister() corelisters.ConfigMapLister {
	s.ensure(kConfigMaps)
	return s.factory.Core().V1().ConfigMaps().Lister()
}

func (s *Store) secretLister() corelisters.SecretLister {
	s.ensure(kSecrets)
	return s.factory.Core().V1().Secrets().Lister()
}

func (s *Store) pvcLister() corelisters.PersistentVolumeClaimLister {
	s.ensure(kPVCs)
	return s.factory.Core().V1().PersistentVolumeClaims().Lister()
}

func (s *Store) nodeLister() corelisters.NodeLister {
	s.ensure(kNodes)
	return s.factory.Core().V1().Nodes().Lister()
}

func (s *Store) nsLister() corelisters.NamespaceLister {
	s.ensure(kNamespaces)
	return s.factory.Core().V1().Namespaces().Lister()
}

func (s *Store) eventLister() corelisters.EventLister {
	s.ensure(kEvents)
	return s.factory.Core().V1().Events().Lister()
}

func (s *Store) crdLister() apiextlisters.CustomResourceDefinitionLister {
	s.ensure(kCRDs)
	return s.apiextFactory.Apiextensions().V1().CustomResourceDefinitions().Lister()
}

// ---- the kinds k9s reaches by :rs, :hpa, :sa, :pv … ------------------------

func (s *Store) rsLister() appslisters.ReplicaSetLister {
	s.ensure(kReplicaSets)
	return s.factory.Apps().V1().ReplicaSets().Lister()
}

func (s *Store) hpaLister() autoscalinglisters.HorizontalPodAutoscalerLister {
	s.ensure(kHPAs)
	return s.factory.Autoscaling().V2().HorizontalPodAutoscalers().Lister()
}

func (s *Store) endpointsLister() corelisters.EndpointsLister {
	s.ensure(kEndpoints)
	return s.factory.Core().V1().Endpoints().Lister()
}

func (s *Store) netPolLister() netlisters.NetworkPolicyLister {
	s.ensure(kNetPols)
	return s.factory.Networking().V1().NetworkPolicies().Lister()
}

func (s *Store) quotaLister() corelisters.ResourceQuotaLister {
	s.ensure(kQuotas)
	return s.factory.Core().V1().ResourceQuotas().Lister()
}

func (s *Store) limitRangeLister() corelisters.LimitRangeLister {
	s.ensure(kLimitRanges)
	return s.factory.Core().V1().LimitRanges().Lister()
}

func (s *Store) pdbLister() policylisters.PodDisruptionBudgetLister {
	s.ensure(kPDBs)
	return s.factory.Policy().V1().PodDisruptionBudgets().Lister()
}

func (s *Store) pvLister() corelisters.PersistentVolumeLister {
	s.ensure(kPVs)
	return s.factory.Core().V1().PersistentVolumes().Lister()
}

func (s *Store) storageClassLister() storagelisters.StorageClassLister {
	s.ensure(kStorageCls)
	return s.factory.Storage().V1().StorageClasses().Lister()
}

func (s *Store) saLister() corelisters.ServiceAccountLister {
	s.ensure(kSAs)
	return s.factory.Core().V1().ServiceAccounts().Lister()
}

func (s *Store) roleLister() rbaclisters.RoleLister {
	s.ensure(kRoles)
	return s.factory.Rbac().V1().Roles().Lister()
}

func (s *Store) roleBindingLister() rbaclisters.RoleBindingLister {
	s.ensure(kRoleBinds)
	return s.factory.Rbac().V1().RoleBindings().Lister()
}

func (s *Store) clusterRoleLister() rbaclisters.ClusterRoleLister {
	s.ensure(kClusterRole)
	return s.factory.Rbac().V1().ClusterRoles().Lister()
}

func (s *Store) clusterRoleBindingLister() rbaclisters.ClusterRoleBindingLister {
	s.ensure(kClusterBind)
	return s.factory.Rbac().V1().ClusterRoleBindings().Lister()
}
