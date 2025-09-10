package influx

import (
	"context"
	"fmt"

	"go-iot-platform/internal/config"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
)

var (
	URL    = config.Get("INFLUX_URL")
	Token  = config.Get("INFLUX_TOKEN")
	Org    = config.Get("INFLUX_ORG")
	Bucket = config.Get("INFLUX_BUCKET")
)

func GetFieldForDevice(device, field string) (float64, error) {
	client := influxdb2.NewClient(URL, Token)
	defer client.Close()
	q := client.QueryAPI(Org)

	flux := fmt.Sprintf(`
        from(bucket: "%s")
        |> range(start: -5m)
        |> filter(fn: (r) => r._measurement == "devices" and r.device == "%s" and r._field == "%s")
        |> last()
    `, Bucket, device, field)

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
