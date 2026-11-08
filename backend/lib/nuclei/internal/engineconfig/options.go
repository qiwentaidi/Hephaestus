// Package engineconfig shares scan configuration and dynamic authentication
// between the embedded SDK and CLI without importing the CLI runner.
package engineconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/gologger"
	"github.com/projectdiscovery/nuclei/v3/pkg/catalog/config"
	"github.com/projectdiscovery/nuclei/v3/pkg/types"
	fileutil "github.com/projectdiscovery/utils/file"
	"github.com/projectdiscovery/utils/generic"
)

var validateOptions = validator.New()

// validateOptions validates the configuration options passed
func ValidateOptions(options *types.Options) error {
	if options.Uncover || len(options.UncoverQuery) > 0 {
		return errors.New("Nuclei Uncover target discovery is not supported; provide resolved targets")
	}
	if options.EnableCloudUpload || options.ScanID != "" || options.ScanUploadFile != "" || options.ScanName != "" || options.TeamID != "" {
		return errors.New("ProjectDiscovery cloud result upload is not supported by the embedded engine")
	}
	if options.AITemplatePrompt != "" {
		return errors.New("ProjectDiscovery cloud template generation is not supported by the embedded engine")
	}
	if err := validateOptions.Struct(options); err != nil {
		if _, ok := err.(*validator.InvalidValidationError); ok {
			return err
		}
		errs := []string{}
		for _, err := range err.(validator.ValidationErrors) {
			errs = append(errs, err.Namespace()+": "+err.Tag())
		}
		return errors.Wrap(errors.New(strings.Join(errs, ", ")), "validation failed for these fields")
	}
	if options.Verbose && options.Silent {
		return errors.New("both verbose and silent mode specified")
	}

	if (options.HeadlessOptionalArguments != nil || options.ShowBrowser || options.UseInstalledChrome) && !options.Headless {
		return errors.New("headless mode (-headless) is required if -ho, -sb, -sc or -lha are set")
	}

	if options.FollowHostRedirects && options.FollowRedirects {
		return errors.New("both follow host redirects and follow redirects specified")
	}
	if options.ShouldFollowHTTPRedirects() && options.DisableRedirects {
		return errors.New("both follow redirects and disable redirects specified")
	}
	// loading the proxy server list from file or cli and test the connectivity
	if err := loadProxyServers(options); err != nil {
		return err
	}
	if options.Validate {
		validateTemplatePaths(options.Logger, config.DefaultConfig.TemplatesDirectory, options.Templates, options.Workflows)
	}
	if options.DAST {
		if err := validateDASTOptions(options); err != nil {
			return err
		}
	}

	// Verify if any of the client certificate options were set since it requires all three to work properly
	if options.HasClientCertificates() {
		if generic.EqualsAny("", options.ClientCertFile, options.ClientKeyFile, options.ClientCAFile) {
			return errors.New("if a client certification option is provided, then all three must be provided")
		}
		validateCertificatePaths(options.Logger, options.ClientCertFile, options.ClientKeyFile, options.ClientCAFile)
	}
	// Verify AWS secrets are passed if a S3 template bucket is passed
	if options.AwsBucketName != "" && options.UpdateTemplates && !options.AwsTemplateDisableDownload {
		missing := validateMissingS3Options(options)
		if missing != nil {
			return fmt.Errorf("aws s3 bucket details are missing. Please provide %s", strings.Join(missing, ","))
		}
	}

	// Verify Azure connection configuration is passed if the Azure template bucket is passed
	if options.AzureContainerName != "" && options.UpdateTemplates && !options.AzureTemplateDisableDownload {
		missing := validateMissingAzureOptions(options)
		if missing != nil {
			return fmt.Errorf("azure connection details are missing. Please provide %s", strings.Join(missing, ","))
		}
	}

	// Verify that all GitLab options are provided if the GitLab server or token is provided
	if len(options.GitLabTemplateRepositoryIDs) != 0 && options.UpdateTemplates && !options.GitLabTemplateDisableDownload {
		missing := validateMissingGitLabOptions(options)
		if missing != nil {
			return fmt.Errorf("gitlab server details are missing. Please provide %s", strings.Join(missing, ","))
		}
	}

	// verify that a valid ip version type was selected (4, 6)
	if len(options.IPVersion) == 0 {
		// add ipv4 as default
		options.IPVersion = append(options.IPVersion, "4")
	}
	var useIPV4, useIPV6 bool
	for _, ipv := range options.IPVersion {
		switch ipv {
		case "4":
			useIPV4 = true
		case "6":
			useIPV6 = true
		default:
			return fmt.Errorf("unsupported ip version: %s", ipv)
		}
	}
	if !useIPV4 && !useIPV6 {
		return errors.New("ipv4 and/or ipv6 must be selected")
	}
	return nil
}

