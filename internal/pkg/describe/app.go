// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package describe

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/aws/copilot-cli/internal/pkg/aws/cloudformation"
	"github.com/aws/copilot-cli/internal/pkg/aws/codepipeline"
	"github.com/aws/copilot-cli/internal/pkg/aws/sessions"
	"github.com/aws/copilot-cli/internal/pkg/config"
	"github.com/aws/copilot-cli/internal/pkg/deploy"
	"github.com/aws/copilot-cli/internal/pkg/deploy/cloudformation/stack"
	"github.com/aws/copilot-cli/internal/pkg/term/color"
	"golang.org/x/mod/semver"
	"gopkg.in/yaml.v3"
)

// App contains serialized parameters for an application.
type App struct {
	Name      string                   `json:"name"`
	URI       string                   `json:"uri"`
	Envs      []*config.Environment    `json:"environments"`
	Services  []*config.Workload       `json:"services"`
	Pipelines []*codepipeline.Pipeline `json:"pipelines"`
}

// JSONString returns the stringified App struct with json format.
func (a *App) JSONString() (string, error) {
	b, err := json.Marshal(a)
	if err != nil {
		return "", fmt.Errorf("marshal application description: %w", err)
	}
	return fmt.Sprintf("%s\n", b), nil
}

// HumanString returns the stringified App struct with human readable format.
func (a *App) HumanString() string {
	var b bytes.Buffer
	writer := tabwriter.NewWriter(&b, minCellWidth, tabWidth, cellPaddingWidth, paddingChar, noAdditionalFormatting)
	fmt.Fprint(writer, color.Bold.Sprint("About\n\n"))
	writer.Flush()
	fmt.Fprintf(writer, "  %s\t%s\n", "Name", a.Name)
	fmt.Fprintf(writer, "  %s\t%s\n", "URI", a.URI)
	fmt.Fprint(writer, color.Bold.Sprint("\nEnvironments\n\n"))
	writer.Flush()
	headers := []string{"Name", "AccountID", "Region"}
	fmt.Fprintf(writer, "  %s\n", strings.Join(headers, "\t"))
	fmt.Fprintf(writer, "  %s\n", strings.Join(underline(headers), "\t"))
	for _, env := range a.Envs {
		fmt.Fprintf(writer, "  %s\t%s\t%s\n", env.Name, env.AccountID, env.Region)
	}
	fmt.Fprint(writer, color.Bold.Sprint("\nServices\n\n"))
	writer.Flush()
	headers = []string{"Name", "Type"}
	fmt.Fprintf(writer, "  %s\n", strings.Join(headers, "\t"))
	fmt.Fprintf(writer, "  %s\n", strings.Join(underline(headers), "\t"))
	for _, svc := range a.Services {
		fmt.Fprintf(writer, "  %s\t%s\n", svc.Name, svc.Type)
	}
	fmt.Fprint(writer, color.Bold.Sprint("\nPipelines\n\n"))
	writer.Flush()
	headers = []string{"Name"}
	fmt.Fprintf(writer, "  %s\n", strings.Join(headers, "\t"))
	fmt.Fprintf(writer, "  %s\n", strings.Join(underline(headers), "\t"))
	for _, pipeline := range a.Pipelines {
		fmt.Fprintf(writer, "  %s\n", pipeline.Name)
	}
	writer.Flush()
	return b.String()
}

// AppDescriber retrieves information about an application.
type AppDescriber struct {
	app string
	cfn cfn
}

// NewAppDescriber instantiates an application describer.
func NewAppDescriber(appName string) (*AppDescriber, error) {
	sess, err := sessions.NewProvider().Default()
	if err != nil {
		return nil, fmt.Errorf("assume default role for app %s: %w", appName, err)
	}
	return &AppDescriber{
		app: appName,
		cfn: cloudformation.New(sess),
	}, nil
}

// Version returns the app CloudFormation template version associated with
// the application by reading the Metadata.Version field from the template.
// Specifically it will get both app CFN stack template version and app StackSet template version,
// and return the minimum as the current app version.
//
// If the Version field does not exist, then it's a legacy template and it returns an deploy.LegacyAppTemplateVersion and nil error.
func (d *AppDescriber) Version() (string, error) {
	type metadata struct {
		TemplateVersion string `yaml:"TemplateVersion"`
	}
	stackMetadata, stackSetMetadata := metadata{}, metadata{}

	appStackName := stack.NameForAppStack(d.app)
	appStackMetadata, err := d.cfn.Metadata(cloudformation.MetadataWithStackName(appStackName))
	if err != nil {
		return "", fmt.Errorf("get metadata for app stack %s: %w", appStackName, err)
	}
	if err := yaml.Unmarshal([]byte(appStackMetadata), &stackMetadata); err != nil {
		return "", fmt.Errorf("unmarshal Metadata property for app stack %s: %w", appStackName, err)
	}
	appStackVersion := stackMetadata.TemplateVersion
	if appStackVersion == "" {
		appStackVersion = deploy.LegacyAppTemplateVersion
	}

	appStackSetName := stack.NameForAppStackSet(d.app)
	appStackSetMetadata, err := d.cfn.Metadata(cloudformation.MetadataWithStackSetName(appStackSetName))
	if err != nil {
		return "", fmt.Errorf("get metadata for app stack set %s: %w", appStackSetName, err)
	}
	if err := yaml.Unmarshal([]byte(appStackSetMetadata), &stackSetMetadata); err != nil {
		return "", fmt.Errorf("unmarshal Metadata property for app stack set %s: %w", appStackSetName, err)
	}
	appStackSetVersion := stackSetMetadata.TemplateVersion
	if appStackSetVersion == "" {
		appStackSetVersion = deploy.LegacyAppTemplateVersion
	}

	minVersion := appStackVersion
	if semver.Compare(appStackVersion, appStackSetVersion) > 0 {
		minVersion = appStackSetVersion
	}
	return minVersion, nil
}
