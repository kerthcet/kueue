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

import "k8s.io/component-base/metrics"

// MetricRecorder represents a metric recorder which takes action when the
// metric Inc(), Dec() and Clear()
type MetricRecorder interface {
	Inc()
	Dec()
	Clear()
}

var _ MetricRecorder = &PendingWorkloadsRecorder{}

// PendingWorkloadsRecorder is an implementation of MetricRecorder
type PendingWorkloadsRecorder struct {
	recorder metrics.GaugeMetric
}

// Inc increases a metric counter by 1, in an atomic way
func (r *PendingWorkloadsRecorder) Inc() {
	r.recorder.Inc()
}

// Dec decreases a metric counter by 1, in an atomic way
func (r *PendingWorkloadsRecorder) Dec() {
	r.recorder.Dec()
}

// Clear set a metric counter to 0, in an atomic way
func (r *PendingWorkloadsRecorder) Clear() {
	r.recorder.Set(float64(0))
}

// NewPendingWorkloadsInQueueRecorder returns PendingWorkloadsInQueue in a Prometheus metric fashion
func NewPendingWorkloadsInQueueRecorder(name, ns string) *PendingWorkloadsRecorder {
	return &PendingWorkloadsRecorder{
		recorder: PendingWorkloadsInQueue(name, ns),
	}
}

// NewPendingWorkloadsInClusterQueueRecorder returns PendingWorkloadsInClusterQueue in a Prometheus metric fashion
func NewPendingWorkloadsInClusterQueueRecorder(name string) *PendingWorkloadsRecorder {
	return &PendingWorkloadsRecorder{
		recorder: PendingWorkloadsInClusterQueue(name),
	}
}
