package webapi

import (
	"context"
	"encoding/gob"
	"fmt"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"path/filepath"
	"time"

	"k8s.io/utils/clock"

	"github.com/flyteorg/flyte/flyteplugins/go/tasks/errors"
	"github.com/flyteorg/flyte/flyteplugins/go/tasks/pluginmachinery/core"
	"github.com/flyteorg/flyte/flyteplugins/go/tasks/pluginmachinery/webapi"
	"github.com/flyteorg/flyte/flytestdlib/cache"
	stdErrs "github.com/flyteorg/flyte/flytestdlib/errors"
	"github.com/flyteorg/flyte/flytestdlib/logger"
)

const (
	pluginStateVersion = 1
	minCacheSize       = 10
	maxCacheSize       = 500000
	minWorkers         = 1
	maxWorkers         = 100
	minSyncDuration    = 5 * time.Second
	maxSyncDuration    = time.Hour
	minBurst           = 5
	maxBurst           = 10000
	minQPS             = 1
	maxQPS             = 100000
)

type CorePlugin struct {
	id             string
	p              webapi.AsyncPlugin
	cache          cache.AutoRefresh
	tokenAllocator tokenAllocator
	metrics        Metrics
}

func (c CorePlugin) unmarshalState(ctx context.Context, stateReader core.PluginStateReader) (State, error) {
	t := c.metrics.SucceededUnmarshalState.Start(ctx)
	existingState := State{}

	// We assume here that the first time this function is called, the custom state we get back is whatever we passed in,
	// namely the zero-value of our struct.
	if _, err := stateReader.Get(&existingState); err != nil {
		c.metrics.FailedUnmarshalState.Inc(ctx)
		logger.Errorf(ctx, "AsyncPlugin [%v] failed to unmarshal custom state. Error: %v",
			c.GetID(), err)

		return State{}, errors.Wrapf(errors.CorruptedPluginState, err,
			"Failed to unmarshal custom state in Handle")
	}

	t.Stop()
	return existingState, nil
}

func (c CorePlugin) GetID() string {
	return c.id
}

func (c CorePlugin) GetProperties() core.PluginProperties {
	return core.PluginProperties{}
}

func waitForDeployment(clientset *kubernetes.Clientset, namespace, deploymentName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		deployment, err := clientset.AppsV1().Deployments(namespace).Get(context.TODO(), deploymentName, metav1.GetOptions{})
		if err != nil {
			return err
		}

		// 检查 Deployment 的条件，这里简单判断 replicas 是否达到期望值
		if deployment.Status.Replicas == deployment.Status.ReadyReplicas {
			fmt.Println("Deployment is ready!")
			return nil
		}

		// 如果还没达到期望状态，等待一段时间后重试
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for Deployment to be ready")
		}

		// 等待一段时间后继续轮询
		time.Sleep(5 * time.Second)
	}
}

