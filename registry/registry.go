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

package registry

import (
	log "github.com/Sirupsen/logrus"
	"github.com/kubernetes-incubator/external-dns/endpoint"
	"github.com/kubernetes-incubator/external-dns/plan"
)

// Registry is an interface which should enables ownership concept in external-dns
// Records() returns ALL records registered with DNS provider (TODO: for multi-zone support return all records)
// each entry includes owner information
// ApplyChanges(changes *plan.Changes) propagates the changes to the DNS Provider API and correspondingly updates ownership depending on type of registry being used
type Registry interface {
	Records() ([]*endpoint.Endpoint, error)
	ApplyChanges(changes *plan.Changes) error
}

//TODO(ideahitme): consider moving this to Plan
func filterOwnedRecords(ownerID string, endpointSets []*endpoint.EndpointSet) []*endpoint.EndpointSet {
	filtered := []*endpoint.EndpointSet{}
	for _, endpointSet := range endpointSets {
		if endpointOwner, ok := endpointSet.Labels[endpoint.OwnerLabelKey]; !ok || endpointOwner != ownerID {
			log.Debugf(`Skipping EndpointSet %v because owner id does not match, found: "%s", required: "%s"`, endpointSet, endpointOwner, ownerID)
			continue
		}
		filtered = append(filtered, endpointSet)
	}
	return filtered
}
