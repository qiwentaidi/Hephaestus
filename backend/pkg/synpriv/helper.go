package synpriv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"time"

	gonmap "github.com/qiwentaidi/gonmap/lib"
)

const (
	HelperFlag            = "--hephaestus-syn-discovery-helper"
	helperTestFlag        = "--self-test"
	helperStreamFlag      = "--stream"
	helperTimeout         = 5 * time.Second
	helperProtocolVersion = 2
)

type OpenPort struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type DiscoveryRequest struct {
	Hosts       []string `json:"hosts"`
	Ports       []int    `json:"ports"`
	TimeoutMS   int      `json:"timeoutMs"`
	Concurrency int      `json:"concurrency"`
	Rate        int      `json:"rate"`
}

type DiscoveryResponse struct {
	Success bool       `json:"success"`
	Error   string     `json:"error,omitempty"`
	Ports   []OpenPort `json:"ports,omitempty"`
	Version int        `json:"version,omitempty"`
}

type discoveryEvent struct {
	Type  string    `json:"type"`
	Port  *OpenPort `json:"port,omitempty"`
	Error string    `json:"error,omitempty"`
}

type HelperStatus struct {
	Path      string `json:"path"`
	Exists    bool   `json:"exists"`
	Setuid    bool   `json:"setuid"`
	RootOwned bool   `json:"rootOwned"`
	Usable    bool   `json:"usable"`
	Error     string `json:"error,omitempty"`
	Version   int    `json:"version,omitempty"`
}

func MaybeRunHelper() bool {
	if len(os.Args) < 2 || os.Args[1] != HelperFlag {
		return false
	}
	if len(os.Args) >= 3 && os.Args[2] == helperTestFlag {
		writeJSON(DiscoveryResponse{Success: CurrentProcessHasRawSocketCapability(), Version: helperProtocolVersion})
		return true
	}
	if len(os.Args) >= 3 && os.Args[2] == helperStreamFlag {
		if err := runDiscoveryHelperStream(); err != nil {
			writeJSON(discoveryEvent{Type: "error", Error: err.Error()})
		}
		return true
	}
	if err := runDiscoveryHelper(); err != nil {
		writeJSON(DiscoveryResponse{Success: false, Error: err.Error()})
	}
	return true
}

func DenyRegularHelperStartup() bool {
	path, err := os.Executable()
	if err != nil {
		return false
	}
	return sameExecutablePath(path, HelperPath())
}

func CurrentProcessHasRawSocketCapability() bool {
	if runtime.GOOS == "windows" {
		return hasWindowsNpcap()
	}
	if !SupportsRawSocketPlatform(runtime.GOOS) {
		return false
	}
	if os.Geteuid() == 0 {
		return true
	}
	conn, err := net.ListenPacket("ip4:tcp", "0.0.0.0")
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// SupportsRawSocketPlatform reports whether the platform can use the native
// raw-socket SYN discovery path. Android uses Linux networking primitives but
// has its own GOOS value, so it must be included explicitly.
func SupportsRawSocketPlatform(platform string) bool {
	return platform == "darwin" || platform == "linux" || platform == "android"
}

func Status(ctx context.Context) HelperStatus {
	path := HelperPath()
	status := helperFileStatus(path)
	if !status.Exists || !status.Setuid || !status.RootOwned {
		return status
	}

	testCtx, cancel := context.WithTimeout(ctx, helperTimeout)
	defer cancel()
	cmd := exec.CommandContext(testCtx, path, HelperFlag, helperTestFlag)
	output, err := cmd.Output()
	if err != nil {
		status.Error = err.Error()
		return status
	}
	var response DiscoveryResponse
	if err := json.Unmarshal(output, &response); err != nil {
		status.Error = err.Error()
		return status
	}
	status.Version = response.Version
	status.Usable = response.Success && response.Version >= helperProtocolVersion
	if !response.Success {
		status.Error = response.Error
	} else if response.Version < helperProtocolVersion {
		status.Error = "SYN 授权辅助程序版本过旧，请重新授权以更新辅助程序"
	}
	return status
}

// DiscoverOpenPortsWithStatusCallback streams open ports from the privileged helper.
func DiscoverOpenPortsWithStatusCallback(ctx context.Context, status HelperStatus, request DiscoveryRequest, onOpen func(OpenPort)) ([]OpenPort, error) {
	if !status.Usable {
		if status.Error != "" {
			return nil, fmt.Errorf("SYN helper unavailable: %s", status.Error)
		}
		return nil, errors.New("SYN helper unavailable")
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, helperRunTimeout(request))
	defer cancel()
	cmd := exec.CommandContext(timeoutCtx, status.Path, HelperFlag, helperStreamFlag)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, err
	}
	if err := json.NewEncoder(stdin).Encode(request); err != nil {
		_ = stdin.Close()
		_ = cmd.Wait()
		return nil, err
	}
	_ = stdin.Close()

	var ports []OpenPort
	decoder := json.NewDecoder(output)
	completed := false
	for {
		var event discoveryEvent
		if err := decoder.Decode(&event); err != nil {
			if err == io.EOF && completed {
				break
			}
			_ = cmd.Wait()
			if timeoutCtx.Err() != nil {
				return ports, timeoutCtx.Err()
			}
			return ports, fmt.Errorf("decode SYN helper stream: %w", err)
		}
		switch event.Type {
		case "port":
			if event.Port == nil {
				return ports, errors.New("SYN helper returned an empty port event")
			}
			ports = append(ports, *event.Port)
			if onOpen != nil {
				onOpen(*event.Port)
			}
		case "done":
			completed = true
		case "error":
			_ = cmd.Wait()
			return ports, errors.New(event.Error)
		default:
			_ = cmd.Wait()
			return ports, fmt.Errorf("SYN helper returned unknown event %q", event.Type)
		}
	}
	if err := cmd.Wait(); err != nil {
		if timeoutCtx.Err() != nil {
			return ports, timeoutCtx.Err()
		}
		return ports, err
	}
	return ports, nil
}

