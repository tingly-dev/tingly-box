package otel

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/obs/exporter"
)

// MeterSetup holds the meter provider and token tracker.
type MeterSetup struct {
	meterProvider *sdkmetric.MeterProvider
	tracker       *TokenTracker
}

// StoreRefs holds references to the storage backends for exporters.
type StoreRefs struct {
	StatsStore *db.StatsStore
	UsageStore *db.UsageStore
	Sink       *obs.Sink
}

// NewMeterSetup creates a new meter setup with the provided config and stores.
func NewMeterSetup(ctx context.Context, cfg *Config, stores *StoreRefs) (*MeterSetup, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	// Build exporter pipeline
	var exporters []sdkmetric.Exporter

	if cfg.SQLiteEnabled {
		sqliteExporter := exporter.NewSQLiteExporter(stores.StatsStore, stores.UsageStore)
		exporters = append(exporters, sqliteExporter)
	}

	if cfg.SinkEnabled && stores.Sink != nil {
		sinkExporter := exporter.NewSinkExporter(stores.Sink)
		exporters = append(exporters, sinkExporter)
	}

	// If no exporters, return early
	if len(exporters) == 0 {
		return &MeterSetup{
			meterProvider: nil,
			tracker:       nil,
		}, nil
	}

	// Create meter provider with periodic export
	reader := sdkmetric.NewPeriodicReader(
		exporter.NewMultiExporter(exporters...),
		sdkmetric.WithInterval(cfg.ExportInterval),
		sdkmetric.WithTimeout(cfg.ExportTimeout),
	)

	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
	)

	otel.SetMeterProvider(meterProvider)
	meter := meterProvider.Meter("tingly-box")

	// Create token tracker
	tracker, err := NewTokenTracker(meter)
	if err != nil {
		// Shutdown meter provider on error
		_ = meterProvider.Shutdown(ctx)
		return nil, fmt.Errorf("failed to create token tracker: %w", err)
	}

	return &MeterSetup{
		meterProvider: meterProvider,
		tracker:       tracker,
	}, nil
}

// Tracker returns the token tracker.
func (ms *MeterSetup) Tracker() *TokenTracker {
	return ms.tracker
}

// Shutdown shuts down the meter provider.
func (ms *MeterSetup) Shutdown(ctx context.Context) error {
	if ms.meterProvider == nil {
		return nil
	}
	return ms.meterProvider.Shutdown(ctx)
}
