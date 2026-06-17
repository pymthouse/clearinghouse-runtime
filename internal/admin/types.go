package admin

type Meter struct {
	ID         string
	Key        string
	Name       string
	Dimensions map[string]string
}

type MeterInput struct {
	Key           string
	Name          string
	Description   string
	EventType     string
	Aggregation   string
	ValueProperty string
	Dimensions    map[string]string
}

type Feature struct {
	ID      string
	Key     string
	Name    string
	MeterID string
}

type FeatureInput struct {
	Key     string
	Name    string
	MeterID string
}

type Plan struct {
	ID     string
	Key    string
	Name   string
	Status string
}

type PlanInput struct {
	Key              string
	Name             string
	FeatureKey       string
	FeatureName      string
	BillableMetterID string
	UnitAmount       string
	Currency         string
	BillingCadence   string
}

type EnsureResult[T any] struct {
	Resource T
	Created  bool
	Warnings []string
}

type PruneResult struct {
	DeletedMeters   []string
	DeletedFeatures []string
	DeletedPlans    []string
	Warnings        []string
}

type BootstrapOptions struct {
	Prune bool
}
