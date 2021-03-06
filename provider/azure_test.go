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

package provider

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/arm/dns"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/to"

	"github.com/kubernetes-incubator/external-dns/endpoint"
	"github.com/kubernetes-incubator/external-dns/plan"
)

type mockZonesClient struct {
	mockZoneListResult *dns.ZoneListResult
}

type mockRecordsClient struct {
	mockRecordSet    *[]dns.RecordSet
	deletedEndpoints []*endpoint.Endpoint
	updatedEndpoints []*endpoint.Endpoint
}

func createMockZone(zone string) dns.Zone {
	return dns.Zone{
		Name: to.StringPtr(zone),
	}
}

func (client *mockZonesClient) ListByResourceGroup(resourceGroupName string, top *int32) (dns.ZoneListResult, error) {
	// Don't bother filtering by resouce group or implementing paging since that's the responsibility
	// of the Azure DNS service
	return *client.mockZoneListResult, nil
}

func (client *mockZonesClient) ListByResourceGroupNextResults(lastResults dns.ZoneListResult) (dns.ZoneListResult, error) {
	return dns.ZoneListResult{}, nil
}

func aRecordSetPropertiesGetter(value string) *dns.RecordSetProperties {
	return &dns.RecordSetProperties{
		ARecords: &[]dns.ARecord{
			{
				Ipv4Address: to.StringPtr(value),
			},
		},
	}
}

func cNameRecordSetPropertiesGetter(value string) *dns.RecordSetProperties {
	return &dns.RecordSetProperties{
		CnameRecord: &dns.CnameRecord{
			Cname: to.StringPtr(value),
		},
	}
}

func txtRecordSetPropertiesGetter(value string) *dns.RecordSetProperties {
	return &dns.RecordSetProperties{
		TxtRecords: &[]dns.TxtRecord{
			{
				Value: &[]string{value},
			},
		},
	}
}

func othersRecordSetPropertiesGetter(value string) *dns.RecordSetProperties {
	return &dns.RecordSetProperties{}
}

func createMockRecordSet(name, recordType, value string) dns.RecordSet {
	var getterFunc func(value string) *dns.RecordSetProperties

	switch recordType {
	case "A":
		getterFunc = aRecordSetPropertiesGetter
	case "CNAME":
		getterFunc = cNameRecordSetPropertiesGetter
	case "TXT":
		getterFunc = txtRecordSetPropertiesGetter
	default:
		getterFunc = othersRecordSetPropertiesGetter
	}
	return dns.RecordSet{
		Name:                to.StringPtr(name),
		Type:                to.StringPtr("Microsoft.Network/dnszones/" + recordType),
		RecordSetProperties: getterFunc(value),
	}

}

func (client *mockRecordsClient) ListByDNSZone(resourceGroupName string, zoneName string, top *int32) (dns.RecordSetListResult, error) {
	return dns.RecordSetListResult{Value: client.mockRecordSet}, nil
}

func (client *mockRecordsClient) ListByDNSZoneNextResults(list dns.RecordSetListResult) (dns.RecordSetListResult, error) {
	return dns.RecordSetListResult{}, nil
}

func (client *mockRecordsClient) Delete(resourceGroupName string, zoneName string, relativeRecordSetName string, recordType dns.RecordType, ifMatch string) (autorest.Response, error) {
	client.deletedEndpoints = append(
		client.deletedEndpoints,
		endpoint.NewEndpoint(
			formatAzureDNSName(relativeRecordSetName, zoneName),
			"",
			string(recordType),
		),
	)
	return autorest.Response{}, nil
}

func (client *mockRecordsClient) CreateOrUpdate(resourceGroupName string, zoneName string, relativeRecordSetName string, recordType dns.RecordType, parameters dns.RecordSet, ifMatch string, ifNoneMatch string) (dns.RecordSet, error) {
	client.updatedEndpoints = append(
		client.updatedEndpoints,
		endpoint.NewEndpoint(
			formatAzureDNSName(relativeRecordSetName, zoneName),
			extractAzureTarget(&parameters),
			string(recordType),
		),
	)
	return parameters, nil
}

func newAzureProvider(domainFilter string, dryRun bool, resourceGroup string, zonesClient ZonesClient, recordsClient RecordsClient) *AzureProvider {
	return &AzureProvider{
		domainFilter:  domainFilter,
		dryRun:        dryRun,
		resourceGroup: resourceGroup,
		zonesClient:   zonesClient,
		recordsClient: recordsClient,
	}
}