func helperRunTimeout(request DiscoveryRequest) time.Duration {
	rate := request.Rate
	if rate <= 0 {
		rate = 1500
	}
	probes := int64(len(request.Hosts)) * int64(len(request.Ports))
	seconds := (probes + int64(rate) - 1) / int64(rate)
	return helperTimeout + requestTimeout(request) + time.Duration(seconds*2)*time.Second
}

func DiscoverOpenPorts(ctx context.Context, request DiscoveryRequest) ([]OpenPort, error) {
	return DiscoverOpenPortsWithStatus(ctx, Status(ctx), request)
}

// DiscoverOpenPortsWithStatus reuses a helper status checked for this scan task.
// The helper command itself still fails if the executable becomes unavailable.
func DiscoverOpenPortsWithStatus(ctx context.Context, status HelperStatus, request DiscoveryRequest) ([]OpenPort, error) {
	if !status.Usable {
		if status.Error != "" {
			return nil, fmt.Errorf("SYN helper unavailable: %s", status.Error)
		}
		return nil, errors.New("SYN helper unavailable")
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, helperRunTimeout(request))
	defer cancel()
	cmd := exec.CommandContext(timeoutCtx, status.Path, HelperFlag)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, err
	}
	if err := json.NewEncoder(stdin).Encode(request); err != nil {
		_ = stdin.Close()
		_ = cmd.Wait()
		return nil, err
	}
	_ = stdin.Close()

	var response DiscoveryResponse
	decodeErr := json.NewDecoder(output).Decode(&response)
	waitErr := cmd.Wait()
	if decodeErr != nil {
		return nil, decodeErr
	}
	if waitErr != nil {
		return nil, waitErr
	}
	if !response.Success {
		return nil, errors.New(response.Error)
	}
	sort.Slice(response.Ports, func(i, j int) bool {
		if response.Ports[i].Host != response.Ports[j].Host {
			return response.Ports[i].Host < response.Ports[j].Host
		}
		return response.Ports[i].Port < response.Ports[j].Port
	})
	return response.Ports, nil
}

func requestTimeout(request DiscoveryRequest) time.Duration {
	timeout := time.Duration(request.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = time.Second
	}
	return timeout
}

func runDiscoveryHelper() error {
	if !CurrentProcessHasRawSocketCapability() {
		return errors.New("helper does not have raw socket privileges")
	}

	var request DiscoveryRequest
	if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
		return err
	}
	if len(request.Hosts) == 0 || len(request.Ports) == 0 {
		writeJSON(DiscoveryResponse{Success: true})
		return nil
	}

	timeout := requestTimeout(request)
	openPorts := gonmap.New(nil).DiscoverOpenPorts(request.Hosts, request.Ports, &gonmap.PortDiscoveryOptions{
		Timeout:     timeout,
		Concurrency: request.Concurrency,
		Mode:        gonmap.PortDiscoveryModeSYN,
		Rate:        request.Rate,
	})
	results := make([]OpenPort, 0, len(openPorts))
	for _, item := range openPorts {
		results = append(results, OpenPort{Host: item.Host, Port: item.Port})
	}
	writeJSON(DiscoveryResponse{Success: true, Ports: results})
	return nil
}

func runDiscoveryHelperStream() error {
	if !CurrentProcessHasRawSocketCapability() {
		return errors.New("helper does not have raw socket privileges")
	}
	var request DiscoveryRequest
	if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
		return err
	}
	encoder := json.NewEncoder(os.Stdout)
	if len(request.Hosts) > 0 && len(request.Ports) > 0 {
		gonmap.New(nil).DiscoverOpenPortsWithContextCallback(context.Background(), request.Hosts, request.Ports, &gonmap.PortDiscoveryOptions{
			Timeout: requestTimeout(request), Concurrency: request.Concurrency,
			Mode: gonmap.PortDiscoveryModeSYN, Rate: request.Rate,
		}, func(port gonmap.OpenPort) {
			item := OpenPort{Host: port.Host, Port: port.Port}
			_ = encoder.Encode(discoveryEvent{Type: "port", Port: &item})
		})
	}
	return encoder.Encode(discoveryEvent{Type: "done"})
}

func writeJSON(value any) {
	_ = json.NewEncoder(os.Stdout).Encode(value)
}
