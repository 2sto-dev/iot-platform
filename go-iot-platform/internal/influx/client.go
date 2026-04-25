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

func GetFieldForDevice(device, field, rangeStr string) (float64, error) {
	if rangeStr == "" {
		rangeStr = DefaultRange
	}
	if !rangeRe.MatchString(rangeStr) {
		return 0, fmt.Errorf("invalid range %q (expected like -5m, -1h, -2d)", rangeStr)
	}

	client := influxdb2.NewClient(URL, Token)
	defer client.Close()
	q := client.QueryAPI(Org)

	flux := fmt.Sprintf(`
        from(bucket: "%s")
        |> range(start: %s)
        |> filter(fn: (r) => r._measurement == "devices" and r.device == "%s" and r._field == "%s")
        |> last()
    `, Bucket, rangeStr, device, field)

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
