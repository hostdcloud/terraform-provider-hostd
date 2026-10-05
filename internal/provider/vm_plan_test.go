// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestDowngradeBlocked(t *testing.T) {
	running := vmResourceModel{
		Cores:      types.Int64Value(4),
		RAMGB:      types.Int64Value(8),
		PowerState: types.StringValue("running"),
	}
	if !downgradeBlocked(vmResourceModel{Cores: types.Int64Value(2), RAMGB: running.RAMGB, PowerState: running.PowerState}, running) {
		t.Fatal("fewer cores on a running VM should be blocked")
	}
	if !downgradeBlocked(vmResourceModel{Cores: running.Cores, RAMGB: types.Int64Value(4), PowerState: running.PowerState}, running) {
		t.Fatal("less RAM on a running VM should be blocked")
	}
	stopped := vmResourceModel{Cores: types.Int64Value(2), RAMGB: running.RAMGB, PowerState: types.StringValue("stopped")}
	if downgradeBlocked(stopped, running) {
		t.Fatal("a downgrade with power_state stopped should be allowed")
	}
	if downgradeBlocked(running, running) {
		t.Fatal("an unchanged VM should not be blocked")
	}
}

func TestLANError(t *testing.T) {
	ctx := context.Background()
	list := func(lans []lanInterfaceModel) types.List {
		t.Helper()
		value, diags := types.ListValueFrom(ctx, types.ObjectType{AttrTypes: lanInterfaceAttrTypes}, lans)
		if diags.HasError() {
			t.Fatal(diags)
		}
		return value
	}
	lan := func(vnet, mode, ip string, gateway bool) lanInterfaceModel {
		addr := types.StringNull()
		if ip != "" {
			addr = types.StringValue(ip)
		}
		return lanInterfaceModel{
			VNet:      types.StringValue(vnet),
			Mode:      types.StringValue(mode),
			IP:        addr,
			IsGateway: types.BoolValue(gateway),
		}
	}
	missing := vmResourceModel{
		WANMode:       types.StringValue("off"),
		LANInterfaces: list([]lanInterfaceModel{lan("a", "select", "", false), lan("b", "auto", "", true)}),
	}
	if missing.lanError(ctx) == "" {
		t.Fatal("select without ip should be rejected")
	}
	two := vmResourceModel{
		WANMode:       types.StringValue("off"),
		LANInterfaces: list([]lanInterfaceModel{lan("a", "auto", "", false), lan("b", "auto", "", false)}),
	}
	if two.lanError(ctx) == "" {
		t.Fatal("two LANs without a gateway should be rejected")
	}
	ok := vmResourceModel{
		WANMode:       types.StringValue("off"),
		LANInterfaces: list([]lanInterfaceModel{lan("a", "select", "10.0.0.10", true), lan("b", "auto", "", false)}),
	}
	if ok.lanError(ctx) != "" {
		t.Fatal(ok.lanError(ctx))
	}
}

func TestWANModeChangeError(t *testing.T) {
	mode := func(value string) types.String { return types.StringValue(value) }
	if wanModeChangeError(mode("ipv4"), mode("dual")) == "" {
		t.Fatal("ipv4 to dual should be rejected")
	}
	if wanModeChangeError(mode("dual"), mode("ipv6")) == "" {
		t.Fatal("dual to ipv6 should be rejected")
	}
	if wanModeChangeError(mode("ipv4"), mode("off")) != "" {
		t.Fatal("ipv4 to off is allowed")
	}
	if wanModeChangeError(mode("off"), mode("dual")) != "" {
		t.Fatal("off to dual is allowed")
	}
}

func TestPlanAddressesKeepsThemWhenWANStays(t *testing.T) {
	state := &vmResourceModel{
		WANMode: types.StringValue("dual"),
		IPv4:    types.StringValue("203.0.113.10"),
		IPv6:    types.StringValue("2001:db8::1"),
	}
	plan := vmResourceModel{WANMode: types.StringValue("dual"), IPv4: types.StringUnknown(), IPv6: types.StringUnknown()}
	planAddresses(&plan, state)
	if plan.IPv4.ValueString() != "203.0.113.10" || plan.IPv6.ValueString() != "2001:db8::1" {
		t.Fatalf("addresses changed: %s %s", plan.IPv4, plan.IPv6)
	}

	off := vmResourceModel{WANMode: types.StringValue("off"), IPv4: types.StringUnknown(), IPv6: types.StringUnknown()}
	planAddresses(&off, state)
	if !off.IPv4.IsNull() || !off.IPv6.IsNull() {
		t.Fatalf("off should clear addresses: %s %s", off.IPv4, off.IPv6)
	}

	created := vmResourceModel{WANMode: types.StringValue("ipv4"), IPv4: types.StringUnknown(), IPv6: types.StringUnknown()}
	planAddresses(&created, nil)
	if !created.IPv4.IsUnknown() || !created.IPv6.IsNull() {
		t.Fatalf("create ipv4: %s %s", created.IPv4, created.IPv6)
	}
}
