// Package common defines shared constants used across the provider.
package common

const (
	// ProviderName is the canonical name of this provider.
	ProviderName = "provider-huawei-elb"

	// Component names (matching definition/provider.yaml keys).
	ComponentElbEngine   = "elbEngine"
	ComponentElbListener = "elbListener"

	// Component type names (matching definition/versions.yaml keys).
	ComponentTypeElbEngine   = "elb-engine"
	ComponentTypeElbListener = "elb-listener"

	// Topology names.
	TopologyPublicELB   = "public-elb"
	TopologyInternalELB = "internal-elb"

	// Kubernetes annotation for CCE ELB integration — binds a pre-created ELB.
	AnnotationELBID = "kubernetes.io/elb.id"

	// Labels for tracking Instance-owned resources.
	LabelInstance = "openeverest.io/instance"
	LabelProvider = "openeverest.io/provider"

	// Default values for listener config.
	DefaultProtocol    = "TCP"
	DefaultPort        = 3306
	DefaultBackendPort = 3306

	// ELBNamePrefix is prepended to the instance name to form the ELB name.
	ELBNamePrefix = "elb-"
)
