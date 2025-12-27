package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	serviceName = "windows_exporter"
	serverPort  = ":5000"
)

func main() {
	http.HandleFunc("/start", handleStart)
	http.HandleFunc("/stop", handleStop)
	http.HandleFunc("/status", handleStatus)

	fmt.Printf("Service Controller listening on %s\n", serverPort)
	fmt.Println("Endpoints: /start, /stop, /status")
	
	if err := http.ListenAndServe(serverPort, nil); err != nil {
		log.Fatal(err)
	}
}

func handleStart(w http.ResponseWriter, r *http.Request) {
	if err := startService(serviceName); err != nil {
		http.Error(w, fmt.Sprintf("Failed to start service: %v", err), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "Service '%s' started successfully.\n", serviceName)
}

func handleStop(w http.ResponseWriter, r *http.Request) {
	if err := stopService(serviceName); err != nil {
		http.Error(w, fmt.Sprintf("Failed to stop service: %v", err), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "Service '%s' stopped successfully.\n", serviceName)
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	status, err := getServiceStatus(serviceName)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get status: %v", err), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "Service '%s' state: %s\n", serviceName, status)
}

// --- Service Logic ---

func startService(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("could not access service: %v", err)
	}
	defer s.Close()

	err = s.Start()
	if err != nil {
		return fmt.Errorf("could not start service: %v", err)
	}
	return nil
}

func stopService(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("could not access service: %v", err)
	}
	defer s.Close()

	status, err := s.Control(svc.Stop)
	if err != nil {
		return fmt.Errorf("could not send stop control: %v", err)
	}

	// Optional: Wait for the service to actually stop
	timeout := time.Now().Add(10 * time.Second)
	for status.State != svc.Stopped {
		if time.Now().After(timeout) {
			return fmt.Errorf("timeout waiting for service to stop")
		}
		time.Sleep(300 * time.Millisecond)
		status, err = s.Query()
		if err != nil {
			return fmt.Errorf("could not retrieve service status: %v", err)
		}
	}
	return nil
}

func getServiceStatus(name string) (string, error) {
	m, err := mgr.Connect()
	if err != nil {
		return "", err
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err != nil {
		return "", fmt.Errorf("could not access service: %v", err)
	}
	defer s.Close()

	status, err := s.Query()
	if err != nil {
		return "", err
	}

	switch status.State {
	case svc.Stopped:
		return "Stopped", nil
	case svc.StartPending:
		return "Start Pending", nil
	case svc.StopPending:
		return "Stop Pending", nil
	case svc.Running:
		return "Running", nil
	default:
		return "Unknown", nil
	}
}