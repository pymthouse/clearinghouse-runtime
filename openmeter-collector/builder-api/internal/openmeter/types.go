package openmeter

// SessionProvision is the result of no-database OpenMeter provisioning for exchange.
type SessionProvision struct {
	Customer    *Customer
	CustomerKey string
}

// ProvisionConfig controls default-plan subscription provisioning.
type ProvisionConfig struct {
	DefaultPlanKey string
}
