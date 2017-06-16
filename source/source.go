/*
Copyright 2017 The Kubernetes Authors.

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

package source

import (
	"github.com/kubernetes-incubator/external-dns/endpoint"
)

const (
	// The annotation used for figuring out which controller is responsible
	controllerAnnotationKey = "external-dns.alpha.kubernetes.io/controller"
	// The annotation used for defining the desired hostname
	hostnameAnnotationKey = "external-dns.alpha.kubernetes.io/hostname"
	// The value of the controller annotation so that we feel resposible
	controllerAnnotationValue = "dns-controller"

	RoleTypeExternal = "external"
	RoleTypeInternal = "internal"
)

// Source defines the interface Endpoint sources should implement.
type Source interface {
	Endpoints() ([]*endpoint.Endpoint, error)
}

func aliasForNodesInRole(role string, roleType string) string {
	return "node/role=" + role + "/" + roleType
}

func aliasForNodeName(nodeName string, roleType string) string {
	return "node/" + nodeName + "/" + roleType
}
