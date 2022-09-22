# Dynamically reclaim resources of Pods of a Workload

## Table of Contents

<!-- toc -->
- [Summary](#summary)
- [Motivation](#motivation)
    - [Goals](#goals)
    - [Non-Goals](#non-goals)
- [Proposal](#proposal)
    - [Pod Successful Completion Cases](#pod-successful-completion-cases)
        - [Job Parallelism Equal To 1](#job-parallelism-equal-to-1)
        - [Job Parallelism Greater Than 1](#job-parallelism-greater-than-1)
    - [Pod Failure Cases](#pod-failure-cases)
        - [RestartPolicy of Pods as Never](#restartpolicy-of-pods-as-never)
        - [RestartPolicy of Pods as OnFailure](#restartpolicy-of-pods-as-onfailure)
- [API](#api)
    - [ReclaimedPodSetPods](#reclaimedpodsetpods)
- [Implementation](#implementation)
- [Testing Plan](#testing-plan)
    - [Unit Tests](#unit-tests)
    - [Integration tests](#integration-tests)
- [Implementation History](#implementation-history)
<!-- /toc -->

## Summary

This proposal allows for the resources of Pods of a Workload to be reclaimed after successful completion of each of the Pods of a Workload. Freeing the unused resources of a Workload earlier may allow more pending Workloads to be admitted. This will allow Kueue to utilize the existing resources of the cluster much more efficiently.

In the remaining of this document, *Pods of a Workload* means the same as *Pods of a Job* and vice-versa.


## Motivation

Currently, the resources owned by a Job are reclaimed by kueue only when the whole Job finishes. For Jobs with multiple Pods, the resources will be reclaimed only after the last Pod of the Job finishes. This is not efficient as the Pods of parallel Job may have laggards consuming the unused resources of the Jobs those are currently executing.

### Goals

- Utilize the unused resources of the successfully completed Pods of a running Job.

### Non-Goals

- Preempting a Job or Pods of a Job to allocate resource to a pending Workload.
- Partially admitting a Workload.

## Proposal

Reclaiming the resources of the succeeded Pods of a running Job as soon as it completes its execution.

We propose to add a new field `.status.ReclaimedPodSetPods` to the Workload API. Workload controller will be responsible for updating the field `.status.ReclaimedPodSetPods` according to the status of the workload's Pods.

`.status.ReclaimedPodSetPods` will keep the mapping between the PodSet name to the count and names of the successfully completed Pods belonging to the Workload. If a name of a Pod of a Workload is present in the field then it means that the resources associated with the completed Pods have been reclaimed. The name of the Pod is recorded to avoid reclaiming the resources of the Pod more than once.

Now we will look at the Pod failure cases and how the reclaiming of resources by Kueue will happen.

### Pod Successful Completion Cases

#### Job Parallelism Equal To 1

By default, the `.spec.parallelism` of Job is equal to `1`.  In this case, if the Pod completes with successful execution, the whole Job execution can be considered as a success. Hence, the resource associated with the Job will be reclaimed by Kueue after the Job completion. This functionality is present today as well and no change will be required.

#### Job Parallelism Greater Than 1

Whenever a Pod of a Job successfully completes its execution, Kueue will reclaim the resources that were associated with successful Pod. The Job might be still in `Running` state as there could be other Pods of the Job that are executing. Thus, Kueue will reclaim the resources of the succeeded Pods of the Job in an incremental manner until the Job completes in either `Succeeded` or `Failed` state. Whenever the Job completes, the only remaining owned resources of the Job will be reclaimed by Kueue.

### Pod Failure Cases

#### RestartPolicy of Pods as Never

Whenever a Pod of a Job fails, the Job will recreate a Pod as replacement against the failed Pod. The Job will continue this process of creating new Pods until the Job reaches its `backoffLimit`(by default the backoffLimit is `6`). The newly created Pod against the failed Pod will reuse the resources of the failed Pod. Once the Job reaches its `backoffLimit` and does not have the required successful `.spec.completions` count, the Job is termed as a `Failed` Job. When the Job is marked as failed, no more Pods would be created by the Job. So, the remaining owned resources of the Job will be reclaimed by queue. 

#### RestartPolicy of Pods as OnFailure

When the Pods of the Job have `.spec.template.spec.restartPolicy = "OnFailure"`, the failed Pod stays on the node and the failed container within the Pod is restarted. The Pod might run indefinitely or get marked as `Failed` after the `.spec.activeDeadlineSeconds` is completed by Job if specified. In this case also, Kueue won't reclaim the resources of the Job until the Job gets completed as either `Failed` or `Succeeded`.

Hence, as seen from the above discussed Pod failure cases, we conclude that the Pods of a Job which fail during its execution, its resources are not immediately reclaimed by Kueue. Only when the Job gets marks as `Failed`, the resources of failed Pods will be reclaimed by Kueue.

## API

A new field `ReclaimedPodSetPods` is added to the `.status` of Workload API.

```go
// WorkloadStatus defines the observed state of Workload
type WorkloadStatus struct {
    // conditions hold the latest available observations of the Workload
    // current state.
    // +optional
    // +listType=map
    // +listMapKey=type
    Conditions []WorkloadCondition `json:"conditions,omitempty"`

    // ReclaimedPodSetPods are the pods (by PodSet name) which have
    //completed their execution successfully and resources of the pods
	//are reclaimed.
    // +optional
    ReclaimedPodSetPods ReclaimedPodSetPods `json:"reclaimedPodSetPods"`
}

type ReclaimedPodSetPods map[string]struct{
    Count int
    Pods []string
}
```

#### ReclaimedPodSetPods
`.status.reclaimedPodSetPods` holds the mapping between PodSet name to the count of Pods and Pod names that have successfully completed its execution and their resources have been reclaimed by Kueue. The names of Pods are recorded to identify the Pods whose resources have been reclaimed by Kueue. If a name of a Pod already exists in the list then the resources associated with the succeeded Pod have already been reclaimed by Kueue.

## Implementation

The Workload reconciler will keep a watch(or `Watches()`) on the Pods of the Job. The Workload reconciler will process the events of the Pods and update `.status.reclaimedPodSetPods` field accordingly.

## Testing Plan

Dynamically reclaiming resources enhancement has unit and integration tests. These tests
are run regularly as a part of kueue's prow CI/CD pipeline.

### Unit Tests

All the Kueue's core components must be covered by unit tests.  Here is a list of unit tests required for the  modules of the feature:

* [Cluster-Queue cache tests](https://github.com/kubernetes-sigs/kueue/blob/main/pkg/cache/cache_test.go)
* [Workload tests](https://github.com/kubernetes-sigs/kueue/blob/main/pkg/workload/workload_test.go)

### Integration tests
* Kueue Job Controller
  - checking the resources owned by a Job are released to the cache and clusterQueue when a Pod of the Job succeed.
  - Integration tests for Job controller are [found here](https://github.com/kubernetes-sigs/kueue/blob/main/test/integration/controller/job/job_controller_test.go).

* Workload Controller
  - A pending Workload should be admitted when enough resources are available after release of resources by the succeeded Pods of the parallel Jobs.
  - Should update the `.spec.reclaimedPodSetPods` of a Workload when a Pod of a Job succeeds.
  - Integration tests for Workload Controller are [found here](https://github.com/kubernetes-sigs/kueue/blob/main/test/integration/controller/core/workload_controller_test.go).

* Scheduler
  - Checking if a Workload gets admitted when an active parallel Job releases resources of completed Pods.
  - Integration tests for Scheduler are [found here](https://github.com/kubernetes-sigs/kueue/blob/main/test/integration/scheduler/scheduler_test.go).

## Implementation History

Dynamically Reclaiming Resources are tracked as part of [enhancement#78](https://github.com/kubernetes-sigs/kueue/issues/78).

**TODO** - Add proposal link