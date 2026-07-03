// Package publicelb contains custom spec types for the public-elb topology.
//
// +k8s:openapi-gen=true
package publicelb

// PublicElbTopologyConfig defines configuration for the public-elb topology.
// This topology creates a public-facing ELB with an EIP and bandwidth.
type PublicElbTopologyConfig struct {
	// BandwidthSize is the bandwidth in Mbit/s for the EIP (1-2000).
	// Defaults to 10 if unset.
	BandwidthSize int32 `json:"bandwidthSize,omitempty"`

	// BandwidthChargeMode is the billing mode: "traffic" or "bandwidth".
	// Defaults to "traffic" (pay-per-traffic).
	BandwidthChargeMode string `json:"bandwidthChargeMode,omitempty"`

	// PublicIPNetworkType is the EIP network type, e.g. "5_bgp".
	// Defaults to "5_bgp" (BGP).
	PublicIPNetworkType string `json:"publicIpNetworkType,omitempty"`
}
