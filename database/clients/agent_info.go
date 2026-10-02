package clients

import "fmt"

// Canonical protocol names only; region and timestamps are computed by the server.
var agentInfoFields = map[string]bool{
	"cpu_name":true, "cpu_cores":true, "cpu_physical_cores":true,
	"arch":true, "os":true, "kernel_version":true, "ipv4":true, "ipv6":true,
	"mem_total":true, "swap_total":true, "disk_total":true, "gpu_name":true,
	"virtualization":true, "version":true,
}

func ValidateAgentInfo(info map[string]interface{}) error {
	for key, value := range info {
		if !agentInfoFields[key] { return fmt.Errorf("unsupported agent field: %s", key) }
		switch key {
		case "cpu_cores", "cpu_physical_cores", "mem_total", "swap_total", "disk_total":
		default:
			s, ok := value.(string)
			if !ok || len(s) > 512 { return fmt.Errorf("invalid agent string: %s", key) }
		}
	}
	return nil
}
