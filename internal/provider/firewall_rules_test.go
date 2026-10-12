// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hostdcloud/terraform-provider-hostd/internal/client"
)

func rule(ruleType, action, proto, dport string) firewallRuleModel {
	return firewallRuleModel{
		Type:    types.StringValue(ruleType),
		Action:  types.StringValue(action),
		Proto:   types.StringValue(proto),
		Source:  types.StringValue(""),
		Dest:    types.StringValue(""),
		SPort:   types.StringValue(""),
		DPort:   types.StringValue(dport),
		Comment: types.StringValue(""),
		Enable:  types.BoolValue(true),
	}
}

func TestFirewallRuleInvalid(t *testing.T) {
	valid := []firewallRuleModel{
		rule("in", "ACCEPT", "tcp", "22"),
		rule("out", "DROP", "icmp", ""),
		rule("group", "web_a3", "", ""),
	}
	for _, r := range valid {
		if msg := r.invalid(); msg != "" {
			t.Errorf("%s %s %s: %s", r.Type, r.Action, r.Proto, msg)
		}
	}
	invalid := []firewallRuleModel{
		rule("in", "web_a3", "tcp", "22"),
		rule("in", "ACCEPT", "", "22"),
		rule("in", "ACCEPT", "icmp", "22"),
		rule("group", "web_a3", "tcp", ""),
		rule("group", "", "", ""),
	}
	for _, r := range invalid {
		if r.invalid() == "" {
			t.Errorf("%s %s %s %s should be refused", r.Type, r.Action, r.Proto, r.DPort)
		}
	}
	unknown := rule("in", "ACCEPT", "", "22")
	unknown.Proto = types.StringUnknown()
	if msg := unknown.invalid(); msg != "" {
		t.Errorf("unknown values must skip the check, got %q", msg)
	}
}

func TestFirewallRulesFromAPI(t *testing.T) {
	ctx := context.Background()
	configured := rule("in", "ACCEPT", "tcp", "22")
	configured.Comment = types.StringValue("ssh\n")
	prior, diags := types.ListValueFrom(ctx, firewallRuleObjectType, []firewallRuleModel{configured})
	if diags.HasError() {
		t.Fatal(diags)
	}
	s := func(v string) *string { return &v }
	disabled := false
	api := []client.FirewallRule{
		{Pos: 0, Type: "in", Action: s("ACCEPT"), Proto: s("tcp"), DPort: s("22"), Comment: s("ssh")},
		{Pos: 1, Type: "group", Action: s("web_a3"), Enable: &disabled},
	}
	got, diags := firewallRulesFromAPI(ctx, prior, api)
	if diags.HasError() {
		t.Fatal(diags)
	}
	models, diags := firewallRuleModels(ctx, got)
	if diags.HasError() {
		t.Fatal(diags)
	}
	if len(models) != 2 {
		t.Fatalf("got %d rules, want 2", len(models))
	}
	if models[0].Comment.ValueString() != "ssh\n" || !models[0].Enable.ValueBool() {
		t.Errorf("first rule: comment %q enable %v; the configured comment and default enable must stay", models[0].Comment.ValueString(), models[0].Enable.ValueBool())
	}
	if models[1].Proto.ValueString() != "" || models[1].Source.ValueString() != "" || models[1].Enable.ValueBool() {
		t.Errorf("group rule: %+v", models[1])
	}
}
