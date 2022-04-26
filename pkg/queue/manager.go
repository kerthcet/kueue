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
	"context"
	"errors"
	"fmt"
	"sync"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kueue "sigs.k8s.io/kueue/apis/kueue/v1alpha1"
	"sigs.k8s.io/kueue/pkg/metrics"
	"sigs.k8s.io/kueue/pkg/workload"
)

const (
	workloadQueueKey     = "spec.queueName"
	queueClusterQueueKey = "spec.clusterQueue"
)

var (
	errQueueDoesNotExist         = errors.New("queue doesn't exist")
	errClusterQueueDoesNotExist  = errors.New("clusterQueue doesn't exist")
	errClusterQueueAlreadyExists = errors.New("clusterQueue already exists")
)

type Manager struct {
	sync.RWMutex
	cond sync.Cond

	client        client.Client
	clusterQueues map[string]ClusterQueue
	queues        map[string]*Queue

	// Key is cohort's name. Value is a set of associated ClusterQueue names.
	cohorts map[string]sets.String
}

func NewManager(client client.Client) *Manager {
	m := &Manager{
		client:        client,
		queues:        make(map[string]*Queue),
		clusterQueues: make(map[string]ClusterQueue),
		cohorts:       make(map[string]sets.String),
	}
	m.cond.L = &m.RWMutex
	return m
}

func (m *Manager) AddClusterQueue(ctx context.Context, cq *kueue.ClusterQueue) error {
	m.Lock()
	defer m.Unlock()

	if _, ok := m.clusterQueues[cq.Name]; ok {
		return errClusterQueueAlreadyExists
	}

	cqImpl, err := newClusterQueue(cq, metrics.NewPendingWorkloadsInClusterQueueRecorder(cq.Name))
	if err != nil {
		return err
	}

	m.clusterQueues[cq.Name] = cqImpl

	cohort := cq.Spec.Cohort
	if cohort != "" {
		m.addCohort(cohort, cq.Name)
	}

	// Iterate through existing queues, as queues corresponding to this cluster
	// queue might have been added earlier.
	var queues kueue.QueueList
	if err := m.client.List(ctx, &queues, client.MatchingFields{queueClusterQueueKey: cq.Name}); err != nil {
		return fmt.Errorf("listing queues pointing to the cluster queue: %w", err)
	}
	addedWorkloads := false
	for _, q := range queues.Items {
		// Checking clusterQueue name again because the field index is not available in tests.
		if string(q.Spec.ClusterQueue) != cq.Name {
			continue
		}
		qImpl := m.queues[Key(&q)]
		if qImpl != nil {
			added := cqImpl.AddFromQueue(qImpl)
			addedWorkloads = addedWorkloads || added
		}
	}

	queued := m.queueAllInadmissibleWorkloadsInCohort(cqImpl)
	if queued || addedWorkloads {
		m.cond.Broadcast()
	}
	return nil
}

func (m *Manager) UpdateClusterQueue(cq *kueue.ClusterQueue) error {
	m.Lock()
	defer m.Unlock()
	cqImpl, ok := m.clusterQueues[cq.Name]
	if !ok {
		return errClusterQueueDoesNotExist
	}
	oldCohort := cqImpl.Cohort()

	// TODO(#8): recreate heap based on a change of queueing policy.
	cqImpl.Update(cq)
	newCohort := cqImpl.Cohort()
	if oldCohort != newCohort {
		m.updateCohort(oldCohort, newCohort, cq.Name)
	}

	// TODO(#8): Selectively move workloads based on the exact event.
	if m.queueAllInadmissibleWorkloadsInCohort(cqImpl) {
		m.cond.Broadcast()
	}

	return nil
}

func (m *Manager) DeleteClusterQueue(cq *kueue.ClusterQueue) {
	m.Lock()
	defer m.Unlock()
	cqImpl := m.clusterQueues[cq.Name]
	if cqImpl == nil {
		return
	}
	delete(m.clusterQueues, cq.Name)

	cohort := cq.Spec.Cohort
	if cohort != "" {
		m.deleteCohort(cohort, cq.Name)
	}
}

