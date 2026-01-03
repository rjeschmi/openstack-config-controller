package controllers

import (
	"os"
	"testing"
	"time"
)

func TestComputeRequeueAfter_NoEnv_DefaultNonZero(t *testing.T) {
	os.Unsetenv("OPENSTACK_RECONCILE_INTERVAL")
	d, err := ComputeRequeueAfter(2 * time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d != 2*time.Minute {
		t.Fatalf("expected 2m, got %v", d)
	}
}

func TestComputeRequeueAfter_NoEnv_DefaultZero(t *testing.T) {
	os.Unsetenv("OPENSTACK_RECONCILE_INTERVAL")
	d, err := ComputeRequeueAfter(0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d != 5*time.Minute {
		t.Fatalf("expected 5m, got %v", d)
	}
}

func TestComputeRequeueAfter_EnvValid(t *testing.T) {
	os.Setenv("OPENSTACK_RECONCILE_INTERVAL", "30s")
	defer os.Unsetenv("OPENSTACK_RECONCILE_INTERVAL")
	d, err := ComputeRequeueAfter(0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d != 30*time.Second {
		t.Fatalf("expected 30s, got %v", d)
	}
}

func TestComputeRequeueAfter_EnvInvalid(t *testing.T) {
	os.Setenv("OPENSTACK_RECONCILE_INTERVAL", "notaduration")
	defer os.Unsetenv("OPENSTACK_RECONCILE_INTERVAL")
	d, err := ComputeRequeueAfter(10 * time.Second)
	if err == nil {
		t.Fatalf("expected error for invalid duration")
	}
	if d != 10*time.Second {
		t.Fatalf("expected fallback to default 10s, got %v", d)
	}
}
