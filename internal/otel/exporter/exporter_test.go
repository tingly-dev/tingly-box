package exporter

import (
	"context"
	"testing"
	"time"
)

func TestNewOTLPExporter_GRPCDefault(t *testing.T) {
	// The gRPC exporter connects lazily, so construction succeeds offline.
	exp, err := NewOTLPExporter(OTLPConfig{
		Endpoint: "localhost:4317",
		Insecure: true,
	})
	if err != nil {
		t.Fatalf("NewOTLPExporter(grpc) failed: %v", err)
	}
	if exp == nil {
		t.Fatal("exporter should not be nil")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = exp.Shutdown(ctx)
}

func TestNewOTLPExporter_HTTP(t *testing.T) {
	exp, err := NewOTLPExporter(OTLPConfig{
		Endpoint: "localhost:4318",
		Protocol: "http/protobuf",
		Insecure: true,
		Headers:  map[string]string{"x-api-key": "test"},
	})
	if err != nil {
		t.Fatalf("NewOTLPExporter(http/protobuf) failed: %v", err)
	}
	if exp == nil {
		t.Fatal("exporter should not be nil")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = exp.Shutdown(ctx)
}

func TestNewOTLPExporter_UnsupportedProtocol(t *testing.T) {
	exp, err := NewOTLPExporter(OTLPConfig{
		Endpoint: "localhost:4317",
		Protocol: "carrier-pigeon",
	})
	if err == nil {
		t.Fatal("expected error for unsupported protocol")
	}
	if exp != nil {
		t.Fatal("exporter should be nil on error")
	}
}
