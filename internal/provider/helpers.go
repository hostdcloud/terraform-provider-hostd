// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// keepConfigured returns the configured value when the API only trimmed the
// whitespace around it (e.g. file() or a heredoc ending in a newline), so Read
// does not report a diff; otherwise it returns the API value.
func keepConfigured(configured types.String, api string) types.String {
	if !configured.IsNull() && strings.TrimSpace(configured.ValueString()) == api {
		return configured
	}
	return types.StringValue(api)
}
