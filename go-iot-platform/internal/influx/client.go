package influx

import (
	"context"
	"fmt"
	"regexp"

	"go-iot-platform/internal/config"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
)

var (
	URL    = config.Get("INFLUX_URL")
	Token  = config.Get("INFLUX_TOKEN")
	Org    = config.Get("INFLUX_ORG")
	Bucket = config.Get("INFLUX_BUCKET") // legacy single-bucket fallback
)

const DefaultRange = "-5m"

var rangeRe = regexp.MustCompile(`^-?\d+[smhd]$`)

// bucketsToTry returnează lista de bucket-uri în care căutăm datele, în ordine de prioritate.
// Plan corect primul, apoi celelalte (date mutate la upgrade/downgrade), apoi legacy.
func bucketsToTry(plan string) []string {
	cfg := BucketConfig{
		Free:       config.Get("INFLUX_BUCKET_FREE"),
		Pro:        config.Get("INFLUX_BUCKET_PRO"),
		Enterprise: config.Get("INFLUX_BUCKET_ENTERPRISE"),
	}
	cfg.applyDefaults()
	primary := cfg.ForPlan(plan)
	seen := map[string]bool{primary: true}
	out := []string{primary}
	for _, b := range []string{cfg.Enterprise, cfg.Pro, cfg.Free, Bucket} {
		if b != "" && !seen[b] {
			seen[b] = true
			out = append(out, b)
		}
	}
	return out
}

func GetFieldForDevice(device, field, rangeStr string, tenantID int64, plan string) (float64, error) {
	if rangeStr == "" {
		rangeStr = DefaultRange
	}
	if !rangeRe.MatchString(rangeStr) {
		return 0, fmt.Errorf("invalid range %q (expected like -5m, -1h, -2d)", rangeStr)
	}

	client := influxdb2.NewClient(URL, Token)
	defer client.Close()
	q := client.QueryAPI(Org)

	// Filtrul pe tenant_id se aplică strict: doar date cu tenant_id corect.
	// Legacy data fără tenant_id e RESPINSĂ (multi-tenant isolation).
	tenantFilter := ""
	if tenantID > 0 {
		tenantFilter = fmt.Sprintf(` and r.tenant_id == "%d"`, tenantID)
	}

	for _, bucket := range bucketsToTry(plan) {
		flux := fmt.Sprintf(`
            from(bucket: "%s")
            |> range(start: %s)
            |> filter(fn: (r) => r._measurement == "devices" and r.device == "%s" and r._field == "%s"%s)
            |> last()
        `, bucket, rangeStr, device, field, tenantFilter)

		result, err := q.Query(context.Background(), flux)
		if err != nil {
			continue
		}
		for result.Next() {
			switch v := result.Record().Value().(type) {
			case float64:
				return v, nil
			case int64:
				return float64(v), nil
			case uint64:
				return float64(v), nil
			case bool:
				if v {
					return 1, nil
				}
				return 0, nil
			}
		}
	}
	return 0, fmt.Errorf("no data")
}
