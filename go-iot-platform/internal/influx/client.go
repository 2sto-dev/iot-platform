package influx

import (
    "context"
    "fmt"

    influxdb2 "github.com/influxdata/influxdb-client-go/v2"
)

var (
    URL    = "http://db-flux.airweb.ro:8086"
    Token  = "teXchv0yTR4y4lCrrUDam_mo2H8l-OlZM4D7gRVAE80ZEeloRT1kYTjPXFoHsbRmp107O96-4kgNEk1YAzTH3A=="
    Org    = "xCore"
    Bucket = "v2ap14shellyem"
)

func GetCurrentForDevice(device string) (float64, error) {
    // Pentru compatibilitate veche (current)
    return GetFieldForDevice(device, "Current")
}

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
