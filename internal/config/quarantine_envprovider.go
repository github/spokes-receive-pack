//go:build integration

package config

func init() {
	envProviders = append(envProviders, quarantineEnvProvider{})
}

type quarantineEnvProvider struct {
}

func (e quarantineEnvProvider) env() map[string]string {
	return map[string]string{
		"GIT_SOCKSTAT_VAR_spokes_quarantine": "true",
		"GIT_SOCKSTAT_VAR_quarantine_dir":    "objects/quarantine",
	}
}
