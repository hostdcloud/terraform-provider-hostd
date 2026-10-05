// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hostdcloud/terraform-provider-hostd/internal/client"
)

const defaultAPIURL = "https://api.hostd.cloud"

var _ provider.Provider = &hostdProvider{}

// New returns the provider factory consumed by main.go.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &hostdProvider{version: version}
	}
}

type hostdProvider struct {
	version string
}

type hostdProviderModel struct {
	APIURL types.String `tfsdk:"api_url"`
	APIKey types.String `tfsdk:"api_key"`
}

func (p *hostdProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "hostd"
	resp.Version = p.version
}

func (p *hostdProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The hostd provider manages [hostd.cloud](https://hostd.cloud) Public Cloud — projects, SSH keys and virtual machines — through the public API.",
		Attributes: map[string]schema.Attribute{
			// Authentication attributes are Optional (never Required) so
			// environment variables can supply them; secrets are Sensitive.
			"api_url": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "API base URL. Defaults to `" + defaultAPIURL + "`. May also be set via the `HOSTD_API_URL` environment variable.",
			},
			"api_key": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "API key created in the portal under Settings → API Keys. May also be set via the `HOSTD_API_KEY` environment variable.",
			},
		},
	}
}

func (p *hostdProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config hostdProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Values wired to other resources' outputs are unknown during planning;
	// treating them as empty would silently mis-authenticate.
	if config.APIURL.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_url"),
			"Unknown API URL",
			"api_url depends on a value known only after apply. Set a static value or use the HOSTD_API_URL environment variable.",
		)
	}
	if config.APIKey.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_key"),
			"Unknown API key",
			"api_key depends on a value known only after apply. Set a static value or use the HOSTD_API_KEY environment variable.",
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	// Explicit configuration wins; environment variables are the fallback.
	apiURL := config.APIURL.ValueString()
	if apiURL == "" {
		apiURL = os.Getenv("HOSTD_API_URL")
	}
	if apiURL == "" {
		apiURL = defaultAPIURL
	}
	apiKey := config.APIKey.ValueString()
	if apiKey == "" {
		apiKey = os.Getenv("HOSTD_API_KEY")
	}

	if apiKey == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_key"),
			"Missing API key",
			"Set api_key in the provider block or export HOSTD_API_KEY.",
		)
		return
	}

	c := client.New(apiURL, apiKey, p.version)
	resp.ResourceData = c
	resp.DataSourceData = c
}

func (p *hostdProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewProjectResource,
		NewSSHKeyResource,
		NewVMResource,
	}
}

func (p *hostdProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewOSImagesDataSource,
	}
}
