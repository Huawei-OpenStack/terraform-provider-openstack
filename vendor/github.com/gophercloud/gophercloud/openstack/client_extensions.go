package openstack

import (
	"github.com/gophercloud/gophercloud"
	"strings"
)

//NewVpcServiceV1 creates the a ServiceClient that may be used to access the v1
//vpc service which is a service of public ip management of huawei cloud
func NewVpcServiceV1(client *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) (*gophercloud.ServiceClient, error) {
	//TODO use real vpc endpoint instead of hacking here.
	sc, err := initClientOpts(client, eo, "compute")
	endpoint := sc.Endpoint
	endpoint = strings.Replace(endpoint, "ecs", "vpc", 1)
	endpoint = strings.Replace(endpoint, "v2", "v1", 1)
	sc.Endpoint = endpoint

	return sc, err
}
