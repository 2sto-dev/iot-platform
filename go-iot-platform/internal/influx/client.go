package influx

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"go-iot-platform/internal/config"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
)

var (
	URL    = config.Get("INFLUX_URL")
	Token  = config.Get("INFLUX_TOKEN")
	Org    = config.Get("INFLUX_ORG")
	Bucket = config.Get("INFLUX_BUCKET")
)

const DefaultRange = "-5m"

var rangeRe = regexp.MustCompile(`^-?\d+[smhd]$`)

// getQueryBuckets returns the list of Influx buckets to query for reads.
// Faza 2.7: writes may be routed to per-plan buckets; for reads we query across
// all configured buckets (default + per-plan) and take the latest point.
func getQueryBuckets() []string {
	bset := map[string]struct{}{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s != "" {
			bset[s] = struct{}{}
		}
	}
	add(Bucket)
	add(os.Getenv("INFLUX_BUCKET_FREE"))
	add(os.Getenv("INFLUX_BUCKET_PRO"))
	add(os.Getenv("INFLUX_BUCKET_ENTERPRISE"))
	out := make([]string, 0, len(bset))
	for b := range bset {
		out = append(out, b)
	}
	return out
}

func GetFieldForDevice(device, field, rangeStr string, tenantID int64) (float64, error) {
	if rangeStr == "" {
		rangeStr = DefaultRange
	}
	if !rangeRe.MatchString(rangeStr) {
		return 0, fmt.Errorf("invalid range %q (expected like -5m, -1h, -2d)", rangeStr)
	}

	client := influxdb2.NewClient(URL, Token)
	defer client.Close()
	q := client.QueryAPI(Org)

	// Filtrul pe tenant_id se aplică doar dacă tenantID > 0. Punctele scrise înainte de
	// Faza 1.8 nu au tag-ul tenant_id; le acceptăm pentru tranziție (legacy data).
	tenantFilter := ""
	if tenantID > 0 {
		tenantFilter = fmt.Sprintf(` and (r.tenant_id == "%d" or not exists r.tenant_id)`, tenantID)
	}

	buckets := getQueryBuckets()
	var flux string
	if len(buckets) == 1 {
		flux = fmt.Sprintf(`
        from(bucket: "%s")
        |> range(start: %s)
        |> filter(fn: (r) => r._measurement == "devices" and r.device == "%s" and r._field == "%s"%s)
        |> last()
    `, buckets[0], rangeStr, device, field, tenantFilter)
	} else {
		tables := make([]string, 0, len(buckets))
		for _, b := range buckets {
			tables = append(tables, fmt.Sprintf(`from(bucket: "%s") |> range(start: %s)`, b, rangeStr))
		}
		flux = fmt.Sprintf(`
        union(tables: [%s])
        |> filter(fn: (r) => r._measurement == "devices" and r.device == "%s" and r._field == "%s"%s)
        |> last()
    `, strings.Join(tables, ", "), device, field, tenantFilter)
	}

	result, err := q.Query(context.Background(), flux)
	if err != nil {
		return 0, err
	}
	for result.Next() {
		if v, ok := result.Record().Value().(float64); ok {
			return v, nil
		}
	}
	return 0, fmt.Errorf("no data")
}