func (m *Manager) AddQueue(ctx context.Context, q *kueue.Queue) error {
	m.Lock()
	defer m.Unlock()

	key := Key(q)
	if _, ok := m.queues[key]; ok {
		return fmt.Errorf("queue %q already exists", q.Name)
	}
	qImpl := newQueue(q, metrics.NewPendingWorkloadsInQueueRecorder(q.Name, q.Namespace))
	m.queues[key] = qImpl
	// Iterate through existing workloads, as workloads corresponding to this
	// queue might have been added earlier.
	var workloads kueue.WorkloadList
	if err := m.client.List(ctx, &workloads, client.MatchingFields{workloadQueueKey: q.Name}, client.InNamespace(q.Namespace)); err != nil {
		return fmt.Errorf("listing workloads that match the queue: %w", err)
	}
	for _, w := range workloads.Items {
		w := w
		// Checking queue name again because the field index is not available in tests.
		if w.Spec.QueueName != q.Name || w.Spec.Admission != nil {
			continue
		}
		qImpl.AddOrUpdate(&w)
	}
	cq := m.clusterQueues[qImpl.ClusterQueue]
	if cq != nil && cq.AddFromQueue(qImpl) {
		m.cond.Broadcast()
	}
	return nil
}

func (m *Manager) UpdateQueue(q *kueue.Queue) error {
	m.Lock()
	defer m.Unlock()
	qImpl, ok := m.queues[Key(q)]
	if !ok {
		return errQueueDoesNotExist
	}
	if qImpl.ClusterQueue != string(q.Spec.ClusterQueue) {
		oldCQ := m.clusterQueues[qImpl.ClusterQueue]
		if oldCQ != nil {
			oldCQ.DeleteFromQueue(qImpl)
		}
		newCQ := m.clusterQueues[string(q.Spec.ClusterQueue)]
		if newCQ != nil && newCQ.AddFromQueue(qImpl) {
			m.cond.Broadcast()
		}
	}
	qImpl.update(q)
	return nil
}

func (m *Manager) DeleteQueue(q *kueue.Queue) {
	m.Lock()
	defer m.Unlock()
	key := Key(q)
	qImpl := m.queues[key]
	if qImpl == nil {
		return
	}
	cq := m.clusterQueues[qImpl.ClusterQueue]
	if cq != nil {
		cq.DeleteFromQueue(qImpl)
	}
	delete(m.queues, key)
}

func (m *Manager) PendingWorkloads(q *kueue.Queue) (int32, error) {
	m.RLock()
	defer m.RUnlock()

	qImpl, ok := m.queues[Key(q)]
	if !ok {
		return 0, errQueueDoesNotExist
	}

	return int32(len(qImpl.items)), nil
}

func (m *Manager) Pending(cq *kueue.ClusterQueue) int32 {
	m.RLock()
	defer m.RUnlock()
	return m.clusterQueues[cq.Name].Pending()
}

func (m *Manager) QueueForWorkloadExists(wl *kueue.Workload) bool {
	m.RLock()
	defer m.RUnlock()
	_, ok := m.queues[queueKeyForWorkload(wl)]
	return ok

}

// ClusterQueueForWorkload returns the name of the ClusterQueue where the
// workload should be queued and whether it exists.
// Returns empty string if the queue doesn't exist.
func (m *Manager) ClusterQueueForWorkload(wl *kueue.Workload) (string, bool) {
	m.RLock()
	defer m.RUnlock()
	q, ok := m.queues[queueKeyForWorkload(wl)]
	if !ok {
		return "", false
	}
	_, ok = m.clusterQueues[q.ClusterQueue]
	return q.ClusterQueue, ok
}

