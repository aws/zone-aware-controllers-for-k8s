/*
Copyright 2016 The Kubernetes Authors.
Modifications Copyright 2022 Amazon.com, Inc. or its affiliates. All Rights Reserved.

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

package utils

import (
	"context"

	apps "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PodControllerFinder functions used to return the controller associated to a pod.
// This is necessary to be able to retrieve what is the expected number of replicas (aka scale) in the ZDB controller.
//
// This code is based on the `finders` related functions defined in the k8s' disruption controller:
// https://github.com/kubernetes/kubernetes/blob/d7123a65248/pkg/controller/disruption/disruption.go#L182

// ControllerAndScale is used to return (controller, scale) pairs from the
// controller finder functions.
type ControllerAndScale struct {
	types.UID
	Scale int32
}

// PodControllerFinder is a function type that maps a pod to a list of
// controllers and their scale.
type PodControllerFinder func(client client.Client, controllerRef *metav1.OwnerReference, namespace string) (*ControllerAndScale, error)

// Only StatefulSets are supported for now!!
// TODO Implement finders of other controler types (e.g Deployment, ReplicaSet)
func Finders() []PodControllerFinder {
	return []PodControllerFinder{getPodStatefulSet}
}

var (
	ControllerKindSS = apps.SchemeGroupVersion.WithKind("StatefulSet")
)

// getPodStatefulSet returns the statefulset referenced by the provided controllerRef.
func getPodStatefulSet(c client.Client, controllerRef *metav1.OwnerReference, namespace string) (*ControllerAndScale, error) {
	ok, err := verifyGroupKind(controllerRef, ControllerKindSS.Kind, []string{"apps"})
	if !ok || err != nil {
		return nil, err
	}

	ss := &apps.StatefulSet{}
	err = c.Get(context.TODO(), client.ObjectKey{Namespace: namespace, Name: controllerRef.Name}, ss)
	if err != nil {
		// NotFound is ok here.
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if ss.UID != controllerRef.UID {
		return nil, nil
	}

	return &ControllerAndScale{ss.UID, *(ss.Spec.Replicas)}, nil
}

func verifyGroupKind(controllerRef *metav1.OwnerReference, expectedKind string, expectedGroups []string) (bool, error) {
	gv, err := schema.ParseGroupVersion(controllerRef.APIVersion)
	if err != nil {
		return false, err
	}

	if controllerRef.Kind != expectedKind {
		return false, nil
	}

	for _, group := range expectedGroups {
		if group == gv.Group {
			return true, nil
		}
	}

	return false, nil
}
