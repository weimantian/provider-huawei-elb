// Package internalelb contains custom spec types for the internal-elb topology.
//
// +k8s:openapi-gen=true
package internalelb

// InternalElbTopologyConfig defines configuration for the internal-elb topology.
// This topology creates an internal ELB without a public IP.
// No additional configuration is needed beyond the component specs.
type InternalElbTopologyConfig struct{}