// AddOrUpdateWorkload adds or updates workload to the corresponding queue.
// Returns whether the queue existed.
func (m *Manager) AddOrUpdateWorkload(w *kueue.Workload) bool {
	m.Lock()
	defer m.Unlock()
	return m.addOrUpdateWorkload(w)
}

func (m *Manager) addOrUpdateWorkload(w *kueue.Workload) bool {
	qKey := queueKeyForWorkload(w)
	q := m.queues[qKey]
	if q == nil {
		return false
	}
	q.AddOrUpdate(w)
	cq := m.clusterQueues[q.ClusterQueue]
	if cq == nil {
		return false
	}
	cq.PushOrUpdate(w)
	m.cond.Broadcast()
	return true
}

// RequeueWorkload requeues the workload ensuring that the queue and the
// workload still exist in the client cache and it's not admitted. It won't
// requeue if the workload is already in the queue (possible if the workload was updated).
func (m *Manager) RequeueWorkload(ctx context.Context, info *workload.Info, immediate bool) bool {
	m.Lock()
	defer m.Unlock()

	q := m.queues[queueKeyForWorkload(info.Obj)]
	if q == nil {
		return false
	}

	var w kueue.Workload
	err := m.client.Get(ctx, client.ObjectKeyFromObject(info.Obj), &w)
	// Since the client is cached, the only possible error is NotFound
	if apierrors.IsNotFound(err) || w.Spec.Admission != nil {
		return false
	}

	q.AddIfNotPresent(info)
	cq := m.clusterQueues[q.ClusterQueue]
	if cq == nil {
		return false
	}

	added := cq.RequeueIfNotPresent(info, immediate)
	if added {
		m.cond.Broadcast()
	}
	return added
}

func (m *Manager) DeleteWorkload(w *kueue.Workload) {
	m.Lock()
	m.deleteWorkloadFromQueueAndClusterQueue(w, queueKeyForWorkload(w))
	m.Unlock()
}

func (m *Manager) deleteWorkloadFromQueueAndClusterQueue(w *kueue.Workload, qKey string) {
	q := m.queues[qKey]
	if q == nil {
		return
	}
	q.Delete(workload.Key(w))
	cq := m.clusterQueues[q.ClusterQueue]
	if cq != nil {
		cq.Delete(w)
	}
}

// QueueAssociatedInadmissibleWorkloads moves all associated workloads from
// inadmissibleWorkloads to heap. If at least one workload is moved,
// returns true. Otherwise returns false.
func (m *Manager) QueueAssociatedInadmissibleWorkloads(w *kueue.Workload) {
	m.Lock()
	defer m.Unlock()

	q := m.queues[queueKeyForWorkload(w)]
	if q == nil {
		return
	}

	cq := m.clusterQueues[q.ClusterQueue]
	if cq == nil {
		return
	}

	if m.queueAllInadmissibleWorkloadsInCohort(cq) {
		m.cond.Broadcast()
	}
}

// queueAllInadmissibleWorkloadsInCohort moves all workloads in the same
// cohort with this ClusterQueue from inadmissibleWorkloads to heap. If the
// cohort of this ClusterQueue is empty, it just moves all workloads in this
// ClusterQueue. If at least one workload is moved, returns true. Otherwise
// returns false.
// The events listed below could make workloads in the same cohort admissible.
// Then queueAllInadmissibleWorkloadsInCohort need to be invoked.
// 1. delete events for any admitted workload in the cohort.
// 2. add events of any cluster queue in the cohort.
// 3. update events of any cluster queue in the cohort.
func (m *Manager) queueAllInadmissibleWorkloadsInCohort(cq ClusterQueue) bool {
	cohort := cq.Cohort()
	if cohort == "" {
		return cq.QueueInadmissibleWorkloads()
	}

	queued := false
	for cqName := range m.cohorts[cohort] {
		if clusterQueue, ok := m.clusterQueues[cqName]; ok {
			queued = clusterQueue.QueueInadmissibleWorkloads() || queued
		}
	}
	return queued
}

