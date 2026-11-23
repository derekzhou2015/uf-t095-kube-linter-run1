package hostport

import (
	"strings"
	"testing"

	ocsAppsV1 "github.com/openshift/api/apps/v1"
	"github.com/stretchr/testify/suite"
	"golang.stackrox.io/kube-linter/pkg/diagnostic"
	"golang.stackrox.io/kube-linter/pkg/k8sutil"
	"golang.stackrox.io/kube-linter/pkg/lintcontext/mocks"
	"golang.stackrox.io/kube-linter/pkg/templates"
	"golang.stackrox.io/kube-linter/pkg/templates/hostport/internal/params"
	appsV1 "k8s.io/api/apps/v1"
	batchV1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHostPort(t *testing.T) {
	suite.Run(t, new(HostPortTestSuite))
}

type HostPortTestSuite struct {
	templates.TemplateTestSuite

	ctx *mocks.MockLintContext
}

func (s *HostPortTestSuite) SetupTest() {
	s.Init(templateKey)
	s.ctx = mocks.NewMockContext()
}

func (s *HostPortTestSuite) addDeploymentWithContainers(name string, containers ...v1.Container) {
	s.ctx.AddMockDeployment(s.T(), name)
	s.ctx.ModifyDeployment(s.T(), name, func(deployment *appsV1.Deployment) {
		deployment.Spec.Template.Spec.Containers = containers
	})
}

func (s *HostPortTestSuite) TestContainerPortSemantics() {
	const targetName = "deployment01"
	testCases := []struct {
		name      string
		container v1.Container
		expected  map[string][]diagnostic.Diagnostic
	}{
		{
			name:      "NoPorts",
			container: v1.Container{Name: "container"},
			expected:  nil,
		},
		{
			name: "ContainerPortOnlyNoHostPort",
			container: v1.Container{
				Name: "container",
				Ports: []v1.ContainerPort{
					{ContainerPort: 8080},
				},
			},
			expected: nil,
		},
		{
			name: "HostPortExplicitlyZero",
			container: v1.Container{
				Name: "container",
				Ports: []v1.ContainerPort{
					{ContainerPort: 8080, HostPort: 0},
				},
			},
			expected: nil,
		},
		{
			name: "HostPortSet",
			container: v1.Container{
				Name: "container",
				Ports: []v1.ContainerPort{
					{ContainerPort: 8080, HostPort: 8080},
				},
			},
			expected: map[string][]diagnostic.Diagnostic{
				targetName: {
					{Message: "host port 8080 (protocol TCP) is mapped in container \"container\"."},
				},
			},
		},
		{
			name: "HostPortSetExplicitTCP",
			container: v1.Container{
				Name: "container",
				Ports: []v1.ContainerPort{
					{ContainerPort: 8080, HostPort: 80, Protocol: v1.ProtocolTCP},
				},
			},
			expected: map[string][]diagnostic.Diagnostic{
				targetName: {
					{Message: "host port 80 (protocol TCP) is mapped in container \"container\"."},
				},
			},
		},
		{
			name: "HostPortSetUDP",
			container: v1.Container{
				Name: "container",
				Ports: []v1.ContainerPort{
					{ContainerPort: 53, HostPort: 53, Protocol: v1.ProtocolUDP},
				},
			},
			expected: map[string][]diagnostic.Diagnostic{
				targetName: {
					{Message: "host port 53 (protocol UDP) is mapped in container \"container\"."},
				},
			},
		},
		{
			name: "HostPortSetSCTP",
			container: v1.Container{
				Name: "container",
				Ports: []v1.ContainerPort{
					{ContainerPort: 9999, HostPort: 9999, Protocol: v1.ProtocolSCTP},
				},
			},
			expected: map[string][]diagnostic.Diagnostic{
				targetName: {
					{Message: "host port 9999 (protocol SCTP) is mapped in container \"container\"."},
				},
			},
		},
		{
			name: "MultiplePortEntriesOnlyViolatingOnesFlagged",
			container: v1.Container{
				Name: "container",
				Ports: []v1.ContainerPort{
					{ContainerPort: 8080},
					{ContainerPort: 53, HostPort: 53, Protocol: v1.ProtocolUDP},
					{ContainerPort: 9090, HostPort: 0},
					{ContainerPort: 443, HostPort: 443},
				},
			},
			expected: map[string][]diagnostic.Diagnostic{
				targetName: {
					{Message: "host port 53 (protocol UDP) is mapped in container \"container\"."},
					{Message: "host port 443 (protocol TCP) is mapped in container \"container\"."},
				},
			},
		},
		{
			name: "SameHostPortDifferentProtocols",
			container: v1.Container{
				Name: "container",
				Ports: []v1.ContainerPort{
					{ContainerPort: 53, HostPort: 53, Protocol: v1.ProtocolTCP},
					{ContainerPort: 53, HostPort: 53, Protocol: v1.ProtocolUDP},
				},
			},
			expected: map[string][]diagnostic.Diagnostic{
				targetName: {
					{Message: "host port 53 (protocol TCP) is mapped in container \"container\"."},
					{Message: "host port 53 (protocol UDP) is mapped in container \"container\"."},
				},
			},
		},
	}
	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.addDeploymentWithContainers(targetName, tc.container)
			s.Validate(s.ctx, []templates.TestCase{{
				Param:       params.Params{},
				Diagnostics: tc.expected,
			}})
		})
	}
}

