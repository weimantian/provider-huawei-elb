package provider

import (
	"fmt"

	"github.com/openeverest/openeverest/v2/provider-runtime/controller"

	"github.com/openeverest/provider-huawei-elb/definition/components"
	"github.com/openeverest/provider-huawei-elb/definition/topologies/public-elb"
	"github.com/openeverest/provider-huawei-elb/internal/common"
)

// ELBConfig is the fully resolved configuration for ELB operations.
// It is built from the Instance spec, topology config, and component custom specs.
type ELBConfig struct {
	// ELB network settings (from elbEngine custom spec).
	VpcID                string
	VipSubnetCidrID      string
	AvailabilityZoneList []string

	// Listener settings (from elbListener custom spec).
	Protocol    string
	Port        int32
	BackendPort int32

	// Topology settings (from topology config).
	IsPublicELB         bool
	BandwidthSize       int32
	BandwidthChargeMode string
	PublicIPNetworkType string

	// Instance info.
	InstanceName string
	Namespace    string
	ELBName      string
}

// ResolveConfig builds an ELBConfig from the Instance spec.
// It reads topology config, component custom specs, and applies defaults.
func ResolveConfig(c *controller.Context) (*ELBConfig, error) {
	cfg := &ELBConfig{
		InstanceName: c.Name(),
		Namespace:    c.Namespace(),
		ELBName:      common.ELBNamePrefix + c.Name(),
	}

	// Determine topology type and decode topology config.
	topoType := ""
	if c.Spec().Topology != nil {
		topoType = c.Spec().Topology.Type
	}
	switch topoType {
	case common.TopologyPublicELB:
		cfg.IsPublicELB = true
		var topoConfig publicelb.PublicElbTopologyConfig
		if err := c.DecodeTopologyConfig(&topoConfig); err != nil {
			return nil, fmt.Errorf("decoding topology config: %w", err)
		}
		cfg.BandwidthSize = topoConfig.BandwidthSize
		cfg.BandwidthChargeMode = topoConfig.BandwidthChargeMode
		cfg.PublicIPNetworkType = topoConfig.PublicIPNetworkType
	case common.TopologyInternalELB:
		cfg.IsPublicELB = false
		// InternalElbTopologyConfig is empty — no fields to decode.
		// The type exists for schema consistency with the topology.yaml configSchema.
	default:
		return nil, fmt.Errorf("unknown topology type %q (expected %q or %q)",
			topoType, common.TopologyPublicELB, common.TopologyInternalELB)
	}

	// Decode elbEngine custom spec.
	engineComps := c.ComponentsOfType(common.ComponentTypeElbEngine)
	if len(engineComps) == 0 {
		return nil, fmt.Errorf("elb-engine component not found in instance spec")
	}
	var engineSpec components.ElbEngineCustomSpec
	if err := c.DecodeComponentCustomSpec(engineComps[0], &engineSpec); err != nil {
		return nil, fmt.Errorf("decoding elbEngine custom spec: %w", err)
	}
	cfg.VpcID = engineSpec.VpcID
	cfg.VipSubnetCidrID = engineSpec.VipSubnetCidrID
	cfg.AvailabilityZoneList = engineSpec.AvailabilityZoneList

	// Decode elbListener custom spec (optional — uses defaults if absent).
	listenerComps := c.ComponentsOfType(common.ComponentTypeElbListener)
	if len(listenerComps) == 0 {
		cfg.Protocol = common.DefaultProtocol
		cfg.Port = common.DefaultPort
		cfg.BackendPort = common.DefaultBackendPort
	} else {
		var listenerSpec components.ElbListenerCustomSpec
		if err := c.DecodeComponentCustomSpec(listenerComps[0], &listenerSpec); err != nil {
			return nil, fmt.Errorf("decoding elbListener custom spec: %w", err)
		}
		cfg.Protocol = withDefault(listenerSpec.Protocol, common.DefaultProtocol)
		cfg.Port = withDefaultInt32(listenerSpec.Port, common.DefaultPort)
		cfg.BackendPort = withDefaultInt32(listenerSpec.BackendPort, common.DefaultBackendPort)
	}

	return cfg, nil
}

func withDefault(val, def string) string {
	if val == "" {
		return def
	}
	return val
}

func withDefaultInt32(val, def int32) int32 {
	if val == 0 {
		return def
	}
	return val
}