func (c CorePlugin) Handle(ctx context.Context, tCtx core.TaskExecutionContext) (core.Transition, error) {
	incomingState, err := c.unmarshalState(ctx, tCtx.PluginStateReader())
	if err != nil {
		return core.UnknownTransition, err
	}

	var nextState *State
	var phaseInfo core.PhaseInfo

	switch incomingState.Phase {
	case PhaseNotStarted:

		// kube config
		kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
		config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			fmt.Printf("Error building kubeconfig: %v\n", err)
			return core.UnknownTransition, err
		}
		// client
		clientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			fmt.Printf("Error creating Kubernetes client: %v\n", err)
			return core.UnknownTransition, err
		}

		//serviceAccountName := "flyteagent"
		namespace := "flyte"

		// 创建 ServiceAccount
		//_, err = clientset.CoreV1().ServiceAccounts(namespace).Create(context.TODO(), &corev1.ServiceAccount{
		//	ObjectMeta: metav1.ObjectMeta{
		//		Name: serviceAccountName,
		//	},
		//}, metav1.CreateOptions{})
		//if err != nil {
		//	fmt.Printf("Error creating serviceAccount: %v\n", err)
		//	return core.UnknownTransition, err
		//}

		name := "flyteagent"
		replicas := int32(1)
		twentyFivePercent := intstr.FromString("25%")
		revisionHistoryLimit := int32(10)
		terminationGracePeriodSeconds := int64(30)

		// 建立 deployment
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels: map[string]string{
					"app.kubernetes.io/instance":   "flyteagent",
					"app.kubernetes.io/managed-by": "Helm",
					"app.kubernetes.io/name":       "flyteagent",
					"helm.sh/chart":                "flyteagent-v0.1.10",
				},
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app.kubernetes.io/instance": "flyteagent",
						"app.kubernetes.io/name":     "flyteagent",
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"app.kubernetes.io/instance":   "flyteagent",
							"app.kubernetes.io/managed-by": "Helm",
							"app.kubernetes.io/name":       "flyteagent",
							"helm.sh/chart":                "flyteagent-v0.1.10",
						},
					},
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								Name: name,
								// SECRET 不知道怎麼寫
							},
						},
						Containers: []corev1.Container{
							{
								Name:    name,
								Image:   "ghcr.io/flyteorg/flyteagent:1.10.1",
								Command: []string{"pyflyte", "serve"},
								Ports: []corev1.ContainerPort{
									{
										Name:          "agent-grpc",
										ContainerPort: 8000,
										Protocol:      "TCP",
									},
								},
								Env: []corev1.EnvVar{
									{
										Name:  "FLYTE_AWS_ENDPOINT",
										Value: "http://flyte-sandbox-minio.flyte:9000",
									},
									{
										Name:  "FLYTE_AWS_ACCESS_KEY_ID",
										Value: "minio",
									},
									{
										Name:  "FLYTE_AWS_SECRET_ACCESS_KEY",
										Value: "miniostorage",
									},
								},
								Resources: corev1.ResourceRequirements{
									Limits: corev1.ResourceList{
										corev1.ResourceCPU:              resource.MustParse("500m"),
										corev1.ResourceEphemeralStorage: resource.MustParse("200Mi"),
										corev1.ResourceMemory:           resource.MustParse("200Mi"),
									},
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:              resource.MustParse("500m"),
										corev1.ResourceEphemeralStorage: resource.MustParse("200Mi"),
										corev1.ResourceMemory:           resource.MustParse("200Mi"),
									},
								},
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      name,
										MountPath: "/etc/secrets",
									},
								},
								TerminationMessagePath:   "/dev/termination-log",
								TerminationMessagePolicy: "File",
								ImagePullPolicy:          "IfNotPresent",
							},
						},
						RestartPolicy:                 "Always",
						TerminationGracePeriodSeconds: &terminationGracePeriodSeconds,
						DNSPolicy:                     "ClusterFirst",
						// 待確認 ServiceAccount:                name,
						ServiceAccountName: name,
						SecurityContext:    nil,
						SchedulerName:      "default-scheduler",
					},
				},
				Strategy: appsv1.DeploymentStrategy{
					Type: "RollingUpdate",
					RollingUpdate: &appsv1.RollingUpdateDeployment{
						MaxUnavailable: &twentyFivePercent,
						MaxSurge:       &twentyFivePercent,
					},
				},
				RevisionHistoryLimit: &revisionHistoryLimit,
			},
		}

		createdDeployment, err := clientset.AppsV1().Deployments(namespace).Create(context.Background(), deployment, metav1.CreateOptions{})
		if err != nil {
			fmt.Printf("Error creating Deployment: %v\n", err)
			return core.UnknownTransition, err
		}
		fmt.Printf("Deployment %s created successfully.\n", createdDeployment.GetName())

		// 等待 Deployment 完成
		err = waitForDeployment(clientset, namespace, name, 10*time.Minute)
		if err != nil {
			fmt.Printf("Error waiting for Deployment: %v\n", err)
			return core.UnknownTransition, err
		}

		if len(c.p.GetConfig().ResourceQuotas) > 0 {
			nextState, phaseInfo, err = c.tokenAllocator.allocateToken(ctx, c.p, tCtx, &incomingState, c.metrics)
		} else {
			nextState, phaseInfo, err = launch(ctx, c.p, tCtx, c.cache, &incomingState)
		}
	case PhaseAllocationTokenAcquired:
		nextState, phaseInfo, err = launch(ctx, c.p, tCtx, c.cache, &incomingState)
	case PhaseResourcesCreated:
		nextState, phaseInfo, err = monitor(ctx, tCtx, c.p, c.cache, &incomingState)
	}

	if err != nil {
		return core.UnknownTransition, err
	}

	if err := tCtx.PluginStateWriter().Put(pluginStateVersion, nextState); err != nil {
		return core.UnknownTransition, err
	}

	return core.DoTransition(phaseInfo), nil
}