func (s *HostPortTestSuite) TestInitAndEphemeralContainers() {
	const targetName = "deployment01"
	hostPortContainer := v1.Container{
		Name: "container",
		Ports: []v1.ContainerPort{
			{ContainerPort: 8080, HostPort: 8080},
		},
	}
	expected := map[string][]diagnostic.Diagnostic{
		targetName: {
			{Message: "host port 8080 (protocol TCP) is mapped in container \"container\"."},
		},
	}

	s.Run("InitContainer", func() {
		s.ctx.AddMockDeployment(s.T(), targetName)
		s.ctx.ModifyDeployment(s.T(), targetName, func(deployment *appsV1.Deployment) {
			deployment.Spec.Template.Spec.InitContainers = []v1.Container{hostPortContainer}
		})
		s.Validate(s.ctx, []templates.TestCase{{
			Param:       params.Params{},
			Diagnostics: expected,
		}})
	})

	s.Run("EphemeralContainer", func() {
		s.ctx.AddMockDeployment(s.T(), targetName)
		s.ctx.ModifyDeployment(s.T(), targetName, func(deployment *appsV1.Deployment) {
			deployment.Spec.Template.Spec.EphemeralContainers = []v1.EphemeralContainer{
				{EphemeralContainerCommon: v1.EphemeralContainerCommon{
					Name: "container",
					Ports: []v1.ContainerPort{
						{ContainerPort: 8080, HostPort: 8080},
					},
				}},
			}
		})
		s.Validate(s.ctx, []templates.TestCase{{
			Param:       params.Params{},
			Diagnostics: expected,
		}})
	})
}

// podSpecContainers returns a pod spec with a single container declaring the
// given ports, used to build one object per supported workload kind below.
func podSpecWithPorts(ports ...v1.ContainerPort) v1.PodSpec {
	return v1.PodSpec{
		Containers: []v1.Container{
			{Name: "app", Ports: ports},
		},
	}
}

func podTemplateWithPorts(ports ...v1.ContainerPort) v1.PodTemplateSpec {
	return v1.PodTemplateSpec{Spec: podSpecWithPorts(ports...)}
}

