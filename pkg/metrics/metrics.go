/*
Copyright 2022 The Kubernetes Authors.

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

package metrics

import (
	"sync"

	"k8s.io/component-base/metrics"
	"k8s.io/component-base/metrics/legacyregistry"
)

const (
	// QueueSubsystem - subsystem name used by queue
	QueueSubsystem = "queue"
)

var (
	pendingWorkloads = metrics.NewGaugeVec(
		&metrics.GaugeOpts{
			Subsystem:      QueueSubsystem,
			Name:           "pending_workloads",
			Help:           "Number of pending workloads, by the owner type. 'queue' means number of workloads in queue; 'clusterQueue' means number of workloads in clusterQueue.",
			StabilityLevel: metrics.ALPHA,
		}, []string{"type", "name", "ns"})

	metricsList = []metrics.Registerable{
		pendingWorkloads,
	}
)

var registerMetrics sync.Once

// Register all metrics.
func Register() {
	// Register the metrics.
	registerMetrics.Do(func() {
		for _, metric := range metricsList {
			legacyregistry.MustRegister(metric)
		}
	})
}

// GetGather returns the gatherer. It used by test case outside current package.
func GetGather() metrics.Gatherer {
	return legacyregistry.DefaultGatherer
}

// PendingWorkloadsInQueue returns the pending workloads metrics with the label queue
func PendingWorkloadsInQueue(name, ns string) metrics.GaugeMetric {
	return pendingWorkloads.With(metrics.Labels{"type": "queue", "name": name, "ns": ns})
}

// PendingWorkloadsInClusterQueue returns the pending workloads metrics with the label clusterQueue
func PendingWorkloadsInClusterQueue(name string) metrics.GaugeMetric {
	return pendingWorkloads.With(metrics.Labels{"type": "clusterQueue", "name": name, "ns": ""})
}
