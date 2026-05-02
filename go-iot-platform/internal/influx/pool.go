// Package influx — WritePool (Faza 2.7): rutează scrierile pe bucket-ul Influx
// corespunzător planului tenantului (free / pro / enterprise).
//
// Retention recomandat per bucket (configurat în InfluxDB la creare):
//   iot-free        → 7 zile
//   iot-pro         → 90 zile
//   iot-enterprise  → 730 zile (2 ani)
package influx

import (
	"context"
	"sync"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	influxdb2api "github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
)

// BucketConfig ține numele bucket-urilor Influx per plan.
// Valorile vin din env vars; dacă lipsesc, default-urile sunt cele de mai jos.
type BucketConfig struct {
	Free       string // INFLUX_BUCKET_FREE,       default "iot-free"
	Pro        string // INFLUX_BUCKET_PRO,        default "iot-pro"
	Enterprise string // INFLUX_BUCKET_ENTERPRISE, default "iot-enterprise"
}

func (c *BucketConfig) applyDefaults() {
	if c.Free == "" {
		c.Free = "iot-free"
	}
	if c.Pro == "" {
		c.Pro = "iot-pro"
	}
	if c.Enterprise == "" {
		c.Enterprise = "iot-enterprise"
	}
}

func (c BucketConfig) ForPlan(plan string) string {
	switch plan {
	case "pro":
		return c.Pro
	case "enterprise":
		return c.Enterprise
	default: // "free" sau necunoscut → free bucket
		return c.Free
	}
}

// WritePool gestionează un WriteAPI per bucket și rutează scrierile.
type WritePool struct {
	apis    map[string]influxdb2api.WriteAPI // bucket → WriteAPI
	buckets BucketConfig
	mu      sync.Mutex
}

// NewWritePool creează un WritePool cu câte un WriteAPI per bucket de plan.
// Erorile asincrone din fiecare WriteAPI sunt trimise pe errCh (buffer 32).
func NewWritePool(
	client influxdb2.Client,
	org string,
	cfg BucketConfig,
	errCh chan<- error,
) *WritePool {
	cfg.applyDefaults()
	apis := map[string]influxdb2api.WriteAPI{}
	for _, bucket := range []string{cfg.Free, cfg.Pro, cfg.Enterprise} {
		if _, ok := apis[bucket]; ok {
			continue // un singur WriteAPI per bucket chiar dacă două planuri pointează la același
		}
		api := client.WriteAPI(org, bucket)
		apis[bucket] = api
		go func(a influxdb2api.WriteAPI) {
			for err := range a.Errors() {
				select {
				case errCh <- err:
				default:
				}
			}
		}(api)
	}
	return &WritePool{apis: apis, buckets: cfg}
}

// APIFor returnează WriteAPI-ul potrivit pentru tenantul dat (după plan).
// plan poate fi "" (necunoscut) → rutează pe bucket-ul free.
func (p *WritePool) APIFor(plan string) influxdb2api.WriteAPI {
	bucket := p.buckets.ForPlan(plan)
	p.mu.Lock()
	api := p.apis[bucket]
	p.mu.Unlock()
	return api
}

// BucketFor returnează numele bucket-ului pentru un plan dat (util la logging).
func (p *WritePool) BucketFor(plan string) string {
	return p.buckets.ForPlan(plan)
}

// Flush forțează flush pe toate WriteAPI-urile active.
func (p *WritePool) Flush(ctx context.Context) {
	_ = ctx
	for _, api := range p.apis {
		api.Flush()
	}
}

// WritePoint scrie un punct pe bucket-ul corespunzător planului.
func (p *WritePool) WritePoint(plan string, pt *write.Point) {
	p.APIFor(plan).WritePoint(pt)
}
