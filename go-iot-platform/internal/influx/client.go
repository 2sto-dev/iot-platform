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
	Bucket = config.Get("INFLUX_BUCKET")
)

const DefaultRange = "-5m"

var rangeRe = regexp.MustCompile(`^-?\d+[smhd]$`)

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

	flux := fmt.Sprintf(`
        from(bucket: "%s")
        |> range(start: %s)
        |> filter(fn: (r) => r._measurement == "devices" and r.device == "%s" and r._field == "%s"%s)
        |> last()
    `, Bucket, rangeStr, device, field, tenantFilter)

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
