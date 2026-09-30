package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

// gatherSample reports whether `name` has a sample carrying the given label values.
func gatherSample(t *testing.T, name string, labels map[string]string) bool {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	assert.NoError(t, err)
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		for _, m := range f.GetMetric() {
			match := true
			for _, lp := range m.GetLabel() {
				if want, ok := labels[lp.GetName()]; ok && want != lp.GetValue() {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
	}
	return false
}

func TestRecordCloudRequestAndScanDuration(t *testing.T) {
	restore := conf.SnapshotConfig()
	defer restore()

	// Disabled: no-op, no panic, no samples.
	conf.Server.Prometheus.Enabled = false
	RecordCloudRequest("gw-test", "/api/fs/list", 200, true, 12*time.Millisecond)
	assert.False(t, gatherSample(t, "cloud_request_count", map[string]string{"gateway": "gw-test"}))

	// Enabled: both the success and the failure shape land in the registry.
	conf.Server.Prometheus.Enabled = true
	RecordCloudRequest("gw-test", "/api/fs/list", 200, true, 12*time.Millisecond)
	RecordCloudRequest("gw-test", "/api/fs/get", 0, false, 3*time.Millisecond) // transport error

	assert.True(t, gatherSample(t, "cloud_request_count", map[string]string{
		"gateway": "gw-test", "op": "/api/fs/list", "status": "200", "ok": "true",
	}))
	assert.True(t, gatherSample(t, "cloud_request_count", map[string]string{
		"gateway": "gw-test", "op": "/api/fs/get", "status": "transport_error", "ok": "false",
	}))
	assert.True(t, gatherSample(t, "cloud_request_latency", map[string]string{
		"gateway": "gw-test", "op": "/api/fs/list",
	}))

	// Scan duration (interface method).
	(&metrics{}).RecordScanDuration(context.Background(), 5*time.Second, true)
	assert.True(t, gatherSample(t, "media_scan_duration", map[string]string{"success": "true"}))
}

func TestRecordScanDurationNoop(t *testing.T) {
	// The noop implementation must accept the new method without panicking.
	var n noopMetrics
	assert.NotPanics(t, func() {
		n.RecordScanDuration(context.Background(), time.Second, true)
	})
}
