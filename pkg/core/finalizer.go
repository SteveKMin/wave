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

package core

// addFinalizer adds the wave finalizer to the given PodController
func addFinalizer(obj PodController) {
	finalizers := obj.GetFinalizers()
	for _, finalizer := range finalizers {
		if finalizer == FinalizerString {
			// PodController already contains the finalizer
			return
		}
	}

	//PodController doesn't contain the finalizer, so add it
	finalizers = append(finalizers, FinalizerString)
	obj.SetFinalizers(finalizers)
}

// removeFinalizer removes the wave finalizer from the given PodController
func removeFinalizer(obj PodController) {
	finalizers := obj.GetFinalizers()

	// Filter existing finalizers removing any that match the finalizerString
	newFinalizers := []string{}
	for _, finalizer := range finalizers {
		if finalizer != FinalizerString {
			newFinalizers = append(newFinalizers, finalizer)
		}
	}

	// Update the object's finalizers
	obj.SetFinalizers(newFinalizers)
}

// hasFinalizer checks for the presence of the Wave finalizer
func hasFinalizer(obj PodController) bool {
	finalizers := obj.GetFinalizers()
	for _, finalizer := range finalizers {
		if finalizer == FinalizerString {
			// PodController already contains the finalizer
			return true
		}
	}

	return false
}
