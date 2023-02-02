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

package job

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	kueue "sigs.k8s.io/kueue/apis/kueue/v1alpha2"
)

type GenericJob interface {
	// Object returns the job instance.
	Object() client.Object
	// IsSuspend returns whether the job is suspend or not.
	IsSuspend() bool
	// Suspend will suspend the job.
	Suspend() error
	// UnSuspend will unsuspend the job.
	UnSuspend() error
	// ResetStatus will reset the job status to the original state.
	// If true, status is modified, if not, status is as it was.
	ResetStatus() bool
	// InjectNodeAffinity will inject the node affinity extracting from workload to job.
	InjectNodeAffinity(nodeSelectors []map[string]string) error
	// RestoreNodeAffinity will restore the original node affinity of job.
	RestoreNodeAffinity(nodeSelectors []map[string]string) error
	// Finished means whether the job is completed/failed or not,
	// condition represents the workload finished condition.
	Finished() (condition metav1.Condition, finished bool)
	// PodSets will build workload podSets corresponding to the job.
	PodSets() []kueue.PodSet
	// EquivalentToWorkload validates whether the workload is semantically equal to the job.
	EquivalentToWorkload(wl kueue.Workload) bool
	// PriorityClass returns the job's priority class name.
	PriorityClass() string
	// QueueName returns the queue name the job enqueued.
	QueueName() string
	// Ignored instructs whether this job should be ignored in reconciling, e.g. lacking the queueName.
	Ignored() bool
}

type SequenceJob interface {
	// PodsReady instructs whether job derived pods are all ready now.
	PodsReady() bool
}
