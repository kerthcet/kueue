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

package queue

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/sets"
	kueue "sigs.k8s.io/kueue/apis/kueue/v1alpha1"
	"sigs.k8s.io/kueue/pkg/metrics"
	"sigs.k8s.io/kueue/pkg/workload"
)

// ClusterQueue is an interface for a cluster queue to store workloads waiting
// to be scheduled.
type ClusterQueue interface {
	// Name returns the ClusterQUeue name.
	Name() string
	// Update updates the properties of this ClusterQueue.
	Update(*kueue.ClusterQueue)
	// Cohort returns the Cohort of this ClusterQueue.
	Cohort() string

	// AddFromQueue pushes all workloads belonging to this queue to
	// the ClusterQueue. If at least one workload is added, returns true.
	// Otherwise returns false.
	AddFromQueue(*Queue) bool
	// DeleteFromQueue removes all workloads belonging to this queue from
	// the ClusterQueue.
	DeleteFromQueue(*Queue)

	// PushOrUpdate pushes the workload to ClusterQueue.
	// If the workload is already present, updates with the new one.
	PushOrUpdate(*kueue.Workload)
	// Delete removes the workload from ClusterQueue.
	Delete(*kueue.Workload)
	// Pop removes the head of the queue and returns it. It returns nil if the
	// queue is empty.
	Pop() *workload.Info

	// RequeueIfNotPresent inserts a workload that was not
	// admitted back into the ClusterQueue. If the boolean is true,
	// the workloads should be put back in the queue immediately,
	// because we couldn't determine if the workload was admissible
	// in the last cycle. If the boolean is false, the implementation might
	// choose to keep it in temporary placeholder stage where it doesn't
	// compete with other workloads, until cluster events free up quota.
	// The workload should not be reinserted if it's already in the ClusterQueue.
	// Returns true if the workload was inserted.
	RequeueIfNotPresent(*workload.Info, bool) bool
	// QueueInadmissibleWorkloads moves all workloads put in temporary placeholder stage
	// to the ClusterQueue. If at least one workload is moved,
	// returns true. Otherwise returns false.
	QueueInadmissibleWorkloads() bool

	// Pending returns the number of pending workloads.
	Pending() int32
	// Dump produces a dump of the current workloads in the heap of
	// this ClusterQueue. It returns false if the queue is empty.
	// Otherwise returns true.
	Dump() (sets.String, bool)
	// Info returns workload.Info for the workload key.
	// Users of this method should not modify the returned object.
	Info(string) *workload.Info
}

var registry = map[kueue.QueueingStrategy]func(*kueue.ClusterQueue, metrics.MetricRecorder) (ClusterQueue, error){
	StrictFIFO:     newClusterQueueStrictFIFO,
	BestEffortFIFO: newClusterQueueBestEffortFIFO,
}

func newClusterQueue(cq *kueue.ClusterQueue, metricRecorder metrics.MetricRecorder) (ClusterQueue, error) {
	strategy := cq.Spec.QueueingStrategy
	f, exist := registry[strategy]
	if !exist {
		return nil, fmt.Errorf("invalid QueueingStrategy %q", cq.Spec.QueueingStrategy)
	}
	return f(cq, metricRecorder)
}
