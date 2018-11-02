/*
Copyright 2018 Pusher Ltd.

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

package deployment

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	// configHashAnnotation is the key of the annotation on the PodTemplate that
	// holds the configuratio hash
	configHashAnnotation = "wave.pusher.com/config-hash"

	// finalizerString is the finalizer added to deployments to allow Wave to
	// perform advanced deletion logic
	finalizerString = "wave.pusher.com/finalizer"

	// requiredAnnotation is the key of the annotation on the Deployment that Wave
	// checks for before processing the deployment
	requiredAnnotation = "wave.pusher.com/update-on-config-change"
)

// object is used as a helper interface when passing Kubernetes resources
// between methods.
// All Kubernetes resources should implement both of these interfaces
type object interface {
	runtime.Object
	metav1.Object
}