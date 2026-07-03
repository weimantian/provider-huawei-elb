package provider

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/openeverest/openeverest/v2/provider-runtime/controller"

	"github.com/openeverest/provider-huawei-elb/internal/common"
	"github.com/openeverest/provider-huawei-elb/internal/huaweicloud"
)

// Compile-time check that Provider implements the required interface.
var _ controller.ProviderInterface = (*Provider)(nil)

// Provider implements controller.ProviderInterface for the provider-huawei-elb provider.
type Provider struct {
	controller.BaseProvider
}

// New creates a new Provider instance.
func New() *Provider {
	return &Provider{
		BaseProvider: controller.BaseProvider{
			ProviderName: common.ProviderName,
			SchemeFuncs:  []func(*runtime.Scheme) error{},
			WatchConfigs: []controller.WatchConfig{},
		},
	}
}

// Validate checks if the Instance spec is valid.
func (p *Provider) Validate(c *controller.Context) error {
	l := log.FromContext(c.Context())
	l.Info("Validating instance", "name", c.Name())

	cfg, err := ResolveConfig(c)
	if err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	if cfg.VpcID == "" {
		return fmt.Errorf("elbEngine.customSpec.vpcId is required")
	}
	if cfg.VipSubnetCidrID == "" {
		return fmt.Errorf("elbEngine.customSpec.vipSubnetCidrId is required")
	}
	if len(cfg.AvailabilityZoneList) == 0 {
		return fmt.Errorf("elbEngine.customSpec.availabilityZoneList is required")
	}
	if cfg.IsPublicELB && cfg.BandwidthSize < 1 {
		return fmt.Errorf("bandwidthSize must be >= 1 for public-elb topology")
	}

	return nil
}

// Sync ensures all required resources exist and are configured correctly.
// It creates the Huawei Cloud ELB (if not yet created) and the K8s
// LoadBalancer Service that binds to it.
func (p *Provider) Sync(c *controller.Context) error {
	l := log.FromContext(c.Context())
	l.Info("Syncing instance", "name", c.Name())

	cfg, err := ResolveConfig(c)
	if err != nil {
		return err
	}

	creds, err := huaweicloud.LoadCredentials()
	if err != nil {
		return fmt.Errorf("loading credentials: %w", err)
	}

	client, err := huaweicloud.NewELBClient(creds)
	if err != nil {
		return fmt.Errorf("creating ELB client: %w", err)
	}

	// Try to get ELB ID from existing Service.
	var elbID string
	if svc, err := GetService(c, cfg); err == nil {
		elbID = GetELBIDFromService(svc)
	}

	// If no ELB ID on Service, try to find existing ELB by name.
	if elbID == "" {
		existing, err := huaweicloud.FindELBByName(client, cfg.ELBName)
		if err != nil {
			return fmt.Errorf("finding existing ELB: %w", err)
		}
		if existing != nil {
			elbID = existing.ID
		}
	}

	// If still no ELB, create one.
	if elbID == "" {
		l.Info("Creating ELB", "name", cfg.ELBName)
		tags := map[string]string{
			common.LabelInstance: cfg.InstanceName,
			common.LabelProvider: common.ProviderName,
		}
		info, err := huaweicloud.CreateELB(client, huaweicloud.CreateELBOption{
			Name:                 cfg.ELBName,
			VpcID:                cfg.VpcID,
			VipSubnetCidrID:      cfg.VipSubnetCidrID,
			AvailabilityZoneList: cfg.AvailabilityZoneList,
			IsPublic:             cfg.IsPublicELB,
			BandwidthSize:        cfg.BandwidthSize,
			BandwidthChargeMode:  cfg.BandwidthChargeMode,
			PublicIPNetworkType:  cfg.PublicIPNetworkType,
			Tags:                 tags,
		})
		if err != nil {
			return fmt.Errorf("creating ELB: %w", err)
		}
		elbID = info.ID
	}

	// Ensure K8s Service with ELB ID annotation.
	if err := EnsureService(c, cfg, elbID); err != nil {
		return err
	}

	return nil
}