// UpdateWorkload updates the workload to the corresponding queue or adds it if
// it didn't exist. Returns whether the queue existed.
func (m *Manager) UpdateWorkload(oldW, w *kueue.Workload) bool {
	m.Lock()
	defer m.Unlock()
	if oldW.Spec.QueueName != w.Spec.QueueName {
		m.deleteWorkloadFromQueueAndClusterQueue(w, queueKeyForWorkload(oldW))
	}
	return m.addOrUpdateWorkload(w)
}

// CleanUpOnContext tracks the context. When closed, it wakes routines waiting
// on elements to be available. It should be called before doing any calls to
// Heads.
func (m *Manager) CleanUpOnContext(ctx context.Context) {
	<-ctx.Done()
	m.cond.Broadcast()
}

// Heads returns the heads of the queues, along with their associated ClusterQueue.
// It blocks if the queues empty until they have elements or the context terminates.
func (m *Manager) Heads(ctx context.Context) []workload.Info {
	m.Lock()
	defer m.Unlock()
	log := ctrl.LoggerFrom(ctx)
	for {
		workloads := m.heads()
		log.V(3).Info("Obtained ClusterQueue heads", "count", len(workloads))
		if len(workloads) != 0 {
			return workloads
		}
		select {
		case <-ctx.Done():
			return nil
		default:
			m.cond.Wait()
		}
	}
}

// Dump is a dump of the queues and it's elements (unordered).
// Only use for testing purposes.
func (m *Manager) Dump() map[string]sets.String {
	m.Lock()
	defer m.Unlock()
	if len(m.queues) == 0 {
		return nil
	}
	dump := make(map[string]sets.String, len(m.queues))
	for key, cq := range m.clusterQueues {
		if elements, ok := cq.Dump(); ok {
			dump[key] = elements
		}
	}
	if len(dump) == 0 {
		return nil
	}
	return dump
}

func (m *Manager) heads() []workload.Info {
	var workloads []workload.Info
	for cqName, cq := range m.clusterQueues {
		wl := cq.Pop()
		if wl == nil {
			continue
		}
		wlCopy := *wl
		wlCopy.ClusterQueue = cqName
		workloads = append(workloads, wlCopy)
		q := m.queues[queueKeyForWorkload(wl.Obj)]
		q.Delete(workload.Key(wl.Obj))
	}
	return workloads
}

func (m *Manager) addCohort(cohort string, cqName string) {
	if m.cohorts[cohort] == nil {
		m.cohorts[cohort] = make(sets.String)
	}
	m.cohorts[cohort].Insert(cqName)
}

func (m *Manager) deleteCohort(cohort string, cqName string) {
	if m.cohorts[cohort] != nil {
		m.cohorts[cohort].Delete(cqName)
		if len(m.cohorts[cohort]) == 0 {
			delete(m.cohorts, cohort)
		}
	}
}

func (m *Manager) updateCohort(oldCohort string, newCohort string, cqName string) {
	m.deleteCohort(oldCohort, cqName)
	m.addCohort(newCohort, cqName)
}

func SetupIndexes(indexer client.FieldIndexer) error {
	err := indexer.IndexField(context.Background(), &kueue.Workload{}, workloadQueueKey, func(o client.Object) []string {
		wl := o.(*kueue.Workload)
		return []string{wl.Spec.QueueName}
	})
	if err != nil {
		return fmt.Errorf("setting index on queue for Workload: %w", err)
	}
	err = indexer.IndexField(context.Background(), &kueue.Queue{}, queueClusterQueueKey, func(o client.Object) []string {
		q := o.(*kueue.Queue)
		return []string{string(q.Spec.ClusterQueue)}
	})
	if err != nil {
		return fmt.Errorf("setting index on clusterQueue for Queue: %w", err)
	}
	return nil
}
