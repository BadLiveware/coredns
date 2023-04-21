package gcp_instances

import (
	"context"
	"fmt"
	"strings"

	gcp "google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
)

type gcpInstances interface {
	listAllInstances(projectID string) ([]Instance, error)
}

type gcpClient struct {
	*gcp.Service
}

type Instance struct {
	Name       string            `json:"name"`
	IP         string            `json:"ip"`
	Zone       string            `json:"zone"`
	Project    string            `json:"project"`
	Network    string            `json:"network"`
	Subnetwork string            `json:"subnetwork"`
	Labels     map[string]string `json:"labels"`
}

func formatInstance(instance gcp.Instance, projectID string) Instance {
	zoneStr := strings.Split(instance.Zone, "/")
	zone := zoneStr[len(zoneStr)-1]

	inst := &Instance{
		Name:       instance.Name,
		IP:         instance.NetworkInterfaces[0].NetworkIP,
		Zone:       zone,
		Project:    projectID,
		Network:    instance.NetworkInterfaces[0].Network,
		Subnetwork: instance.NetworkInterfaces[0].Subnetwork,
		Labels:     instance.Labels,
	}

	return *inst
}

func (c gcpClient) listAllInstances(projectID string) ([]Instance, error) {
	ctx := context.Background()
	instancesService, err := gcp.NewService(ctx, option.WithScopes(gcp.ComputeScope))
	if err != nil {
		return nil, fmt.Errorf("failed to create compute service: %v", err)
	}
	instances := make([]Instance, 0)

	aggregatedListCall := instancesService.Instances.AggregatedList(projectID)
	if err := aggregatedListCall.Pages(ctx, func(page *gcp.InstanceAggregatedList) error {
		for _, instancesScopedList := range page.Items {
			for _, instance := range instancesScopedList.Instances {
				instanceFormatted := formatInstance(*instance, projectID)
				instances = append(instances, instanceFormatted)
			}
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to list instances: %v", err)
	}
	return instances, nil
}
