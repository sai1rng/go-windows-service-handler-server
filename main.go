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
	serviceName     = "go-windows-service-handler"
	exporterSvcName = "windows_exporter"
	serverPort      = ":5000"
)

func main() {
	// 1. Determine if we are running as a Service or a Console
	isService, err := svc.IsWindowsService()
	if err != nil {
		log.Fatalf("failed to determine if we are running in an interactive session: %v", err)
	}

	if isService {
		runService(serviceName)
		return
	}

	// 2. If running in Console (testing manually), just start the server
	fmt.Println("Running in Console mode (Press Ctrl+C to stop)")
	startServer()
}

// --- Windows Service Wrapper Logic ---

type myService struct{}

func runService(name string) {
	err := svc.Run(name, &myService{})
	if err != nil {
		log.Fatalf("%s service failed: %v", name, err)
	}
}

// Execute is called by Windows Service Manager
func (m *myService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown

	// Tell Windows we are "Running"
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	// Start the HTTP Server in a separate Goroutine
	go startServer()

	// Wait for a Stop signal from Windows
loop:
	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				break loop
			default:
				log.Printf("unexpected control request #%d", c)
			}
		}
	}

	changes <- svc.Status{State: svc.StopPending}
	return
}

// --- The Actual Web Server Logic ---

func startServer() {
	http.HandleFunc("/start", handleStart)
	http.HandleFunc("/stop", handleStop)
	http.HandleFunc("/status", handleStatus)

	log.Printf("Service Handler listening on %s", serverPort)
	if err := http.ListenAndServe(serverPort, nil); err != nil {
		log.Fatal(err)
	}
}

// --- Endpoint Handlers ---

func handleStart(w http.ResponseWriter, r *http.Request) {
	if err := startWindowsService(exporterSvcName); err != nil {
		http.Error(w, fmt.Sprintf("Failed to start service: %v", err), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "Service '%s' started successfully.\n", exporterSvcName)
}

func handleStop(w http.ResponseWriter, r *http.Request) {
	if err := stopWindowsService(exporterSvcName); err != nil {
		http.Error(w, fmt.Sprintf("Failed to stop service: %v", err), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "Service '%s' stopped successfully.\n", exporterSvcName)
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	status, err := getServiceStatus(exporterSvcName)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get status: %v", err), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "Service '%s' state: %s\n", exporterSvcName, status)
}

// --- Helper Functions ---

func startWindowsService(name string) error {
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

	// FIXED: Start takes variadic arguments, not a slice
	err = s.Start()
	if err != nil {
		return fmt.Errorf("could not start service: %v", err)
	}

	return waitForState(s, svc.Running)
}

func stopWindowsService(name string) error {
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

	// FIXED: Used _ to ignore the status variable
	_, err = s.Control(svc.Stop)
	if err != nil {
		return fmt.Errorf("could not send stop control: %v", err)
	}

	return waitForState(s, svc.Stopped)
}

func waitForState(s *mgr.Service, desired svc.State) error {
	timeout := time.Now().Add(10 * time.Second)
	for {
		status, err := s.Query()
		if err != nil {
			return err
		}
		if status.State == desired {
			return nil
		}
		if time.Now().After(timeout) {
			return fmt.Errorf("timeout waiting for service state change")
		}
		time.Sleep(300 * time.Millisecond)
	}
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