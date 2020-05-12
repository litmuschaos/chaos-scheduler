/*
Copyright The Kubernetes Authors.

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

// Code generated by lister-gen. DO NOT EDIT.

package v1alpha1

import (
	v1alpha1 "github.com/litmuschaos/chaos-scheduler/pkg/apis/litmuschaos/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// ChaosScheduleLister helps list ChaosSchedules.
type ChaosScheduleLister interface {
	// List lists all ChaosSchedules in the indexer.
	List(selector labels.Selector) (ret []*v1alpha1.ChaosSchedule, err error)
	// ChaosSchedules returns an object that can list and get ChaosSchedules.
	ChaosSchedules(namespace string) ChaosScheduleNamespaceLister
	ChaosScheduleListerExpansion
}

// chaosScheduleLister implements the ChaosScheduleLister interface.
type chaosScheduleLister struct {
	indexer cache.Indexer
}

// NewChaosScheduleLister returns a new ChaosScheduleLister.
func NewChaosScheduleLister(indexer cache.Indexer) ChaosScheduleLister {
	return &chaosScheduleLister{indexer: indexer}
}

// List lists all ChaosSchedules in the indexer.
func (s *chaosScheduleLister) List(selector labels.Selector) (ret []*v1alpha1.ChaosSchedule, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.ChaosSchedule))
	})
	return ret, err
}

// ChaosSchedules returns an object that can list and get ChaosSchedules.
func (s *chaosScheduleLister) ChaosSchedules(namespace string) ChaosScheduleNamespaceLister {
	return chaosScheduleNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// ChaosScheduleNamespaceLister helps list and get ChaosSchedules.
type ChaosScheduleNamespaceLister interface {
	// List lists all ChaosSchedules in the indexer for a given namespace.
	List(selector labels.Selector) (ret []*v1alpha1.ChaosSchedule, err error)
	// Get retrieves the ChaosSchedule from the indexer for a given namespace and name.
	Get(name string) (*v1alpha1.ChaosSchedule, error)
	ChaosScheduleNamespaceListerExpansion
}

// chaosScheduleNamespaceLister implements the ChaosScheduleNamespaceLister
// interface.
type chaosScheduleNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all ChaosSchedules in the indexer for a given namespace.
func (s chaosScheduleNamespaceLister) List(selector labels.Selector) (ret []*v1alpha1.ChaosSchedule, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.ChaosSchedule))
	})
	return ret, err
}

// Get retrieves the ChaosSchedule from the indexer for a given namespace and name.
func (s chaosScheduleNamespaceLister) Get(name string) (*v1alpha1.ChaosSchedule, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("chaosschedule"), name)
	}
	return obj.(*v1alpha1.ChaosSchedule), nil
}
