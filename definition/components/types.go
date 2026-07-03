// Package components contains custom spec types for provider component types.
//
// Each struct here corresponds to a component type defined in versions.yaml
// and is converted to an OpenAPI schema during generation.
//
// +k8s:openapi-gen=true
package components

// ElbEngineCustomSpec defines custom configuration for elb-engine components.
// It holds the network parameters required to create a Huawei Cloud ELB.
type ElbEngineCustomSpec struct {
	// VpcID is the Huawei Cloud VPC ID where the ELB will be created.
	VpcID string `json:"vpcId"`

	// VipSubnetCidrID is the IPv4 subnet ID (neutron_subnet_id) for the ELB VIP.
	VipSubnetCidrID string `json:"vipSubnetCidrId"`

	// AvailabilityZoneList specifies one or more availability zones for the ELB.
	// At least one zone is required; two or more are recommended for HA.
	AvailabilityZoneList []string `json:"availabilityZoneList"`
}

// ElbListenerCustomSpec defines custom configuration for elb-listener components.
// It defines the protocol and port mapping for the ELB listener.
type ElbListenerCustomSpec struct {
	// Protocol is the listener protocol: TCP, HTTP, or HTTPS.
	Protocol string `json:"protocol"`

	// Port is the front-end port the ELB listens on.
	Port int32 `json:"port"`

	// BackendPort is the target port on the backend pods.
	BackendPort int32 `json:"backendPort"`
}