func (c CorePlugin) Abort(ctx context.Context, tCtx core.TaskExecutionContext) error {
	incomingState, err := c.unmarshalState(ctx, tCtx.PluginStateReader())
	if err != nil {
		return err
	}

	logger.Infof(ctx, "Attempting to abort resource [%v].", tCtx.TaskExecutionMetadata().GetTaskExecutionID().GetID())

	err = c.p.Delete(ctx, newPluginContext(incomingState.ResourceMeta, nil, "Aborted", tCtx))
	if err != nil {
		logger.Errorf(ctx, "Failed to abort some resources [%v]. Error: %v",
			tCtx.TaskExecutionMetadata().GetTaskExecutionID().GetGeneratedName(), err)
		return err
	}

	return nil
}

func (c CorePlugin) Finalize(ctx context.Context, tCtx core.TaskExecutionContext) error {
	if len(c.p.GetConfig().ResourceQuotas) == 0 {
		// If there are no defined quotas, there is nothing to cleanup.
		return nil
	}

	logger.Infof(ctx, "Attempting to finalize resource [%v].",
		tCtx.TaskExecutionMetadata().GetTaskExecutionID().GetGeneratedName())

	return c.tokenAllocator.releaseToken(ctx, c.p, tCtx, c.metrics)
}

func validateRangeInt(fieldName string, min, max, provided int) error {
	if provided > max || provided < min {
		return fmt.Errorf("%v is expected to be between %v and %v. Provided value is %v",
			fieldName, min, max, provided)
	}

	return nil
}

func validateRangeFloat64(fieldName string, min, max, provided float64) error {
	if provided > max || provided < min {
		return fmt.Errorf("%v is expected to be between %v and %v. Provided value is %v",
			fieldName, min, max, provided)
	}

	return nil
}

func validateConfig(cfg webapi.PluginConfig) error {
	errs := stdErrs.ErrorCollection{}
	errs.Append(validateRangeInt("cache size", minCacheSize, maxCacheSize, cfg.Caching.Size))
	errs.Append(validateRangeInt("workers count", minWorkers, maxWorkers, cfg.Caching.Workers))
	errs.Append(validateRangeFloat64("resync interval", minSyncDuration.Seconds(), maxSyncDuration.Seconds(), cfg.Caching.ResyncInterval.Seconds()))
	errs.Append(validateRangeInt("read burst", minBurst, maxBurst, cfg.ReadRateLimiter.Burst))
	errs.Append(validateRangeInt("read qps", minQPS, maxQPS, cfg.ReadRateLimiter.QPS))
	errs.Append(validateRangeInt("write burst", minBurst, maxBurst, cfg.WriteRateLimiter.Burst))
	errs.Append(validateRangeInt("write qps", minQPS, maxQPS, cfg.WriteRateLimiter.QPS))

	return errs.ErrorOrDefault()
}

func createRemotePlugin(pluginEntry webapi.PluginEntry, c clock.Clock) core.PluginEntry {
	return core.PluginEntry{
		ID:                  pluginEntry.ID,
		RegisteredTaskTypes: pluginEntry.SupportedTaskTypes,
		LoadPlugin: func(ctx context.Context, iCtx core.SetupContext) (
			core.Plugin, error) {
			p, err := pluginEntry.PluginLoader(ctx, iCtx)
			if err != nil {
				return nil, err
			}

			err = validateConfig(p.GetConfig())
			if err != nil {
				return nil, fmt.Errorf("config validation failed. Error: %w", err)
			}

			// If the plugin will use a custom state, register it to be able to
			// serialize/deserialize interfaces later.
			if customState := p.GetConfig().ResourceMeta; customState != nil {
				gob.Register(customState)
			}

			if quotas := p.GetConfig().ResourceQuotas; len(quotas) > 0 {
				for ns, quota := range quotas {
					err := iCtx.ResourceRegistrar().RegisterResourceQuota(ctx, ns, quota)
					if err != nil {
						return nil, err
					}
				}
			}

			resourceCache, err := NewResourceCache(ctx, pluginEntry.ID, p, p.GetConfig().Caching,
				iCtx.MetricsScope().NewSubScope("cache"))

			if err != nil {
				return nil, err
			}

			err = resourceCache.Start(ctx)
			if err != nil {
				return nil, err
			}

			return CorePlugin{
				id:             pluginEntry.ID,
				p:              p,
				cache:          resourceCache,
				metrics:        newMetrics(iCtx.MetricsScope()),
				tokenAllocator: newTokenAllocator(c),
			}, nil
		},
	}
}

func CreateRemotePlugin(pluginEntry webapi.PluginEntry) core.PluginEntry {
	return createRemotePlugin(pluginEntry, clock.RealClock{})
}