func validateMissingS3Options(options *types.Options) []string {
	var missing []string
	if options.AwsBucketName == "" {
		missing = append(missing, "AWS_TEMPLATE_BUCKET")
	}
	if options.AwsProfile == "" {
		var missingCreds []string
		if options.AwsAccessKey == "" {
			missingCreds = append(missingCreds, "AWS_ACCESS_KEY")
		}
		if options.AwsSecretKey == "" {
			missingCreds = append(missingCreds, "AWS_SECRET_KEY")
		}
		if options.AwsRegion == "" {
			missingCreds = append(missingCreds, "AWS_REGION")
		}

		missing = append(missing, missingCreds...)

		if len(missingCreds) > 0 {
			missing = append(missing, "AWS_PROFILE")
		}
	}

	return missing
}

func validateMissingAzureOptions(options *types.Options) []string {
	var missing []string
	if options.AzureTenantID == "" {
		missing = append(missing, "AZURE_TENANT_ID")
	}
	if options.AzureClientID == "" {
		missing = append(missing, "AZURE_CLIENT_ID")
	}
	if options.AzureClientSecret == "" {
		missing = append(missing, "AZURE_CLIENT_SECRET")
	}
	if options.AzureServiceURL == "" {
		missing = append(missing, "AZURE_SERVICE_URL")
	}
	if options.AzureContainerName == "" {
		missing = append(missing, "AZURE_CONTAINER_NAME")
	}
	return missing
}

func validateMissingGitLabOptions(options *types.Options) []string {
	var missing []string
	if options.GitLabToken == "" {
		missing = append(missing, "GITLAB_TOKEN")
	}
	if len(options.GitLabTemplateRepositoryIDs) == 0 {
		missing = append(missing, "GITLAB_REPOSITORY_IDS")
	}

	return missing
}

func validateDASTOptions(options *types.Options) error {
	// Ensure the DAST server token meets minimum length requirement
	if len(options.DASTServerToken) > 0 && len(options.DASTServerToken) < 16 {
		return fmt.Errorf("DAST server token must be at least 16 characters long")
	}
	return nil
}

func validateTemplatePaths(logger *gologger.Logger, templatesDirectory string, templatePaths, workflowPaths []string) {
	allGivenTemplatePaths := append(templatePaths, workflowPaths...)
	for _, templatePath := range allGivenTemplatePaths {
		if templatesDirectory != templatePath && filepath.IsAbs(templatePath) {
			fileInfo, err := os.Stat(templatePath)
			if err == nil && fileInfo.IsDir() {
				relativizedPath, err2 := filepath.Rel(templatesDirectory, templatePath)
				if err2 != nil || (len(relativizedPath) >= 2 && relativizedPath[:2] == "..") {
					logger.Warning().Msgf("The given path (%s) is outside the default template directory path (%s)! "+
						"Referenced sub-templates with relative paths in workflows will be resolved against the default template directory.", templatePath, templatesDirectory)
					break
				}
			}
		}
	}
}

func validateCertificatePaths(logger *gologger.Logger, certificatePaths ...string) {
	for _, certificatePath := range certificatePaths {
		if !fileutil.FileExists(certificatePath) {
			// The provided path to the PEM certificate does not exist for the client authentication. As this is
			// required for successful authentication, log and return an error
			logger.Fatal().Msgf("The given path (%s) to the certificate does not exist!", certificatePath)
			break
		}
	}
}
