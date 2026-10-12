// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hostdcloud/terraform-provider-hostd/internal/client"
)

var firewallRuleObjectType = types.ObjectType{AttrTypes: map[string]attr.Type{
	"type":    types.StringType,
	"action":  types.StringType,
	"proto":   types.StringType,
	"source":  types.StringType,
	"dest":    types.StringType,
	"sport":   types.StringType,
	"dport":   types.StringType,
	"comment": types.StringType,
	"enable":  types.BoolType,
}}

type firewallRuleModel struct {
	Type    types.String `tfsdk:"type"`
	Action  types.String `tfsdk:"action"`
	Proto   types.String `tfsdk:"proto"`
	Source  types.String `tfsdk:"source"`
	Dest    types.String `tfsdk:"dest"`
	SPort   types.String `tfsdk:"sport"`
	DPort   types.String `tfsdk:"dport"`
	Comment types.String `tfsdk:"comment"`
	Enable  types.Bool   `tfsdk:"enable"`
}

func emptyFirewallRules() types.List {
	return types.ListValueMust(firewallRuleObjectType, []attr.Value{})
}

func emptyString(description string) schema.StringAttribute {
	return schema.StringAttribute{
		Optional:            true,
		Computed:            true,
		Default:             stringdefault.StaticString(""),
		MarkdownDescription: description,
	}
}

// firewallRulesAttribute is the ordered rule list. withGroups allows type
// `group`, which attaches a security group to a VM.
func firewallRulesAttribute(description string, withGroups bool) schema.ListNestedAttribute {
	ruleTypes := []string{"in", "out"}
	typeDescription := "`in` or `out`."
	actionDescription := "`ACCEPT` or `DROP`."
	protoDescription := "`tcp`, `udp` or `icmp`."
	if withGroups {
		ruleTypes = append(ruleTypes, "group")
		typeDescription = "`in` or `out` for a packet filter, or `group` to attach a security group."
		actionDescription = "`ACCEPT` or `DROP` for a packet filter. For `type = \"group\"`, the security group's `pve_group_name`."
		protoDescription = "`tcp`, `udp` or `icmp` for a packet filter. Leave empty for a security group."
	}
	address := "address, CIDR (IPv4 no larger than `/26`, IPv6 no larger than `/64`), IPv4 range, or an IP set as `+<pve_ipset_name>`. Empty matches any."
	return schema.ListNestedAttribute{
		Optional:            true,
		Computed:            true,
		Default:             listdefault.StaticValue(emptyFirewallRules()),
		MarkdownDescription: description,
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"type": schema.StringAttribute{
					Required:            true,
					MarkdownDescription: typeDescription,
					Validators:          []validator.String{stringvalidator.OneOf(ruleTypes...)},
				},
				"action": schema.StringAttribute{
					Required:            true,
					MarkdownDescription: actionDescription,
				},
				"proto": schema.StringAttribute{
					Optional:            true,
					Computed:            true,
					Default:             stringdefault.StaticString(""),
					MarkdownDescription: protoDescription,
					Validators:          []validator.String{stringvalidator.OneOf("", "tcp", "udp", "icmp")},
				},
				"source":  emptyString("Source " + address),
				"dest":    emptyString("Destination " + address),
				"sport":   emptyString("Source port, range (`80:85`) or comma-separated list. TCP and UDP only."),
				"dport":   emptyString("Destination port, range (`80:85`) or comma-separated list. TCP and UDP only."),
				"comment": emptyStringAtMost("Free-text comment, up to 255 characters.", 255),
				"enable": schema.BoolAttribute{
					Optional:            true,
					Computed:            true,
					Default:             booldefault.StaticBool(true),
					MarkdownDescription: "Whether the rule is enabled. Defaults to true.",
				},
			},
		},
	}
}

func emptyStringAtMost(description string, maxLength int) schema.StringAttribute {
	attribute := emptyString(description)
	attribute.Validators = []validator.String{stringvalidator.LengthAtMost(maxLength)}
	return attribute
}

