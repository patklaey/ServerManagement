package utils

import (
	"context"
	"fmt"
	"log"

	pagerduty "github.com/PagerDuty/go-pagerduty"
)

type PagerdutyConfig struct {
	AuthToken string `yaml:"authToken"`
	ServiceId string `yaml:"serviceId"`
	From      string `yaml:"from"`
}

type IncidentOptions struct {
	BodyDetails string
	Title       string
	Urgency     string
}

const (
	UrgencyLow  = "low"
	UrgencyHigh = "high"
)

func CreateIncident(options IncidentOptions, config *PagerdutyConfig, logger *log.Logger) (*pagerduty.Incident, error) {
	if config == nil {
		return nil, fmt.Errorf("Failed to load configuration file")
	}

	serviceRef := pagerduty.APIReference{
		ID:   config.ServiceId,
		Type: "service_reference",
	}

	incidentBody := pagerduty.APIDetails{
		Type:    "incident_body",
		Details: options.BodyDetails,
	}

	ctx := context.Background()
	client := pagerduty.NewClient(config.AuthToken)
	pagerDutyOptions := pagerduty.CreateIncidentOptions{
		Title:   "New IP Address",
		Service: &serviceRef,
		Urgency: "low",
		Body:    &incidentBody,
	}
	incident, err := client.CreateIncidentWithContext(ctx, config.From, &pagerDutyOptions)
	if err != nil {
		return nil, err
	}
	return incident, nil
}
