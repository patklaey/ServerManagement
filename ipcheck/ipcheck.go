package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"ServerManagement/utils"

	"gopkg.in/yaml.v3"
)

var now string

type configuration struct {
	LogFilePath     string                 `yaml:"logFilePath"`
	OldIpFilePath   string                 `yaml:"oldIpFilePath"`
	PagerDutyConfig *utils.PagerdutyConfig `yaml:"pagerduty"`
}

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
	config := loadConfig(*configPath)
	if config == nil {
		log.Fatalf("Failed to load config file")
		os.Exit(1)
	}

	logfile := config.LogFilePath
	oldIPFile := config.OldIpFilePath

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
		options := utils.IncidentOptions{
			BodyDetails: fmt.Sprintf("The public IP of our home changed from %s to %s. Please update the bind configuration and switchplus nameserver in order to have a functional DNS", oldIP, currentIP),
			Title:       "New IP Address",
			Urgency:     utils.UrgencyLow,
		}
		incident, err := utils.CreateIncident(options, config.PagerDutyConfig, logger)
		if err != nil {
			logger.Printf("%s : Failed to create incident: %s\n", now, err)
		} else {
			logger.Printf("%s : Incident successfully created %v\n", now, incident)
		}

		if err = writeIP(oldIPFile, currentIP); err != nil {
			log.Fatalf("Failed to update IP file: %v", err)
		}
	} else {
		logger.Printf("%s : Ipcheck successful, still the same ip: %s\n", now, currentIP)
	}
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

func loadConfig(configFile string) *configuration {
	var config configuration

	yfile, err := os.ReadFile(configFile)
	if err != nil {
		fmt.Println("Could not read config file: ", err)
		return nil
	}

	err = yaml.Unmarshal(yfile, &config)
	if err != nil {
		fmt.Println("Error loading configuration: ", err)
		return nil
	}
	return &config
}