func TestAzureRecord(t *testing.T) {
	zonesClient := mockZonesClient{
		mockZoneListResult: &dns.ZoneListResult{
			Value: &[]dns.Zone{
				createMockZone("example.com"),
			},
		},
	}

	recordsClient := mockRecordsClient{
		mockRecordSet: &[]dns.RecordSet{
			createMockRecordSet("@", "NS", "ns1-03.azure-dns.com."),
			createMockRecordSet("@", "SOA", "Email: azuredns-hostmaster.microsoft.com"),
			createMockRecordSet("@", "A", "123.123.123.122"),
			createMockRecordSet("@", "TXT", "heritage=external-dns,external-dns/owner=default"),
			createMockRecordSet("nginx", "A", "123.123.123.123"),
			createMockRecordSet("nginx", "TXT", "heritage=external-dns,external-dns/owner=default"),
			createMockRecordSet("hack", "CNAME", "hack.azurewebsites.net"),
		},
	}

	provider := newAzureProvider("example.com", true, "k8s", &zonesClient, &recordsClient)
	actual, err := provider.Records()

	if err != nil {
		t.Fatal(err)
	}
	expected := []*endpoint.Endpoint{
		endpoint.NewEndpoint("example.com", "123.123.123.122", "A"),
		endpoint.NewEndpoint("example.com", "heritage=external-dns,external-dns/owner=default", "TXT"),
		endpoint.NewEndpoint("nginx.example.com", "123.123.123.123", "A"),
		endpoint.NewEndpoint("nginx.example.com", "heritage=external-dns,external-dns/owner=default", "TXT"),
		endpoint.NewEndpoint("hack.example.com", "hack.azurewebsites.net", "CNAME"),
	}

	validateEndpoints(t, actual, expected)
}

func TestAzureApplyChanges(t *testing.T) {
	recordsClient := mockRecordsClient{}

	testAzureApplyChangesInternal(t, false, &recordsClient)

	validateEndpoints(t, recordsClient.deletedEndpoints, []*endpoint.Endpoint{
		endpoint.NewEndpoint("old.example.com", "", "A"),
		endpoint.NewEndpoint("oldcname.example.com", "", "CNAME"),
		endpoint.NewEndpoint("deleted.example.com", "", "A"),
		endpoint.NewEndpoint("deletedcname.example.com", "", "CNAME"),
	})

	validateEndpoints(t, recordsClient.updatedEndpoints, []*endpoint.Endpoint{
		endpoint.NewEndpoint("example.com", "1.2.3.4", "A"),
		endpoint.NewEndpoint("example.com", "tag", "TXT"),
		endpoint.NewEndpoint("foo.example.com", "1.2.3.4", "A"),
		endpoint.NewEndpoint("foo.example.com", "tag", "TXT"),
		endpoint.NewEndpoint("bar.example.com", "other.com", "CNAME"),
		endpoint.NewEndpoint("bar.example.com", "tag", "TXT"),
		endpoint.NewEndpoint("other.com", "5.6.7.8", "A"),
		endpoint.NewEndpoint("other.com", "tag", "TXT"),
		endpoint.NewEndpoint("new.example.com", "111.222.111.222", "A"),
		endpoint.NewEndpoint("newcname.example.com", "other.com", "CNAME"),
	})
}

func TestAzureApplyChangesDryRun(t *testing.T) {
	recordsClient := mockRecordsClient{}

	testAzureApplyChangesInternal(t, true, &recordsClient)

	validateEndpoints(t, recordsClient.deletedEndpoints, []*endpoint.Endpoint{})

	validateEndpoints(t, recordsClient.updatedEndpoints, []*endpoint.Endpoint{})
}

func testAzureApplyChangesInternal(t *testing.T, dryRun bool, client RecordsClient) {
	provider := newAzureProvider(
		"",
		dryRun,
		"group",
		&mockZonesClient{
			mockZoneListResult: &dns.ZoneListResult{
				Value: &[]dns.Zone{
					createMockZone("example.com"),
					createMockZone("other.com"),
				},
			},
		},
		client,
	)

	createRecords := []*endpoint.Endpoint{
		endpoint.NewEndpoint("example.com", "1.2.3.4", "A"),
		endpoint.NewEndpoint("example.com", "tag", "TXT"),
		endpoint.NewEndpoint("foo.example.com", "1.2.3.4", ""),
		endpoint.NewEndpoint("foo.example.com", "tag", "TXT"),
		endpoint.NewEndpoint("bar.example.com", "other.com", ""),
		endpoint.NewEndpoint("bar.example.com", "tag", "TXT"),
		endpoint.NewEndpoint("other.com", "5.6.7.8", "A"),
		endpoint.NewEndpoint("other.com", "tag", "TXT"),
		endpoint.NewEndpoint("nope.com", "4.4.4.4", ""),
		endpoint.NewEndpoint("nope.com", "tag", "TXT"),
	}

	currentRecords := []*endpoint.Endpoint{
		endpoint.NewEndpoint("old.example.com", "121.212.121.212", "A"),
		endpoint.NewEndpoint("oldcname.example.com", "other.com", ""),
		endpoint.NewEndpoint("old.nope.com", "121.212.121.212", ""),
	}
	updatedRecords := []*endpoint.Endpoint{
		endpoint.NewEndpoint("new.example.com", "111.222.111.222", ""),
		endpoint.NewEndpoint("newcname.example.com", "other.com", ""),
		endpoint.NewEndpoint("new.nope.com", "222.111.222.111", "A"),
	}

	deleteRecords := []*endpoint.Endpoint{
		endpoint.NewEndpoint("deleted.example.com", "111.222.111.222", ""),
		endpoint.NewEndpoint("deletedcname.example.com", "other.com", ""),
		endpoint.NewEndpoint("deleted.nope.com", "222.111.222.111", "A"),
	}

	changes := &plan.Changes{
		Create:    createRecords,
		UpdateNew: updatedRecords,
		UpdateOld: currentRecords,
		Delete:    deleteRecords,
	}

	if err := provider.ApplyChanges(changes); err != nil {
		t.Fatal(err)
	}
}
