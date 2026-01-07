package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	pagerduty "github.com/PagerDuty/go-pagerduty"
	"gopkg.in/ini.v1"
)

const PagerDutySection = "Pagerduty"

var now string

func main() {
	// ----- CLI flags -----
	configPath := flag.String("config", "", "Path to configuration file")
	flag.StringVar(configPath, "c", "", "Path to configuration file (shorthand)")
	help := flag.Bool("help", false, "Show help")
	flag.BoolVar(help, "h", false, "Show help (shorthand)")
	flag.Parse()

	if *help {
		flag.Usage()
		os.Exit(0)
	}

	if *configPath == "" {
		log.Fatal("No configuration file passed! Use -c or --config")
	}

	if _, err := os.Stat(*configPath); err != nil {
		log.Fatalf("Configuration file %s is not readable: %v", *configPath, err)
	}

	// ----- Load INI config -----
	cfg, err := ini.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config file: %v", err)
	}

	logfile := cfg.Section("General").Key("Logfile").String()
	oldIPFile := cfg.Section("General").Key("OldIpFile").String()

	if logfile == "" || oldIPFile == "" {
		log.Fatal("Missing required configuration values")
	}

	// ----- Open logfile -----
	logFH, err := os.OpenFile(logfile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Cannot open logfile %s: %v", logfile, err)
	}
	defer logFH.Close()

	logger := log.New(logFH, "", 0)

	now = time.Now().Format("2006-01-02T15:04:05")

	// ----- Resolve current IP -----
	currentIP, err := resolveIP("patklaey.internet-box.ch")
	if err != nil {
		logger.Printf("%s : IP could not be retrieved. Error: %v\n", now, err)
		os.Exit(0)
	}

	// ----- Read old IP -----
	oldIP, err := readOldIP(oldIPFile)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatalf("Failed to read old IP file: %v", err)
	}

	if oldIP == "" {
		// First run: write IP
		if err := writeIP(oldIPFile, currentIP); err != nil {
			log.Fatalf("Failed to write IP file: %v", err)
		}
		return
	}

	if oldIP != currentIP {
		createIncident(oldIP, currentIP, cfg, logger)

		if err := writeIP(oldIPFile, currentIP); err != nil {
			log.Fatalf("Failed to update IP file: %v", err)
		}
	} else {
		logger.Printf("%s : Ipcheck successful, still the same ip: %s\n", now, currentIP)
	}
}

func createIncident(oldIp string, newIp string, cfg *ini.File, logger *log.Logger) error {
	authtoken := cfg.Section(PagerDutySection).Key("AuthToken").String()
	serviceId := cfg.Section(PagerDutySection).Key("ServiceId").String()

	serviceRef := pagerduty.APIReference{
		ID:   serviceId,
		Type: "service_reference",
	}

	incidentBody := pagerduty.APIDetails{
		Type:    "incident_body",
		Details: fmt.Sprintf("The public IP of our home changed from %s to %s. Please update the bind configuration and switchplus nameserver in order to have a functional DNS", oldIp, newIp),
	}

	ctx := context.Background()
	client := pagerduty.NewClient(authtoken)
	options := pagerduty.CreateIncidentOptions{
		Title:   "New IP Address",
		Service: &serviceRef,
		Urgency: "low",
		Body:    &incidentBody,
	}
	from := cfg.Section(PagerDutySection).Key("From").String()
	incident, err := client.CreateIncidentWithContext(ctx, from, &options)
	if err != nil {
		return err
	}
	logger.Printf("%s : Incident successfully created %v\n", now, incident)
	return nil

}

// resolveIP resolves the first IPv4 address for a hostname
func resolveIP(host string) (string, error) {
	ips, err := net.LookupIP(host)
	if err != nil {
		return "", err
	}

	for _, ip := range ips {
		if ipv4 := ip.To4(); ipv4 != nil {
			return ipv4.String(), nil
		}
	}

	return "", errors.New("no IPv4 address found")
}

func readOldIP(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text()), nil
	}

	return "", nil
}

func writeIP(path, ip string) error {
	return os.WriteFile(path, []byte(ip), 0644)
}
