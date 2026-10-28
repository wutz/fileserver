/*
Copyright 2026.

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

package v1alpha1

// The names and keys below are a contract with the clients that create and
// wake file servers, and with the volumes they serve. They are kept from the
// forgeai platform, so existing file servers and clients keep working.
const (
	// DefaultName is the name of the file server a client creates in a namespace.
	DefaultName = "default"
	// SecretName is the Secret that holds the basic auth username and password
	// of a file server. The operator creates it in the file server's namespace.
	SecretName = "forgeai-fileserver"
	// AnnotationLastActive is a client heartbeat on a FileServer (RFC3339).
	// The file server runs while the heartbeat is recent.
	AnnotationLastActive = "fileserver.wutz.dev/last-active"
	// LabelIgnore marks a PersistentVolumeClaim that file servers must not serve,
	// with the value "true". Clients set it on volumes they trash; the operator
	// does not serve them.
	LabelIgnore = "fileserver.wutz.dev/ignore"
)
