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
	"bytes"
	"fmt"
	"html/template"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/kubernetes-incubator/external-dns/endpoint"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
)

type podSource struct {
	client    kubernetes.Interface
	namespace string
	// process Services with legacy annotations
	compatibility string
	fqdntemplate  *template.Template
}

func NewPodSource(client kubernetes.Interface, namespace, fqdntemplate string, compatibility string) (Source, error) {
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

	return &podSource{
		client:        client,
		namespace:     namespace,
		compatibility: compatibility,
		fqdntemplate:  tmpl,
	}, nil
}

func (ps *podSource) Endpoints() ([]*endpoint.Endpoint, error) {
	pods, err := ps.client.CoreV1().Pods(ps.namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	endpoints := []*endpoint.Endpoint{}

	for _, pod := range pods.Items {
		// Check controller annotation to see if we are responsible.
		controller, ok := pod.Annotations[controllerAnnotationKey]
		if ok && controller != controllerAnnotationValue {
			log.Debugf("Skipping pod %s/%s because controller value does not match, found: %s, required: %s",
				pod.Namespace, pod.Name, controller, controllerAnnotationValue)
			continue
		}

		podEndpoints := endpointsFromPod(&pod)

		// process legacy annotations if no endpoints were returned and compatibility mode is enabled.
		if len(podEndpoints) == 0 && ps.compatibility != "" {
			podEndpoints = legacyEndpointsFromPod(&pod, ps.compatibility)
		}

		// apply template if none of the above is found
		if len(podEndpoints) == 0 && ps.fqdntemplate != nil {
			podEndpoints, err = ps.endpointsFromTemplate(&pod)
			if err != nil {
				return nil, err
			}
		}

		if len(podEndpoints) == 0 {
			log.Debugf("No endpoints could be generated from service %s/%s", pod.Namespace, pod.Name)
			continue
		}

		log.Debugf("Endpoints generated from service: %s/%s: %v", pod.Namespace, pod.Name, podEndpoints)
		endpoints = append(endpoints, podEndpoints...)
	}

	return endpoints, nil
}

func (ps *podSource) endpointsFromTemplate(pod *v1.Pod) ([]*endpoint.Endpoint, error) {
	var endpoints []*endpoint.Endpoint

	var buf bytes.Buffer
	err := ps.fqdntemplate.Execute(&buf, pod)
	if err != nil {
		return nil, fmt.Errorf("failed to apply template on pod %s: %v", pod.String(), err)
	}

	hostname := buf.String()
	if pod.Spec.HostNetwork {
		nodeName := pod.Spec.NodeName
		if nodeName != "" {
			endpoints = append(endpoints, endpoint.NewEndpoint(hostname, aliasForNodeName(nodeName, RoleTypeExternal), endpoint.RecordTypeInternalALIAS))
		}
	}

	return endpoints, nil
}

func endpointsFromPod(pod *v1.Pod) []*endpoint.Endpoint {
	var endpoints []*endpoint.Endpoint

	// Get the desired hostname of the service from the annotation.
	hostname, exists := pod.Annotations[hostnameAnnotationKey]
	if !exists {
		return nil
	}

	if pod.Spec.HostNetwork {
		nodeName := pod.Spec.NodeName
		if nodeName != "" {
			endpoints = append(endpoints, endpoint.NewEndpoint(hostname, aliasForNodeName(nodeName, RoleTypeExternal), endpoint.RecordTypeInternalALIAS))
		}
	} else {
		log.Debugf("Pod %q had %s, but was not HostNetwork", pod.Name, hostnameAnnotationKey)
	}

	return endpoints
}
