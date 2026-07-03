package provider

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/openeverest/openeverest/v2/provider-runtime/controller"
	"github.com/openeverest/provider-huawei-elb/internal/common"
)

// EnsureService creates or updates the K8s LoadBalancer Service
// that binds to the pre-created Huawei Cloud ELB via the elb.id annotation.
func EnsureService(c *controller.Context, cfg *ELBConfig, elbID string) error {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfg.ELBName,
			Namespace: cfg.Namespace,
			Annotations: map[string]string{
				common.AnnotationELBID: elbID,
			},
			Labels: map[string]string{
				common.LabelInstance: cfg.InstanceName,
				common.LabelProvider: common.ProviderName,
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
			Ports: []corev1.ServicePort{
				{
					Name:       "listener",
				Protocol:   toK8sProtocol(cfg.Protocol),
					Port:       cfg.Port,
					TargetPort: intstr.FromInt(int(cfg.BackendPort)),
				},
			},
			Selector: map[string]string{
				common.LabelInstance: cfg.InstanceName,
			},
		},
	}

	if err := c.Apply(svc); err != nil {
		return fmt.Errorf("applying Service %s: %w", cfg.ELBName, err)
	}
	return nil
}

// GetService retrieves the K8s Service for the given instance.
func GetService(c *controller.Context, cfg *ELBConfig) (*corev1.Service, error) {
	svc := &corev1.Service{}
	if err := c.Get(svc, cfg.ELBName); err != nil {
		return nil, err
	}
	return svc, nil
}

// GetELBIDFromService extracts the ELB ID from the Service annotation.
// Returns empty string if the annotation is not present.
func GetELBIDFromService(svc *corev1.Service) string {
	if svc.Annotations == nil {
		return ""
	}
	return svc.Annotations[common.AnnotationELBID]
}

// DeleteService deletes the K8s Service. Ignores not-found errors.
func DeleteService(c *controller.Context, cfg *ELBConfig) error {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfg.ELBName,
			Namespace: cfg.Namespace,
		},
	}
	return c.Delete(svc)
}

// toK8sProtocol maps an ELB listener protocol string to a Kubernetes Service protocol.
// TCP and UDP map directly; HTTP/HTTPS fall back to TCP (L7 routing is handled at the ELB layer).
func toK8sProtocol(protocol string) corev1.Protocol {
	switch strings.ToUpper(protocol) {
	case "UDP":
		return corev1.ProtocolUDP
	case "SCTP":
		return corev1.ProtocolSCTP
	default:
		return corev1.ProtocolTCP
	}
}