// Status computes the current status of the database instance.
// It checks the ELB provisioning status and returns connection details
// when the ELB is active.
func (p *Provider) Status(c *controller.Context) (controller.Status, error) {
	l := log.FromContext(c.Context())
	l.Info("Computing status", "name", c.Name())

	cfg, err := ResolveConfig(c)
	if err != nil {
		return controller.Failed(fmt.Sprintf("config error: %v", err)), nil
	}

	// Get ELB ID from Service.
	svc, err := GetService(c, cfg)
	if err != nil {
		return controller.Provisioning("waiting for Service creation"), nil
	}
	elbID := GetELBIDFromService(svc)
	if elbID == "" {
		return controller.Provisioning("waiting for ELB creation"), nil
	}

	// Check ELB status via Huawei Cloud API.
	creds, err := huaweicloud.LoadCredentials()
	if err != nil {
		return controller.Failed(fmt.Sprintf("credential error: %v", err)), nil
	}
	client, err := huaweicloud.NewELBClient(creds)
	if err != nil {
		return controller.Failed(fmt.Sprintf("client error: %v", err)), nil
	}

	info, err := huaweicloud.ShowELB(client, elbID)
	if err != nil {
		return controller.Status{}, fmt.Errorf("showing ELB: %w", err)
	}

	if info.ProvisioningStatus == "ERROR" {
		return controller.Failed(fmt.Sprintf("ELB provisioning failed: %s", info.ProvisioningStatus)), nil
	}
	if info.ProvisioningStatus != "ACTIVE" {
		return controller.Provisioning(fmt.Sprintf("ELB provisioning status: %s", info.ProvisioningStatus)), nil
	}

	// ELB is active — return connection details.
	host := info.VipAddress
	if cfg.IsPublicELB && info.PublicIP != "" {
		host = info.PublicIP
	}

	return controller.ReadyWithConnectionDetails(
		controller.ConnectionDetails{
			Type:     "elb",
			Provider: common.ProviderName,
			Host:     host,
			Port:     fmt.Sprintf("%d", cfg.Port),
		},
	), nil
}

// Cleanup handles deletion of provider-managed resources.
// It deletes the Huawei Cloud ELB and the K8s Service.
func (p *Provider) Cleanup(c *controller.Context) error {
	l := log.FromContext(c.Context())
	l.Info("Cleaning up instance", "name", c.Name())

	cfg, err := ResolveConfig(c)
	if err != nil {
		return err
	}

	// Get ELB ID from Service (if it exists).
	var elbID string
	if svc, err := GetService(c, cfg); err == nil {
		elbID = GetELBIDFromService(svc)
	}

	// Delete the ELB.
	if elbID != "" {
		creds, err := huaweicloud.LoadCredentials()
		if err != nil {
			return fmt.Errorf("loading credentials: %w", err)
		}
		client, err := huaweicloud.NewELBClient(creds)
		if err != nil {
			return fmt.Errorf("creating ELB client: %w", err)
		}

		l.Info("Deleting ELB", "id", elbID)
		if err := huaweicloud.DeleteELB(client, elbID); err != nil {
			return fmt.Errorf("deleting ELB: %w", err)
		}
	} else {
		// Try to find by name as fallback.
		creds, err := huaweicloud.LoadCredentials()
		if err != nil {
			return fmt.Errorf("loading credentials for cleanup: %w", err)
		}
		client, err := huaweicloud.NewELBClient(creds)
		if err != nil {
			return fmt.Errorf("creating ELB client for cleanup: %w", err)
		}
		existing, err := huaweicloud.FindELBByName(client, cfg.ELBName)
		if err != nil {
			return fmt.Errorf("finding ELB by name for cleanup: %w", err)
		}
		if existing != nil {
			l.Info("Deleting ELB found by name", "id", existing.ID)
			if err := huaweicloud.DeleteELB(client, existing.ID); err != nil {
				return fmt.Errorf("deleting ELB: %w", err)
			}
		}
	}

	// Delete the K8s Service (owner refs may handle this, but explicit is safer).
	if err := DeleteService(c, cfg); err != nil {
		l.Error(err, "deleting Service", "name", cfg.ELBName)
	}

	return nil
}
