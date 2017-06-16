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
	"k8s.io/client-go/pkg/api/v1"

	log "github.com/Sirupsen/logrus"
	"github.com/kubernetes-incubator/external-dns/endpoint"
)

const (
	mateAnnotationKey                  = "zalando.org/dnsname"
	moleculeAnnotationKey              = "domainName"
	dnsControllerExternalAnnotationKey = "dns.alpha.kubernetes.io/external-test"
	dnsControllerInternalAnnotationKey = "dns.alpha.kubernetes.io/internal-test"

	compatibilityMate          = "mate"
	compatibilityMolecule      = "molecule"
	compatibilityDnsController = "dnscontroller"
)

// legacyEndpointsFromService tries to retrieve Endpoints from Services
// annotated with legacy annotations.
func legacyEndpointsFromService(svc *v1.Service, compatibility string) []*endpoint.Endpoint {
	switch compatibility {
	case compatibilityMate:
		return legacyEndpointsFromMateService(svc)
	case compatibilityMolecule:
		return legacyEndpointsFromMoleculeService(svc)
	case compatibilityDnsController:
		return legacyEndpointsFromDnsControllerService(svc)
	}

	return []*endpoint.Endpoint{}
}

func legacyEndpointsFromPod(pod *v1.Pod, compatibility string) []*endpoint.Endpoint {
	switch compatibility {
	case compatibilityDnsController:
		return legacyEndpointsFromDnsControllerPod(pod)
	}

	return []*endpoint.Endpoint{}
}

// legacyEndpointsFromMateService tries to retrieve Endpoints from Services
// annotated with Mate's annotation semantics.
func legacyEndpointsFromMateService(svc *v1.Service) []*endpoint.Endpoint {
	var endpoints []*endpoint.Endpoint

	// Get the desired hostname of the service from the annotation.
	hostname, exists := svc.Annotations[mateAnnotationKey]
	if !exists {
		return nil
	}

	// Create a corresponding endpoint for each configured external entrypoint.
	for _, lb := range svc.Status.LoadBalancer.Ingress {
		if lb.IP != "" {
			endpoints = append(endpoints, endpoint.NewEndpoint(hostname, lb.IP, endpoint.RecordTypeA))
		}
		if lb.Hostname != "" {
			endpoints = append(endpoints, endpoint.NewEndpoint(hostname, lb.Hostname, endpoint.RecordTypeCNAME))
		}
	}

	return endpoints
}

// legacyEndpointsFromMoleculeService tries to retrieve Endpoints from Services
// annotated with Molecule Software's annotation semantics.
func legacyEndpointsFromMoleculeService(svc *v1.Service) []*endpoint.Endpoint {
	var endpoints []*endpoint.Endpoint

	// Check that the Service opted-in to being processed.
	if svc.Labels["dns"] != "route53" {
		return nil
	}

	// Get the desired hostname of the service from the annotation.
	hostname, exists := svc.Annotations[moleculeAnnotationKey]
	if !exists {
		return nil
	}

	// Create a corresponding endpoint for each configured external entrypoint.
	for _, lb := range svc.Status.LoadBalancer.Ingress {
		if lb.IP != "" {
			endpoints = append(endpoints, endpoint.NewEndpoint(hostname, lb.IP, endpoint.RecordTypeA))
		}
		if lb.Hostname != "" {
			endpoints = append(endpoints, endpoint.NewEndpoint(hostname, lb.Hostname, endpoint.RecordTypeCNAME))
		}
	}

	return endpoints
}

func legacyEndpointsFromDnsControllerService(svc *v1.Service) []*endpoint.Endpoint {
	var endpoints []*endpoint.Endpoint

	internal, internalExists := svc.Annotations[dnsControllerInternalAnnotationKey]
	external, externalExists := svc.Annotations[dnsControllerExternalAnnotationKey]

	if !internalExists && !externalExists {
		return nil
	}

	if svc.Spec.Type == v1.ServiceTypeLoadBalancer {
		for _, lb := range svc.Status.LoadBalancer.Ingress {
			if lb.IP != "" {
				if internalExists {
					endpoints = append(endpoints, endpoint.NewEndpoint(internal, lb.IP, endpoint.RecordTypeA))
				}

				if externalExists {
					endpoints = append(endpoints, endpoint.NewEndpoint(external, lb.IP, endpoint.RecordTypeA))
				}
			}

			if lb.Hostname != "" {
				if internalExists {
					endpoints = append(endpoints, endpoint.NewEndpoint(internal, lb.Hostname, endpoint.RecordTypeCNAME))
				}

				if externalExists {
					endpoints = append(endpoints, endpoint.NewEndpoint(external, lb.Hostname, endpoint.RecordTypeCNAME))
				}
			}
		}
	} else if svc.Spec.Type == v1.ServiceTypeNodePort {
		if internalExists && externalExists {
			log.Debug("DNS Records not possible for both Internal and Externals IPs.")
		} else if internalExists {
			endpoints = append(endpoints, endpoint.NewEndpoint(internal, aliasForNodesInRole("node", RoleTypeInternal), endpoint.RecordTypeInternalALIAS))
		} else if externalExists {
			endpoints = append(endpoints, endpoint.NewEndpoint(external, aliasForNodesInRole("node", RoleTypeExternal), endpoint.RecordTypeInternalALIAS))
		}
	}

	return endpoints
}

func legacyEndpointsFromDnsControllerPod(pod *v1.Pod) []*endpoint.Endpoint {
	var endpoints []*endpoint.Endpoint

	// Get the desired hostname of the pod from the annotation.
	internal, internalExists := pod.Annotations[dnsControllerInternalAnnotationKey]
	external, externalExists := pod.Annotations[dnsControllerExternalAnnotationKey]

	if !internalExists && !externalExists {
		return nil
	}

	if internalExists {
		if pod.Spec.HostNetwork {
			podIP := pod.Status.PodIP
			if podIP != "" {
				endpoints = append(endpoints, endpoint.NewEndpoint(internal, podIP, endpoint.RecordTypeA))
			}
		} else {
			log.Debugf("Pod %q had %s=%s, but was not HostNetwork", pod.Name, dnsControllerInternalAnnotationKey, internal)
		}
	}

	if externalExists {
		if pod.Spec.HostNetwork {
			nodeName := pod.Spec.NodeName
			if nodeName != "" {
				endpoints = append(endpoints, endpoint.NewEndpoint(external, aliasForNodeName(nodeName, RoleTypeExternal), endpoint.RecordTypeInternalALIAS))
			}
		} else {
			log.Debugf("Pod %q had %s=%s, but was not HostNetwork", pod.Name, dnsControllerExternalAnnotationKey, external)
		}
	}

	return endpoints
}
