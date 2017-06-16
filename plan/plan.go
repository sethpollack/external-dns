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

package plan

import (
	log "github.com/Sirupsen/logrus"
	"github.com/kubernetes-incubator/external-dns/endpoint"
)

type RecordKey struct {
	RecordType string
	DNSName    string
}

// Plan can convert a list of desired and current records to a series of create,
// update and delete actions.
type Plan struct {
	Aliases map[string][]*endpoint.Endpoint

	Labels map[RecordKey]map[string]string

	CurrentTargets map[RecordKey][]string

	RecordTargets map[RecordKey][]string
	// Policies under which the desired changes are calculated
	Policies []Policy
	// List of changes necessary to move towards desired state
	// Populated after calling Calculate()
	Changes *Changes
}

// List of changes necessary to move towards desired state
func NewPlan(current, desired []*endpoint.Endpoint, policy Policy) *Plan {
	plan := &Plan{
		Policies:       []Policy{policy},
		Aliases:        make(map[string][]*endpoint.Endpoint),
		CurrentTargets: make(map[RecordKey][]string),
		RecordTargets:  make(map[RecordKey][]string),
		Labels:         make(map[RecordKey]map[string]string),
	}

	records := []*endpoint.Endpoint{}

	// collect aliases
	for _, ep := range desired {
		if ep.AliasTarget {
			plan.Aliases[ep.DNSName] = append(plan.Aliases[ep.DNSName], ep)
		} else {
			records = append(records, ep)
		}
	}

	// aggregate desired endpoint target values
	for _, ep := range records {
		if ep.RecordType == endpoint.RecordTypeInternalALIAS {
			// expand aliases
			aliases := plan.Aliases[ep.Target]
			for _, alias := range aliases {
				key := RecordKey{
					RecordType: alias.RecordType,
					DNSName:    ep.DNSName,
				}
				plan.RecordTargets[key] = append(plan.RecordTargets[key], alias.Target)
			}
		} else {
			key := RecordKey{
				RecordType: ep.RecordType,
				DNSName:    ep.DNSName,
			}
			plan.RecordTargets[key] = append(plan.RecordTargets[key], ep.Target)
		}
	}

	// aggregate current endpoint target values
	for _, ep := range current {
		key := RecordKey{
			RecordType: ep.RecordType,
			DNSName:    ep.DNSName,
		}
		plan.Labels[key] = ep.Labels
		plan.CurrentTargets[key] = append(plan.CurrentTargets[key], ep.Target)
	}

	return plan
}

// Changes holds lists of actions to be executed by dns providers
type Changes struct {
	// Records that need to be created
	Create []*endpoint.EndpointSet
	// Records that need to be updated (current data)
	UpdateOld []*endpoint.EndpointSet
	// Records that need to be updated (desired data)
	UpdateNew []*endpoint.EndpointSet
	// Records that need to be deleted
	Delete []*endpoint.EndpointSet
}

// Calculate computes the actions needed to move current state towards desired
// state. It then passes those changes to the current policy for further
// processing. It returns a copy of Plan with the changes populated.
func (plan *Plan) Calculate() *Plan {
	changes := &Changes{}
	for key, desired := range plan.RecordTargets {
		if _, exists := plan.CurrentTargets[key]; !exists {
			changes.Create = append(changes.Create, &endpoint.EndpointSet{
				DNSName:    key.DNSName,
				RecordType: key.RecordType,
				Targets:    desired,
			})
		} else if endpoint.TargetSliceEquals(plan.CurrentTargets[key], desired) {
			log.Debugf("Skipping EndpointSet %s -> (%+v) because targets have not changed", key.DNSName, desired)
		} else {
			changes.UpdateOld = append(changes.UpdateOld, &endpoint.EndpointSet{
				DNSName:    key.DNSName,
				RecordType: key.RecordType,
				Targets:    plan.CurrentTargets[key],
				Labels:     plan.Labels[key],
			})

			changes.UpdateNew = append(changes.UpdateNew, &endpoint.EndpointSet{
				DNSName:    key.DNSName,
				RecordType: key.RecordType,
				Targets:    desired,
				Labels:     plan.Labels[key],
			})
		}
	}

	for key, current := range plan.CurrentTargets {
		if _, exists := plan.RecordTargets[key]; !exists {
			changes.Delete = append(changes.Delete, &endpoint.EndpointSet{
				DNSName:    key.DNSName,
				RecordType: key.RecordType,
				Targets:    current,
				Labels:     plan.Labels[key],
			})
		}
	}

	for _, pol := range plan.Policies {
		changes = pol.Apply(changes)
	}

	return &Plan{
		Aliases:        plan.Aliases,
		CurrentTargets: plan.CurrentTargets,
		RecordTargets:  plan.RecordTargets,
		Changes:        changes,
	}
}
