package hostport

import (
	"fmt"

	"golang.stackrox.io/kube-linter/pkg/check"
	"golang.stackrox.io/kube-linter/pkg/config"
	"golang.stackrox.io/kube-linter/pkg/diagnostic"
	"golang.stackrox.io/kube-linter/pkg/objectkinds"
	"golang.stackrox.io/kube-linter/pkg/templates"
	"golang.stackrox.io/kube-linter/pkg/templates/hostport/internal/params"
	"golang.stackrox.io/kube-linter/pkg/templates/util"
	v1 "k8s.io/api/core/v1"
)

const (
	templateKey = "host-port"
)

func init() {
	templates.Register(check.Template{
		HumanName:   "Host Port",
		Key:         templateKey,
		Description: "Flag containers exposing ports on the host (via hostPort)",
		SupportedObjectKinds: config.ObjectKindsDesc{
			ObjectKinds: []string{objectkinds.DeploymentLike},
		},
		Parameters:             params.ParamDescs,
		ParseAndValidateParams: params.ParseAndValidate,
		Instantiate: params.WrapInstantiateFunc(func(_ params.Params) (check.Func, error) {
			return util.PerContainerCheck(func(container *v1.Container) []diagnostic.Diagnostic {
				var results []diagnostic.Diagnostic
				for _, port := range container.Ports {
					// A hostPort of 0 is the Kubernetes API default when the field is
					// omitted, and means "no port is exposed on the host". Only an
					// explicitly set, non-zero hostPort maps a port on the host.
					if port.HostPort == 0 {
						continue
					}
					// The k8s protocol defaults to TCP even if not set in the YAML.
					protocol := string(port.Protocol)
					if protocol == "" {
						protocol = "TCP"
					}
					results = append(results, diagnostic.Diagnostic{
						Message: fmt.Sprintf("host port %d (protocol %s) is mapped in container %q.",
							port.HostPort, protocol, container.Name),
					})
				}
				return results
			}), nil
		}),
	})
}
