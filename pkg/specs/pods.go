/*
Copyright The CloudNativePG Contributors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package specs contains the specification of the K8s resources
// generated by the CloudNativePG operator
package specs

import (
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/management/url"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/postgres"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"
)

const (
	// MetadataNamespace is the annotation and label namespace used by the operator
	MetadataNamespace = "cnpg.io"

	// ClusterSerialAnnotationName is the name of the annotation containing the
	// serial number of the node
	ClusterSerialAnnotationName = MetadataNamespace + "/nodeSerial"

	// ClusterRestartAnnotationName is the name of the annotation containing the
	// latest required restart time
	ClusterRestartAnnotationName = "kubectl.kubernetes.io/restartedAt"

	// ClusterReloadAnnotationName is the name of the annotation containing the
	// latest required restart time
	ClusterReloadAnnotationName = MetadataNamespace + "/reloadedAt"

	// ClusterRoleLabelName label is applied to Pods to mark primary ones
	ClusterRoleLabelName = "role"

	// ClusterRoleLabelPrimary is written in labels to represent primary servers
	ClusterRoleLabelPrimary = "primary"

	// ClusterRoleLabelReplica is written in labels to represent replica servers
	ClusterRoleLabelReplica = "replica"

	// WatchedLabelName label is for Secrets or ConfigMaps that needs to be reloaded
	WatchedLabelName = MetadataNamespace + "/reload"

	// PostgresContainerName is the name of the container executing PostgreSQL
	// inside one Pod
	PostgresContainerName = "postgres"

	// BootstrapControllerContainerName is the name of the container copying the bootstrap
	// controller inside the Pod file system
	BootstrapControllerContainerName = "bootstrap-controller"

	// PgDataPath is the path to PGDATA variable
	PgDataPath = "/var/lib/postgresql/data/pgdata"

	// PgWalPath is the path to the pg_wal directory
	PgWalPath = PgDataPath + "/pg_wal"

	// PgWalArchiveStatusPath is the path to the archive status directory
	PgWalArchiveStatusPath = PgWalPath + "/archive_status"

	// ReadinessProbePeriod is the period set for the postgres instance readiness probe
	ReadinessProbePeriod = 10
)

func createEnvVarPostgresContainer(cluster apiv1.Cluster, podName string) []corev1.EnvVar {
	envVar := []corev1.EnvVar{
		{
			Name:  "PGDATA",
			Value: PgDataPath,
		},
		{
			Name:  "POD_NAME",
			Value: podName,
		},
		{
			Name:  "NAMESPACE",
			Value: cluster.Namespace,
		},
		{
			Name:  "CLUSTER_NAME",
			Value: cluster.Name,
		},
		{
			Name:  "PGPORT",
			Value: strconv.Itoa(postgres.ServerPort),
		},
		{
			Name:  "PGHOST",
			Value: postgres.SocketDirectory,
		},
	}

	return envVar
}

// createPostgresContainers create the PostgreSQL containers that are
// used for every instance
func createPostgresContainers(
	cluster apiv1.Cluster,
	podName string,
) []corev1.Container {
	containers := []corev1.Container{
		{
			Name:            PostgresContainerName,
			Image:           cluster.GetImageName(),
			ImagePullPolicy: cluster.Spec.ImagePullPolicy,
			Env:             createEnvVarPostgresContainer(cluster, podName),
			VolumeMounts:    createPostgresVolumeMounts(cluster),
			ReadinessProbe: &corev1.Probe{
				TimeoutSeconds: 5,
				PeriodSeconds:  ReadinessProbePeriod,
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: url.PathReady,
						Port: intstr.FromInt(url.StatusPort),
					},
				},
			},
			// From K8s 1.17 and newer, startup probes will be available for
			// all users and not just protected from feature gates. For now
			// let's use the LivenessProbe. When we will drop support for K8s
			// 1.16, we'll configure a StartupProbe and this will lead to a
			// better LivenessProbe (without InitialDelaySeconds).
			LivenessProbe: &corev1.Probe{
				InitialDelaySeconds: cluster.GetMaxStartDelay(),
				TimeoutSeconds:      5,
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: url.PathHealth,
						Port: intstr.FromInt(url.StatusPort),
					},
				},
			},
			Command: []string{
				"/controller/manager",
				"instance",
				"run",
			},
			Resources: cluster.Spec.Resources,
			Ports: []corev1.ContainerPort{
				{
					Name:          "postgresql",
					ContainerPort: postgres.ServerPort,
					Protocol:      "TCP",
				},
				{
					Name:          "metrics",
					ContainerPort: int32(url.PostgresMetricsPort),
					Protocol:      "TCP",
				},
				{
					Name:          "status",
					ContainerPort: int32(url.StatusPort),
					Protocol:      "TCP",
				},
			},
			SecurityContext: CreateContainerSecurityContext(),
		},
	}

	addManagerLoggingOptions(cluster, &containers[0])

	return containers
}

// CreateAffinitySection creates the affinity sections for Pods, given the configuration
// from the user
func CreateAffinitySection(clusterName string, config apiv1.AffinityConfiguration) *corev1.Affinity {
	// Initialize affinity
	affinity := CreateGeneratedAntiAffinity(clusterName, config)

	if config.AdditionalPodAffinity == nil &&
		config.AdditionalPodAntiAffinity == nil {
		return affinity
	}

	if affinity == nil {
		affinity = &corev1.Affinity{}
	}

	if config.AdditionalPodAffinity != nil {
		affinity.PodAffinity = config.AdditionalPodAffinity
	}

	if config.AdditionalPodAntiAffinity != nil {
		if affinity.PodAntiAffinity == nil {
			affinity.PodAntiAffinity = &corev1.PodAntiAffinity{}
		}
		affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution = append(
			affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution,
			config.AdditionalPodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution...)
		affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution = append(
			affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution,
			config.AdditionalPodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution...)
	}
	return affinity
}

// CreateGeneratedAntiAffinity generates the affinity terms the operator is in charge for if enabled,
// return nil if disabled or an error occurred, as invalid values should be validated before this method is called
func CreateGeneratedAntiAffinity(clusterName string, config apiv1.AffinityConfiguration) *corev1.Affinity {
	// We have no anti affinity section if the user don't have it configured
	if config.EnablePodAntiAffinity != nil && !(*config.EnablePodAntiAffinity) {
		return nil
	}
	affinity := &corev1.Affinity{PodAntiAffinity: &corev1.PodAntiAffinity{}}
	topologyKey := config.TopologyKey
	if len(topologyKey) == 0 {
		topologyKey = "kubernetes.io/hostname"
	}

	podAffinityTerm := corev1.PodAffinityTerm{
		LabelSelector: &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      utils.ClusterLabelName,
					Operator: metav1.LabelSelectorOpIn,
					Values: []string{
						clusterName,
					},
				},
			},
		},
		TopologyKey: topologyKey,
	}

	// Switch pod anti-affinity type:
	// - if it is "required", 'RequiredDuringSchedulingIgnoredDuringExecution' will be properly set.
	// - if it is "preferred",'PreferredDuringSchedulingIgnoredDuringExecution' will be properly set.
	// - by default, return nil.
	switch config.PodAntiAffinityType {
	case apiv1.PodAntiAffinityTypeRequired:
		affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution = []corev1.PodAffinityTerm{
			podAffinityTerm,
		}
	case apiv1.PodAntiAffinityTypePreferred:
		affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution = []corev1.WeightedPodAffinityTerm{
			{
				Weight:          100,
				PodAffinityTerm: podAffinityTerm,
			},
		}
	default:
		return nil
	}
	return affinity
}

// CreatePodSecurityContext defines the security context under which the containers are running
func CreatePodSecurityContext(user, group int64) *corev1.PodSecurityContext {
	// Under Openshift we inherit SecurityContext from the restricted security context constraint
	if utils.HaveSecurityContextConstraints() {
		return nil
	}

	seccompProfile := &corev1.SeccompProfile{
		Type: corev1.SeccompProfileTypeRuntimeDefault,
	}
	if !utils.HaveSeccompSupport() {
		seccompProfile = nil
	}

	trueValue := true
	return &corev1.PodSecurityContext{
		RunAsNonRoot:   &trueValue,
		RunAsUser:      &user,
		RunAsGroup:     &group,
		FSGroup:        &group,
		SeccompProfile: seccompProfile,
	}
}

// PodWithExistingStorage create a new instance with an existing storage
func PodWithExistingStorage(cluster apiv1.Cluster, nodeSerial int) *corev1.Pod {
	podName := GetInstanceName(cluster.Name, nodeSerial)
	gracePeriod := int64(cluster.GetMaxStopDelay())

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				utils.OldClusterLabelName:   cluster.Name,
				utils.ClusterLabelName:      cluster.Name,
				utils.InstanceNameLabelName: podName,
				utils.PodRoleLabelName:      string(utils.PodRoleInstance),
			},
			Annotations: map[string]string{
				ClusterSerialAnnotationName: strconv.Itoa(nodeSerial),
			},
			Name:      podName,
			Namespace: cluster.Namespace,
		},
		Spec: corev1.PodSpec{
			Hostname:  podName,
			Subdomain: cluster.GetServiceAnyName(),
			InitContainers: []corev1.Container{
				createBootstrapContainer(cluster),
			},
			Containers:                    createPostgresContainers(cluster, podName),
			Volumes:                       createPostgresVolumes(cluster, podName),
			SecurityContext:               CreatePodSecurityContext(cluster.GetPostgresUID(), cluster.GetPostgresGID()),
			Affinity:                      CreateAffinitySection(cluster.Name, cluster.Spec.Affinity),
			Tolerations:                   cluster.Spec.Affinity.Tolerations,
			ServiceAccountName:            cluster.Name,
			NodeSelector:                  cluster.Spec.Affinity.NodeSelector,
			TerminationGracePeriodSeconds: &gracePeriod,
		},
	}

	if utils.IsAnnotationAppArmorPresent(cluster.Annotations) {
		utils.AnnotateAppArmor(&pod.ObjectMeta, cluster.Annotations)
	}
	return pod
}

// GetInstanceName returns a string indicating the instance name
func GetInstanceName(clusterName string, nodeSerial int) string {
	return fmt.Sprintf("%s-%v", clusterName, nodeSerial)
}

// AddBarmanEndpointCAToPodSpec adds the required volumes and env variables needed by barman to work correctly
func AddBarmanEndpointCAToPodSpec(
	podSpec *corev1.PodSpec,
	caSecret *apiv1.SecretKeySelector,
	credentials apiv1.BarmanCredentials,
) {
	if caSecret == nil || caSecret.Name == "" || caSecret.Key == "" {
		return
	}

	podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
		Name: "barman-endpoint-ca",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: caSecret.Name,
				Items: []corev1.KeyToPath{
					{
						Key:  caSecret.Key,
						Path: postgres.BarmanRestoreEndpointCACertificateFileName,
					},
				},
			},
		},
	})

	podSpec.Containers[0].VolumeMounts = append(podSpec.Containers[0].VolumeMounts,
		corev1.VolumeMount{
			Name:      "barman-endpoint-ca",
			MountPath: postgres.CertificatesDir,
		},
	)

	var envVars []corev1.EnvVar
	// todo: add a case for the Google provider
	switch {
	case credentials.Azure != nil:
		envVars = append(envVars, corev1.EnvVar{
			Name:  "REQUESTS_CA_BUNDLE",
			Value: postgres.BarmanRestoreEndpointCACertificateLocation,
		})
	// If nothing is set we fall back to AWS, this is to avoid breaking changes with previous versions
	default:
		envVars = append(envVars, corev1.EnvVar{
			Name:  "AWS_CA_BUNDLE",
			Value: postgres.BarmanRestoreEndpointCACertificateLocation,
		})
	}

	podSpec.Containers[0].Env = append(podSpec.Containers[0].Env, envVars...)
}
