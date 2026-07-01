package openmeter

// TrialCreditBalance is the user-scoped allowance snapshot for session minting.
type TrialCreditBalance struct {
	HasAccess                bool
	BalanceUsdMicros         string
	ConsumedUsdMicros        string
	LifetimeGrantedUsdMicros string
}

// SessionProvision is the result of no-database OpenMeter provisioning for exchange.
type SessionProvision struct {
	Customer    *Customer
	CustomerKey string
	Balance     TrialCreditBalance
}

// ProvisionConfig controls subscription and allowance provisioning.
type ProvisionConfig struct {
	DefaultPlanKey              string
	TrialFeatureKey             string
	DefaultStarterIncludedMicros int64
}
