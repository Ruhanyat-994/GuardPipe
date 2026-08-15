package trivy_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/trivy"
)

func TestParseReport_VulnerabilitiesAndSecrets(t *testing.T) {
	raw := []byte(`{
		"Results": [
			{
				"Target": "alpine:3.19 (alpine 3.19.1)",
				"Class": "os-pkgs",
				"Type": "alpine",
				"Vulnerabilities": [
					{
						"VulnerabilityID": "CVE-2024-1234",
						"PkgName": "openssl",
						"InstalledVersion": "3.1.0",
						"FixedVersion": "3.1.4",
						"Severity": "CRITICAL",
						"Title": "OpenSSL buffer overflow",
						"Description": "A buffer overflow in OpenSSL",
						"CweIDs": ["CWE-120"],
						"PrimaryURL": "https://avd.aquasec.com/nvd/cve-2024-1234",
						"Layer": {"Digest": "sha256:abc123"}
					}
				]
			},
			{
				"Target": "app/.env",
				"Class": "secret",
				"Secrets": [
					{
						"RuleID": "aws-access-key-id",
						"Category": "AWS",
						"Title": "AWS Access Key ID",
						"Severity": "CRITICAL",
						"StartLine": 2,
						"EndLine": 2,
						"Layer": {"Digest": "sha256:def456"}
					}
				]
			}
		]
	}`)

	report, err := trivy.ParseReport(raw)
	require.NoError(t, err)
	require.Len(t, report.Results, 2)

	require.Len(t, report.Results[0].Vulnerabilities, 1)
	v := report.Results[0].Vulnerabilities[0]
	require.Equal(t, "CVE-2024-1234", v.VulnerabilityID)
	require.Equal(t, "openssl", v.PkgName)
	require.Equal(t, "3.1.4", v.FixedVersion)
	require.Equal(t, "sha256:abc123", v.Layer.Digest)

	require.Len(t, report.Results[1].Secrets, 1)
	s := report.Results[1].Secrets[0]
	require.Equal(t, "aws-access-key-id", s.RuleID)
	require.Equal(t, 2, s.StartLine)
}

func TestParseReport_Misconfigurations(t *testing.T) {
	raw := []byte(`{
		"Results": [
			{
				"Target": "Dockerfile",
				"Class": "config",
				"Type": "dockerfile",
				"Misconfigurations": [
					{
						"ID": "DS002",
						"Title": "Image user should not be 'root'",
						"Message": "Specify at least 1 USER command",
						"Resolution": "Add a USER instruction to the Dockerfile",
						"Severity": "HIGH",
						"CauseMetadata": {"StartLine": 1, "EndLine": 1}
					}
				]
			}
		]
	}`)

	report, err := trivy.ParseReport(raw)
	require.NoError(t, err)
	require.Len(t, report.Results, 1)
	require.Len(t, report.Results[0].Misconfigurations, 1)

	m := report.Results[0].Misconfigurations[0]
	require.Equal(t, "DS002", m.ID)
	require.Equal(t, "HIGH", m.Severity)
	require.Equal(t, 1, m.CauseMetadata.StartLine)
}

func TestParseReport_EmptyResults(t *testing.T) {
	report, err := trivy.ParseReport([]byte(`{"Results": []}`))
	require.NoError(t, err)
	require.Empty(t, report.Results)
}

func TestParseReport_MalformedJSON_Errors(t *testing.T) {
	_, err := trivy.ParseReport([]byte(`not json`))
	require.Error(t, err)
}