func firewallRuleModels(ctx context.Context, rules types.List) ([]firewallRuleModel, diag.Diagnostics) {
	if rules.IsNull() || rules.IsUnknown() {
		return nil, nil
	}
	var models []firewallRuleModel
	diags := rules.ElementsAs(ctx, &models, false)
	return models, diags
}

// validateFirewallRules reports rules the API would refuse. Unknown values are skipped.
func validateFirewallRules(ctx context.Context, rules types.List, attribute path.Path) diag.Diagnostics {
	models, diags := firewallRuleModels(ctx, rules)
	for i, rule := range models {
		if msg := rule.invalid(); msg != "" {
			diags.AddAttributeError(attribute.AtListIndex(i), "Invalid firewall rule", msg)
		}
	}
	return diags
}

func (m firewallRuleModel) invalid() string {
	for _, value := range []types.String{m.Type, m.Action, m.Proto, m.SPort, m.DPort, m.Source, m.Dest} {
		if value.IsUnknown() {
			return ""
		}
	}
	if m.Type.ValueString() == "group" {
		if m.Action.ValueString() == "" {
			return "action must be the security group's pve_group_name."
		}
		if m.Proto.ValueString() != "" || m.Source.ValueString() != "" || m.Dest.ValueString() != "" || m.SPort.ValueString() != "" || m.DPort.ValueString() != "" {
			return "A security group attachment uses only action (the group's pve_group_name), comment and enable."
		}
		return ""
	}
	if action := m.Action.ValueString(); action != "ACCEPT" && action != "DROP" {
		return fmt.Sprintf("action must be ACCEPT or DROP, got %q.", action)
	}
	switch m.Proto.ValueString() {
	case "tcp", "udp":
	case "icmp":
		if m.SPort.ValueString() != "" || m.DPort.ValueString() != "" {
			return "Ports are only valid for tcp and udp."
		}
	default:
		return "proto must be tcp, udp or icmp."
	}
	return ""
}

func firewallRuleInputs(ctx context.Context, rules types.List) ([]client.FirewallRuleInput, diag.Diagnostics) {
	models, diags := firewallRuleModels(ctx, rules)
	inputs := make([]client.FirewallRuleInput, 0, len(models))
	for _, rule := range models {
		inputs = append(inputs, client.FirewallRuleInput{
			Type:    rule.Type.ValueString(),
			Action:  rule.Action.ValueString(),
			Proto:   rule.Proto.ValueString(),
			Source:  rule.Source.ValueString(),
			Dest:    rule.Dest.ValueString(),
			SPort:   rule.SPort.ValueString(),
			DPort:   rule.DPort.ValueString(),
			Comment: rule.Comment.ValueString(),
			Enable:  rule.Enable.ValueBool(),
		})
	}
	return inputs, diags
}

// firewallRulesFromAPI builds the rule list from the API, keeping a prior
// value where the API only trimmed whitespace around it.
func firewallRulesFromAPI(ctx context.Context, prior types.List, api []client.FirewallRule) (types.List, diag.Diagnostics) {
	previous, diags := firewallRuleModels(ctx, prior)
	if diags.HasError() {
		return prior, diags
	}
	models := make([]firewallRuleModel, 0, len(api))
	for i, rule := range api {
		var prev firewallRuleModel
		if i < len(previous) {
			prev = previous[i]
		}
		enable := true
		if rule.Enable != nil {
			enable = *rule.Enable
		}
		models = append(models, firewallRuleModel{
			Type:    types.StringValue(rule.Type),
			Action:  keepConfigured(prev.Action, deref(rule.Action)),
			Proto:   keepConfigured(prev.Proto, deref(rule.Proto)),
			Source:  keepConfigured(prev.Source, deref(rule.Source)),
			Dest:    keepConfigured(prev.Dest, deref(rule.Dest)),
			SPort:   keepConfigured(prev.SPort, deref(rule.SPort)),
			DPort:   keepConfigured(prev.DPort, deref(rule.DPort)),
			Comment: keepConfigured(prev.Comment, deref(rule.Comment)),
			Enable:  types.BoolValue(enable),
		})
	}
	return types.ListValueFrom(ctx, firewallRuleObjectType, models)
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