// TestAllSupportedWorkloadKinds exercises every object kind reachable through
// the DeploymentLike object kind (the complete set of pod-spec-bearing kinds
// registered in the object kinds registry): each kind gets one object that
// must fire and one that must not.
func (s *HostPortTestSuite) TestAllSupportedWorkloadKinds() {
	violatingPorts := []v1.ContainerPort{{ContainerPort: 8080, HostPort: 8080}}
	harmlessPorts := []v1.ContainerPort{{ContainerPort: 8080}}

	objects := map[string]k8sutil.Object{
		"pod-fire": &v1.Pod{
			ObjectMeta: metaV1.ObjectMeta{Name: "pod-fire"},
			Spec:       podSpecWithPorts(violatingPorts...),
		},
		"pod-dont-fire": &v1.Pod{
			ObjectMeta: metaV1.ObjectMeta{Name: "pod-dont-fire"},
			Spec:       podSpecWithPorts(harmlessPorts...),
		},
		"deployment-fire": &appsV1.Deployment{
			ObjectMeta: metaV1.ObjectMeta{Name: "deployment-fire"},
			Spec:       appsV1.DeploymentSpec{Template: podTemplateWithPorts(violatingPorts...)},
		},
		"deployment-dont-fire": &appsV1.Deployment{
			ObjectMeta: metaV1.ObjectMeta{Name: "deployment-dont-fire"},
			Spec:       appsV1.DeploymentSpec{Template: podTemplateWithPorts(harmlessPorts...)},
		},
		"daemonset-fire": &appsV1.DaemonSet{
			ObjectMeta: metaV1.ObjectMeta{Name: "daemonset-fire"},
			Spec:       appsV1.DaemonSetSpec{Template: podTemplateWithPorts(violatingPorts...)},
		},
		"daemonset-dont-fire": &appsV1.DaemonSet{
			ObjectMeta: metaV1.ObjectMeta{Name: "daemonset-dont-fire"},
			Spec:       appsV1.DaemonSetSpec{Template: podTemplateWithPorts(harmlessPorts...)},
		},
		"statefulset-fire": &appsV1.StatefulSet{
			ObjectMeta: metaV1.ObjectMeta{Name: "statefulset-fire"},
			Spec:       appsV1.StatefulSetSpec{Template: podTemplateWithPorts(violatingPorts...)},
		},
		"statefulset-dont-fire": &appsV1.StatefulSet{
			ObjectMeta: metaV1.ObjectMeta{Name: "statefulset-dont-fire"},
			Spec:       appsV1.StatefulSetSpec{Template: podTemplateWithPorts(harmlessPorts...)},
		},
		"replicaset-fire": &appsV1.ReplicaSet{
			ObjectMeta: metaV1.ObjectMeta{Name: "replicaset-fire"},
			Spec:       appsV1.ReplicaSetSpec{Template: podTemplateWithPorts(violatingPorts...)},
		},
		"replicaset-dont-fire": &appsV1.ReplicaSet{
			ObjectMeta: metaV1.ObjectMeta{Name: "replicaset-dont-fire"},
			Spec:       appsV1.ReplicaSetSpec{Template: podTemplateWithPorts(harmlessPorts...)},
		},
		"replicationcontroller-fire": &v1.ReplicationController{
			ObjectMeta: metaV1.ObjectMeta{Name: "replicationcontroller-fire"},
			Spec: v1.ReplicationControllerSpec{
				Template: func() *v1.PodTemplateSpec { t := podTemplateWithPorts(violatingPorts...); return &t }(),
			},
		},
		"replicationcontroller-dont-fire": &v1.ReplicationController{
			ObjectMeta: metaV1.ObjectMeta{Name: "replicationcontroller-dont-fire"},
			Spec: v1.ReplicationControllerSpec{
				Template: func() *v1.PodTemplateSpec { t := podTemplateWithPorts(harmlessPorts...); return &t }(),
			},
		},
		"job-fire": &batchV1.Job{
			ObjectMeta: metaV1.ObjectMeta{Name: "job-fire"},
			Spec:       batchV1.JobSpec{Template: podTemplateWithPorts(violatingPorts...)},
		},
		"job-dont-fire": &batchV1.Job{
			ObjectMeta: metaV1.ObjectMeta{Name: "job-dont-fire"},
			Spec:       batchV1.JobSpec{Template: podTemplateWithPorts(harmlessPorts...)},
		},
		"cronjob-fire": &batchV1.CronJob{
			ObjectMeta: metaV1.ObjectMeta{Name: "cronjob-fire"},
			Spec: batchV1.CronJobSpec{
				JobTemplate: batchV1.JobTemplateSpec{Spec: batchV1.JobSpec{Template: podTemplateWithPorts(violatingPorts...)}},
			},
		},
		"cronjob-dont-fire": &batchV1.CronJob{
			ObjectMeta: metaV1.ObjectMeta{Name: "cronjob-dont-fire"},
			Spec: batchV1.CronJobSpec{
				JobTemplate: batchV1.JobTemplateSpec{Spec: batchV1.JobSpec{Template: podTemplateWithPorts(harmlessPorts...)}},
			},
		},
		"deploymentconfig-fire": &ocsAppsV1.DeploymentConfig{
			ObjectMeta: metaV1.ObjectMeta{Name: "deploymentconfig-fire"},
			Spec: ocsAppsV1.DeploymentConfigSpec{
				Template: func() *v1.PodTemplateSpec { t := podTemplateWithPorts(violatingPorts...); return &t }(),
			},
		},
		"deploymentconfig-dont-fire": &ocsAppsV1.DeploymentConfig{
			ObjectMeta: metaV1.ObjectMeta{Name: "deploymentconfig-dont-fire"},
			Spec: ocsAppsV1.DeploymentConfigSpec{
				Template: func() *v1.PodTemplateSpec { t := podTemplateWithPorts(harmlessPorts...); return &t }(),
			},
		},
	}

	expected := make(map[string][]diagnostic.Diagnostic)
	for name, obj := range objects {
		s.ctx.AddObject(name, obj)
		if strings.HasSuffix(name, "-fire") && !strings.Contains(name, "dont") {
			expected[name] = []diagnostic.Diagnostic{
				{Message: "host port 8080 (protocol TCP) is mapped in container \"app\"."},
			}
		}
	}

	s.Validate(s.ctx, []templates.TestCase{{
		Param:       params.Params{},
		Diagnostics: expected,
	}})
}
