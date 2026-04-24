package portmap

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
)

const commentPrefix = "PPANEL_HOP"

// HopPortRange describes a single hop-port DNAT mapping rule.
type HopPortRange struct {
	StartPort   int
	EndPort     int
	ServicePort int
	Comment     string // iptables comment tag for precise cleanup
}

// ParseHopPorts parses a hop_ports string like "30001-60000" into start and end ports.
// Returns (0, 0, nil) if the string is empty.
func ParseHopPorts(hopPorts string) (start, end int, err error) {
	hopPorts = strings.TrimSpace(hopPorts)
	if hopPorts == "" {
		return 0, 0, nil
	}
	parts := strings.SplitN(hopPorts, "-", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid hop_ports format: %q (expected START-END)", hopPorts)
	}
	start, err = strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid hop_ports start port: %v", err)
	}
	end, err = strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid hop_ports end port: %v", err)
	}
	if start < 1 || end < 1 || start > 65535 || end > 65535 {
		return 0, 0, fmt.Errorf("hop_ports out of range: %d-%d", start, end)
	}
	if start > end {
		return 0, 0, fmt.Errorf("hop_ports start (%d) > end (%d)", start, end)
	}
	return start, end, nil
}

// generateComment creates a unique iptables comment for a hop rule.
func generateComment(servicePort, startPort, endPort int) string {
	return fmt.Sprintf("%s_%d_%d_%d", commentPrefix, servicePort, startPort, endPort)
}

// ApplyHopPorts creates iptables and ip6tables DNAT rules for the given hop_ports range.
// Returns a HopPortRange record for later cleanup, or nil if hop_ports is empty.
func ApplyHopPorts(servicePort int, hopPorts string) (*HopPortRange, error) {
	start, end, err := ParseHopPorts(hopPorts)
	if err != nil {
		return nil, err
	}
	if start == 0 && end == 0 {
		return nil, nil // empty hop_ports, nothing to do
	}

	comment := generateComment(servicePort, start, end)
	portRange := fmt.Sprintf("%d:%d", start, end)

	// IPv4: iptables
	if err := runIptables("iptables", portRange, servicePort, comment); err != nil {
		return nil, fmt.Errorf("apply IPv4 hop rule failed: %v", err)
	}
	log.Infof("[PortMap] IPv4 DNAT 已添加: UDP %d-%d -> %d", start, end, servicePort)

	// IPv6: ip6tables (best-effort, log warning on failure)
	// Try to load kernel modules first
	_ = exec.Command("modprobe", "ip6_tables").Run()
	_ = exec.Command("modprobe", "ip6table_nat").Run()
	if err := runIptables("ip6tables", portRange, servicePort, comment); err != nil {
		log.Warnf("[PortMap] IPv6 DNAT 添加失败 (可能不支持): %v", err)
	} else {
		log.Infof("[PortMap] IPv6 DNAT 已添加: UDP %d-%d -> %d", start, end, servicePort)
	}

	return &HopPortRange{
		StartPort:   start,
		EndPort:     end,
		ServicePort: servicePort,
		Comment:     comment,
	}, nil
}

// runIptables executes an iptables/ip6tables DNAT rule addition.
func runIptables(cmd string, portRange string, servicePort int, comment string) error {
	args := []string{
		"-t", "nat", "-A", "PREROUTING",
		"-p", "udp",
		"--dport", portRange,
		"-j", "DNAT",
		"--to-destination", fmt.Sprintf(":%d", servicePort),
		"-m", "comment", "--comment", comment,
	}
	out, err := exec.Command(cmd, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %s", cmd, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// RemoveHopPorts removes a single hop-port DNAT rule from both iptables and ip6tables.
func RemoveHopPorts(rule *HopPortRange) error {
	if rule == nil {
		return nil
	}
	removeByComment("iptables", rule.Comment)
	removeByComment("ip6tables", rule.Comment)
	log.Infof("[PortMap] DNAT 已清理: UDP %d-%d -> %d", rule.StartPort, rule.EndPort, rule.ServicePort)
	return nil
}

// RemoveAllHopPorts removes all hop-port DNAT rules in the list.
func RemoveAllHopPorts(rules []*HopPortRange) {
	for _, r := range rules {
		_ = RemoveHopPorts(r)
	}
}

// removeByComment finds and deletes all PREROUTING NAT rules matching the given comment.
// Uses -S to list rules, then -D to delete the exact rule spec.
func removeByComment(cmd string, comment string) {
	// List all rules in PREROUTING chain as rule specs
	out, err := exec.Command(cmd, "-t", "nat", "-S", "PREROUTING").CombinedOutput()
	if err != nil {
		return
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, comment) {
			continue
		}
		// line looks like: -A PREROUTING -p udp --dport 30001:60000 -j DNAT ...
		// Replace -A with -D to build the delete command
		if strings.HasPrefix(line, "-A ") {
			deleteSpec := "-D " + line[3:]
			args := strings.Fields(deleteSpec)
			delCmd := exec.Command(cmd, append([]string{"-t", "nat"}, args...)...)
			if delOut, delErr := delCmd.CombinedOutput(); delErr != nil {
				log.Warnf("[PortMap] 删除规则失败 (%s): %s", cmd, strings.TrimSpace(string(delOut)))
			}
		}
	}
}

// CheckPortRangeConflict checks if a new port range [newStart, newEnd] conflicts
// with any existing range in the list. Returns a descriptive error if conflict found.
func CheckPortRangeConflict(ranges []PortRangeRecord, newStart, newEnd int, newHost, newLabel string) error {
	for _, r := range ranges {
		if newStart <= r.End && newEnd >= r.Start {
			return fmt.Errorf("端口范围冲突: %s [%d-%d] (%s) 与 %s [%d-%d] (%s)",
				newLabel, newStart, newEnd, newHost,
				r.Label, r.Start, r.End, r.Host)
		}
	}
	return nil
}

// PortRangeRecord records a registered port or port range for conflict detection.
type PortRangeRecord struct {
	Start int
	End   int
	Host  string
	Label string // e.g. "port" or "hop_ports"
}
