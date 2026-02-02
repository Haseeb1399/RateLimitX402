package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// CPUStats represents CPU utilization information.
type CPUStats struct {
	Utilization float64 `json:"utilization"` // Percentage (0-100)
	Timestamp   string  `json:"timestamp"`
}

// CPUHandler returns an HTTP handler that responds with current CPU utilization.
func CPUHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		utilization, err := getCPUUtilization()
		if err != nil {
			http.Error(w, "Failed to get CPU utilization: "+err.Error(), http.StatusInternalServerError)
			return
		}

		stats := CPUStats{
			Utilization: utilization,
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	}
}

// GinCPUHandler returns a Gin handler for CPU utilization.
func GinCPUHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		utilization, err := getCPUUtilization()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get CPU utilization"})
			return
		}

		c.JSON(http.StatusOK, CPUStats{
			Utilization: utilization,
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
		})
	}
}

// getCPUUtilization reads CPU stats from /proc/stat and calculates utilization.
func getCPUUtilization() (float64, error) {
	idle1, total1, err := readCPUStat()
	if err != nil {
		return 0, err
	}

	time.Sleep(50 * time.Millisecond)

	idle2, total2, err := readCPUStat()
	if err != nil {
		return 0, err
	}

	idleDelta := idle2 - idle1
	totalDelta := total2 - total1

	if totalDelta == 0 {
		return 0, nil
	}

	utilization := (1.0 - float64(idleDelta)/float64(totalDelta)) * 100
	return utilization, nil
}

// readCPUStat reads the first line of /proc/stat and returns idle and total CPU time.
func readCPUStat() (idle, total uint64, err error) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, 0, err
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {
		return 0, 0, nil
	}

	// First line: cpu  user nice system idle iowait irq softirq steal guest guest_nice
	fields := strings.Fields(lines[0])
	if len(fields) < 5 {
		return 0, 0, nil
	}

	for i := 1; i < len(fields); i++ {
		val, _ := strconv.ParseUint(fields[i], 10, 64)
		total += val
		if i == 4 { // idle is the 4th value (0-indexed: 4)
			idle = val
		}
	}

	return idle, total, nil
}
