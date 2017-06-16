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
	"html/template"
	"strings"

	"github.com/kubernetes-incubator/external-dns/endpoint"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
)

type nodeSource struct {
	client       kubernetes.Interface
	fqdntemplate *template.Template
}

func NewNodeSource(client kubernetes.Interface, fqdntemplate string) (Source, error) {
	var tmpl *template.Template
	var err error
	if fqdntemplate != "" {
		tmpl, err = template.New("endpoint").Funcs(template.FuncMap{
			"trimPrefix": strings.TrimPrefix,
		}).Parse(fqdntemplate)
		if err != nil {
			return nil, err
		}
	}

	return &nodeSource{
		client:       client,
		fqdntemplate: tmpl,
	}, nil
}

func (ns *nodeSource) Endpoints() ([]*endpoint.Endpoint, error) {
	nodes, err := ns.client.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	endpoints := []*endpoint.Endpoint{}

	for _, node := range nodes.Items {
		for _, address := range node.Status.Addresses {
			if address.Type == v1.NodeInternalIP {
				// node/<name>/internal -> InternalIP
				endpoints = append(endpoints, endpoint.NewAliasTargetEndpoint(aliasForNodeName(node.Name, RoleTypeInternal), address.Address, endpoint.RecordTypeA))
			} else if address.Type == v1.NodeExternalIP {
				// node/<name>/external -> ExternalIP
				endpoints = append(endpoints, endpoint.NewAliasTargetEndpoint(aliasForNodeName(node.Name, RoleTypeExternal), address.Address, endpoint.RecordTypeA))
			}
		}

		role := getNodeRole(&node)

		for _, address := range node.Status.Addresses {
			var roleType string
			if address.Type == v1.NodeInternalIP {
				// node/role=<role>/internal -> InternalIP
				roleType = RoleTypeInternal
			} else if address.Type == v1.NodeExternalIP {
				// node/role=<role>/external -> ExternalIP
				roleType = RoleTypeExternal
			} else {
				continue
			}
			endpoints = append(endpoints, endpoint.NewAliasTargetEndpoint(aliasForNodesInRole(role, roleType), address.Address, endpoint.RecordTypeA))
		}
	}

	return endpoints, nil
}

func getNodeRole(node *v1.Node) string {
	role := ""
	// Newer labels
	for k := range node.Labels {
		if strings.HasPrefix(k, "node-role.kubernetes.io/") {
			role = strings.TrimPrefix(k, "node-role.kubernetes.io/")
		}
	}
	// Older label
	if role == "" {
		role = node.Labels["kubernetes.io/role"]
	}

	return role
}
